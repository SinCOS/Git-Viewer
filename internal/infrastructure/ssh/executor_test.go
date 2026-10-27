package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestEncodeShellRoundTrip(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "INJECTED")
	values := []string{"", "space 中文", "line\nbreak", "single'quote", "-leading", "$(touch " + marker + ")", "`touch " + marker + "`", "; touch " + marker, "\\\"$HOME"}
	encoded, err := Encode(append([]string{"printf", "%s\\0"}, values...))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", encoded).Output()
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte(strings.Join(values, "\x00") + "\x00")
	if !bytes.Equal(out, expected) {
		t.Fatalf("%q != %q", out, expected)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("shell injection")
	}
	if _, err := Encode([]string{"bad\x00arg"}); err == nil {
		t.Fatal("NUL accepted")
	}
}
func TestRemoteGitAvailabilityAndVersionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		code         domain.ErrorCode
	}{{"missing-git", "#!/bin/sh\nprintf 'git: command not found' >&2\nexit 127\n", domain.GitUnavailable}, {"old-git", "#!/bin/sh\nprintf 'git version 2.20.0\\n'\n", domain.UnsupportedGitVersion}} {
		t.Run(tc.name, func(t *testing.T) {
			script := filepath.Join(t.TempDir(), "ssh-fixture")
			testgit.Write(t, script, tc.script)
			if err := os.Chmod(script, 0700); err != nil {
				t.Fatal(err)
			}
			_, err := (Executor{SSHPath: script}).Open(context.Background(), "fixture", "/repo")
			var ge *domain.GitError
			if !errors.As(err, &ge) || ge.Code != tc.code {
				t.Fatal(err)
			}
		})
	}
}
func TestRejectHostInjection(t *testing.T) {
	for _, host := range []string{"-oProxyCommand=touch bad", "host;touch bad", "user@host", "host\nother", ""} {
		c := remoteCommand("/tmp/repo")
		c.Repository.SSHHost = host
		if _, err := (Executor{}).Execute(context.Background(), c); err == nil {
			t.Fatal(host)
		}
	}
}
func remoteCommand(dir string) domain.GitCommand {
	c := testgit.Command(dir, domain.Status)
	c.Repository.Repository.Location = "ssh"
	c.Repository.SSHHost = "fixture"
	c.Timeout = 5 * time.Second
	return c
}
func keygen(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
}

// The server listens only on loopback, uses fresh keys, and has no relation to
// the user's ~/.ssh configuration or the machine's remote-login service.
func TestOpenSSHIntegration(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable: ", err)
	}
	dir := t.TempDir()
	keygen(t, filepath.Join(dir, "host"))
	keygen(t, filepath.Join(dir, "identity"))
	keygen(t, filepath.Join(dir, "wrong"))
	identity, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "authorized_keys"), string(identity))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\nStrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nLogLevel ERROR\n", port, filepath.Join(dir, "host"), filepath.Join(dir, "pid"), filepath.Join(dir, "authorized_keys"))
	testgit.Write(t, filepath.Join(dir, "sshd_config"), serverConfig)
	var serverLog bytes.Buffer
	server := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", filepath.Join(dir, "sshd_config"))
	server.Stdout = &serverLog
	server.Stderr = &serverLog
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	t.Cleanup(func() {
		_ = server.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("sshd shutdown timed out")
		}
	})
	ready := false
	for i := 0; i < 100; i++ {
		select {
		case err := <-done:
			done <- err
			t.Skipf("isolated sshd unavailable: %v: %s", err, serverLog.String())
		default:
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("sshd did not listen")
	}
	hostKey, err := os.ReadFile(filepath.Join(dir, "host.pub"))
	if err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known_hosts")
	testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	config := filepath.Join(dir, "ssh_config")
	writeConfig := func(identity string) {
		testgit.Write(t, config, fmt.Sprintf("Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n", port, current.Username, identity))
	}
	writeConfig(filepath.Join(dir, "identity"))
	executor := Executor{ConfigPath: config, KnownHostsFile: known}
	repo := filepath.Join(dir, "repo 中文 ' $(touch INJECTED)\n-end")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Run(t, repo, "init", "-b", "main")
	testgit.Write(t, filepath.Join(repo, "中文\nfile"), "data")
	result, err := executor.Execute(context.Background(), remoteCommand(repo))
	if err != nil {
		t.Fatalf("SSH Config/remote Git: %v stderr=%s", err, result.Stderr)
	}
	if !bytes.Contains(result.Stdout, []byte("中文\nfile\x00")) {
		t.Fatalf("%q", result.Stdout)
	}
	t.Log("real loopback SSH Config, host key verification and remote Git passed")
	t.Run("remote-product-reads-and-writes", func(t *testing.T) {
		ctx := context.Background()
		opened, err := executor.Open(ctx, "fixture", repo)
		if err != nil {
			t.Fatal(err)
		}
		if opened.SSHHost != "fixture" || opened.CommonDir == "" {
			t.Fatal(opened)
		}
		reader := executor.Reader("fixture")
		status, err := reader.Status(ctx, opened)
		if err != nil || len(status.Changes) != 1 {
			t.Fatal(status, err)
		}
		preview, err := reader.Diff(ctx, opened, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "中文\nfile"})
		if err != nil || !strings.Contains(preview.Text, "+data") {
			t.Fatal(preview, err)
		}
		testgit.Run(t, repo, "config", "user.name", "Remote Test")
		testgit.Run(t, repo, "config", "user.email", "remote@example.invalid")
		for _, op := range []domain.GitOperation{{Operation: domain.Stage, Paths: []string{"中文\nfile"}}, {Operation: domain.Unstage, Paths: []string{"中文\nfile"}}, {Operation: domain.Stage, Paths: []string{"中文\nfile"}}, {Operation: domain.Commit, Message: "remote stdin message ' $(touch INJECTED)"}} {
			op.RepositoryID = opened.Repository.RepositoryID
			written, err := reader.Write(ctx, opened, op)
			if err != nil || !written.Verified {
				t.Fatal(op, written, err)
			}
		}
		page, err := reader.History(ctx, opened, "", 0, 10)
		if err != nil || len(page.Commits) != 1 {
			t.Fatal(page, err)
		}
		preview, err = reader.Diff(ctx, opened, domain.DiffRequest{Scope: domain.CommitDiff, Commit: page.Commits[0].OID})
		if err != nil || !strings.Contains(preview.Text, "+data") {
			t.Fatal(preview, err)
		}
		branches, err := reader.BranchList(ctx, opened)
		if err != nil || len(branches) != 1 {
			t.Fatal(branches, err)
		}
		trees, err := reader.WorktreeList(ctx, opened)
		if err != nil || len(trees) != 1 {
			t.Fatal(trees, err)
		}
		if _, err = os.Stat(filepath.Join(repo, "INJECTED")); !os.IsNotExist(err) {
			t.Fatal("remote injection", err)
		}
		t.Run("remote-worktree-lifecycle", func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(parent, "worktree ' $(touch INJECTED) 中文")
			testgit.Run(t, repo, "branch", "worktree-test")
			created, err := reader.CreateWorktree(ctx, opened, target, "worktree-test")
			if err != nil || !created.Verified || len(created.Worktrees) != 2 {
				t.Fatalf("%+v %#v", created, err)
			}
			status, err := reader.WorktreeStatus(ctx, opened, target)
			if err != nil || len(status.Changes) != 0 {
				t.Fatal(status, err)
			}
			if _, err := reader.RemoveWorktree(ctx, opened, repo); err == nil {
				t.Fatal("removed main worktree")
			}
			testgit.Write(t, filepath.Join(target, "untracked"), "do not remove")
			if _, err := reader.RemoveWorktree(ctx, opened, target); err == nil {
				t.Fatal("removed dirty worktree")
			}
			if err := os.Remove(filepath.Join(target, "untracked")); err != nil {
				t.Fatal(err)
			}
			removed, err := reader.RemoveWorktree(ctx, opened, target)
			if err != nil || !removed.Verified || len(removed.Worktrees) != 1 {
				t.Fatal(removed, err)
			}
			if _, err := os.Stat(filepath.Join(parent, "INJECTED")); !os.IsNotExist(err) {
				t.Fatal("remote path was executed by a shell", err)
			}
		})
		testgit.Run(t, repo, "branch", "other")
		remote := filepath.Join(dir, "upstream.git")
		testgit.Run(t, dir, "init", "--bare", remote)
		testgit.Run(t, repo, "remote", "add", "origin", remote)
		testgit.Run(t, repo, "push", "-u", "origin", "main")
		for _, op := range []domain.GitOperation{{Operation: domain.SwitchBranch, Ref: "other"}, {Operation: domain.SwitchBranch, Ref: "main"}, {Operation: domain.Fetch}, {Operation: domain.PullFFOnly}, {Operation: domain.Push}} {
			op.RepositoryID = opened.Repository.RepositoryID
			result, err := reader.Write(ctx, opened, op)
			if err != nil || !result.Verified {
				t.Fatal(op, result, err)
			}
		}
		t.Run("running-write-cancel", func(t *testing.T) {
			testgit.Write(t, filepath.Join(repo, "中文\nfile"), "changed\n")
			testgit.Run(t, repo, "add", "--", "中文\nfile")
			marker := filepath.Join(dir, "write-started")
			finished := filepath.Join(dir, "hook-finished")
			readyCommand, _ := Encode([]string{"touch", marker})
			doneCommand, _ := Encode([]string{"touch", finished})
			hook := filepath.Join(opened.GitDir, "hooks", "pre-commit")
			testgit.Write(t, hook, "#!/bin/sh\ntrap '' HUP\n"+readyCommand+"\nsleep 0.5\n"+doneCommand+"\nexit 1\n")
			if err := os.Chmod(hook, 0700); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(hook)
			testgit.Run(t, repo, "config", "core.hooksPath", filepath.Dir(hook))
			running, cancel := context.WithCancel(ctx)
			defer cancel()
			type outcome struct {
				result  bool
				unknown bool
				err     error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := reader.Write(running, opened, domain.GitOperation{RepositoryID: opened.Repository.RepositoryID, Operation: domain.Commit, Message: "cancelled commit"})
				done <- outcome{result.Verified, result.Unknown, err}
			}()
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := os.Stat(marker); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("remote hook never started")
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
			select {
			case result := <-done:
				var ge *domain.GitError
				if !errors.As(result.err, &ge) || ge.Code != domain.OperationStateUnknown || !result.unknown || !result.result {
					t.Fatal(result)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("running remote write did not cancel and verify")
			}
			deadline = time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(finished); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("test hook did not cleanly finish")
				}
				time.Sleep(10 * time.Millisecond)
			}
			testgit.Run(t, repo, "restore", "--staged", "--", "中文\nfile")
			testgit.Run(t, repo, "restore", "--", "中文\nfile")
		})

	})

	t.Run("remote-invalid-and-permission", func(t *testing.T) {
		if _, err := executor.Open(context.Background(), "fixture", dir); err == nil {
			t.Fatal("non-repository accepted")
		}
		blocked := filepath.Join(dir, "blocked")
		if err := os.Mkdir(blocked, 0000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(blocked, 0700)
		_, err := executor.Open(context.Background(), "fixture", blocked)
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.PermissionDenied {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		name   string
		change func()
		code   domain.ErrorCode
	}{
		{"unknown-host", func() { testgit.Write(t, known, "") }, domain.HostKeyRejected},
		{"changed-host", func() {
			wrong, _ := os.ReadFile(filepath.Join(dir, "wrong.pub"))
			testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, wrong))
		}, domain.HostKeyRejected},
		{"authentication", func() {
			testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
			writeConfig(filepath.Join(dir, "wrong"))
		}, domain.SshAuthFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			result, err := executor.Execute(context.Background(), remoteCommand(repo))
			var ge *domain.GitError
			if !errors.As(err, &ge) || ge.Code != tc.code {
				t.Fatalf("%v %s", err, result.Stderr)
			}
		})
	}
	writeConfig(filepath.Join(dir, "identity"))
	testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := executor.Execute(ctx, remoteCommand(repo))
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
			t.Fatal(err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		c := remoteCommand(repo)
		c.Timeout = time.Nanosecond
		_, err := executor.Execute(context.Background(), c)
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.OperationTimedOut {
			t.Fatal(err)
		}
	})
	t.Run("disconnect", func(t *testing.T) {
		_ = server.Process.Kill()
		err := <-done
		done <- err
		_, err = executor.Execute(context.Background(), remoteCommand(repo))
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
			t.Fatal(err)
		}
	})
}
