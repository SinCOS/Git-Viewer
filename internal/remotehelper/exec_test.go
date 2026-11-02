//go:build unix

package remotehelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
	"gitviewer/internal/testgit"
)

func TestEngineReadsAndWritesRepository(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	testgit.Run(t, dir, "add", "--", "a.txt")
	engine := NewEngine("")
	ctx := context.Background()

	status := engine.Exec(ctx, dir, 1<<20, 5*time.Second, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}, nil)
	if status.Error != nil || !strings.Contains(string(status.Stdout), "a.txt") {
		t.Fatalf("status: %+v", status)
	}
	commit := engine.Exec(ctx, dir, 1<<20, 30*time.Second, []string{"commit", "--file=-"}, []byte("first\n"))
	if commit.Error != nil || commit.Exit != 0 {
		t.Fatalf("commit: %+v", commit)
	}
	history := engine.Exec(ctx, dir, 1<<20, 5*time.Second, []string{"log", "--topo-order", "-z", "--format=" + gitargs.HistoryFormat, "--max-count=5", "--skip=0", strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD"))), "--"}, nil)
	if history.Error != nil || len(history.Stdout) == 0 {
		t.Fatalf("log: %+v", history)
	}
	apply := engine.Exec(ctx, dir, 1<<20, 5*time.Second, []string{"apply", "--unidiff-zero", "--whitespace=nowarn", "--cached", "-"}, []byte("bad patch"))
	if apply.Error == nil || apply.Error.Code != string(domain.CommandFailed) {
		t.Fatalf("expected a rejected patch, got %+v", apply)
	}
}

func TestEngineValidatesBeforeRunning(t *testing.T) {
	engine := NewEngine("")
	ctx := context.Background()
	cases := []struct {
		name string
		dir  string
		args []string
	}{
		{"relative dir", "repo", []string{"--version"}},
		{"shell", "/tmp", []string{"sh", "-c", "true"}},
		{"force push", "/tmp", []string{"push", "--force"}},
		{"nul", "/tmp", []string{"--version\x00"}},
	}
	for _, tc := range cases {
		result := engine.Exec(ctx, tc.dir, 0, time.Second, tc.args, nil)
		if result.Error == nil || result.Error.Code != string(domain.UnsafeOperation) {
			t.Errorf("%s: %+v", tc.name, result)
		}
	}
	if result := engine.Exec(ctx, "/tmp", MaxLimit+1, 5*time.Second, []string{"--version"}, nil); result.Error == nil || result.Error.Code != string(domain.UnsafeOperation) {
		t.Errorf("oversized limit accepted: %+v", result)
	}
	if result := engine.Exec(ctx, "/tmp", 0, 5*time.Second, []string{"--version"}, make([]byte, MaxInput+1)); result.Error == nil {
		t.Error("oversized input accepted")
	}
}

func TestEngineEnvironmentPolicy(t *testing.T) {
	script := filepath.Join(t.TempDir(), "git")
	testgit.Write(t, script, `#!/bin/sh
printf 'global=%s\n' "$GIT_CONFIG_GLOBAL"
printf 'nosystem=%s\n' "$GIT_CONFIG_NOSYSTEM"
printf 'prompt=%s\n' "$GIT_TERMINAL_PROMPT"
`)
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(script)
	ctx := context.Background()
	read := engine.Exec(ctx, "/tmp", 0, 5*time.Second, []string{"--version"}, nil)
	if read.Error != nil || !strings.Contains(string(read.Stdout), "global=/dev/null") || !strings.Contains(string(read.Stdout), "nosystem=1") {
		t.Fatalf("read environment: %+v", read)
	}
	write := engine.Exec(ctx, "/tmp", 0, 5*time.Second, []string{"commit", "--file=-"}, []byte("x"))
	if write.Error != nil || strings.Contains(string(write.Stdout), "global=/dev/null") || strings.Contains(string(write.Stdout), "nosystem=1") {
		t.Fatalf("write environment: %+v", write)
	}
}

func TestEngineLimitsTimeoutAndMissingGit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	testgit.Write(t, script, "#!/bin/sh\nprintf '0123456789012345678901234567890'\n")
	_ = os.Chmod(script, 0700)
	engine := NewEngine(script)
	result := engine.Exec(context.Background(), "/tmp", 10, 5*time.Second, []string{"--version"}, nil)
	if result.Error == nil || result.Error.Code != string(domain.OutputLimitExceeded) {
		t.Fatalf("expected output limit, got %+v", result)
	}
	testgit.Write(t, script, "#!/bin/sh\nsleep 30\n")
	_ = os.Chmod(script, 0700)
	start := time.Now()
	result = engine.Exec(context.Background(), "/tmp", 0, 100*time.Millisecond, []string{"--version"}, nil)
	if result.Error == nil || result.Error.Code != string(domain.OperationTimedOut) || time.Since(start) > 5*time.Second {
		t.Fatalf("expected timeout, got %+v", result)
	}
	missing := NewEngine(filepath.Join(dir, "missing"))
	result = missing.Exec(context.Background(), "/tmp", 0, 5*time.Second, []string{"--version"}, nil)
	if result.Error == nil || result.Error.Code != string(domain.GitUnavailable) {
		t.Fatalf("expected GitUnavailable, got %+v", result)
	}
}

func TestEngineCancelKillsHookDescendants(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	pidFile := filepath.Join(t.TempDir(), "hook.pid")
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	testgit.Write(t, hook, "#!/bin/sh\nsleep 30 &\necho $! > "+pidFile+"\nwait\n")
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	testgit.Run(t, dir, "add", "--", "a.txt")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan CommandResult, 1)
	go func() {
		done <- NewEngine("").Exec(ctx, dir, 1<<20, time.Minute, []string{"commit", "--file=-"}, []byte("blocked\n"))
	}()
	var pid int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(pidFile)
		if err == nil && len(data) > 0 {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if pid != 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		t.Fatal("hook never started")
	}
	cancel()
	select {
	case result := <-done:
		if result.Error == nil || result.Error.Code != string(domain.OperationCancelled) {
			t.Fatalf("expected cancellation, got %+v", result)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("canceled commit did not return")
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("hook descendant %d survived cancellation", pid)
}

func TestEngineSerializesWrites(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	testgit.Run(t, dir, "add", "--", "a.txt")
	testgit.Run(t, dir, "commit", "-m", "initial")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "two\n")
	engine := NewEngine("")
	ctx := context.Background()
	started := time.Now()
	results := make(chan CommandResult, 2)
	for _, message := range []string{"one", "two"} {
		go func(message string) {
			results <- engine.Exec(ctx, dir, 1<<20, 30*time.Second, []string{"stash", "push"}, nil)
		}(message)
	}
	first, second := <-results, <-results
	if first.Error != nil && second.Error != nil {
		t.Fatalf("both writes failed: %+v %+v", first, second)
	}
	if time.Since(started) > 30*time.Second {
		t.Fatal("writes did not finish")
	}
}

func TestEngineDeadlineIncludesSlotQueue(t *testing.T) {
	engine := NewEngine("missing-must-not-start")
	for _, write := range []bool{false, true} {
		slots := engine.readSlots
		args := []string{"--version"}
		if write {
			slots = engine.writeSlots
			args = []string{"commit", "--file=-"}
		}
		for i := 0; i < cap(slots); i++ {
			slots <- struct{}{}
		}
		start := time.Now()
		result := engine.Exec(context.Background(), "/tmp", 0, 30*time.Millisecond, args, nil)
		for i := 0; i < cap(slots); i++ {
			<-slots
		}
		if result.Error == nil || result.Error.Code != string(domain.OperationTimedOut) || time.Since(start) > time.Second {
			t.Fatalf("queue deadline: %+v", result)
		}
	}
}

func TestEngineNonUTF8RepositoryPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo\xff")
	if err := os.Mkdir(dir, 0700); err != nil {
		if errors.Is(err, syscall.EILSEQ) {
			t.Skip("filesystem rejects non-UTF-8 names; protocol byte round trip is covered separately")
		}
		t.Fatal(err)
	}
	testgit.Run(t, dir, "init")
	result := NewEngine("").Exec(context.Background(), dir, 0, 5*time.Second, []string{"rev-parse", "--is-inside-work-tree"}, nil)
	if result.Error != nil || string(result.Stdout) != "true\n" {
		t.Fatalf("byte directory: %+v", result)
	}
}
