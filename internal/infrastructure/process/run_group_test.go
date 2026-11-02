//go:build unix

package process

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
)

func TestRunInputGroupOutputAndExit(t *testing.T) {
	result, err := RunInputGroup(context.Background(), 5*time.Second, 1<<20, "sh", []string{"-c", "printf out; printf err >&2; exit 3"}, nil, nil)
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.CommandFailed {
		t.Fatalf("expected CommandFailed, got %v", err)
	}
	if result.ExitCode != 3 || string(result.Stdout) != "out" || string(result.Stderr) != "err" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := RunInputGroup(context.Background(), 5*time.Second, 3, "sh", []string{"-c", "printf 123456789"}, nil, nil); !errors.As(err, &ge) || ge.Code != domain.OutputLimitExceeded {
		t.Fatalf("expected output limit, got %v", err)
	}
	if _, err := RunInputGroup(context.Background(), 5*time.Second, 1024, filepath.Join(t.TempDir(), "missing"), nil, nil, nil); !errors.As(err, &ge) || ge.Code != domain.GitUnavailable {
		t.Fatalf("expected GitUnavailable, got %v", err)
	}
}

func TestRunInputGroupKillsDescendants(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := "sleep 30 & echo $! > " + pidFile + "; wait"
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, err := RunInputGroup(ctx, time.Minute, 1<<20, "sh", []string{"-c", script}, nil, nil)
		finished <- err
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
		t.Fatal("grandchild never started")
	}
	cancel()
	select {
	case err := <-finished:
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("process group did not stop")
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("grandchild %d survived cancellation", pid)
}

func TestRunInputGroupTimeout(t *testing.T) {
	_, err := RunInputGroup(context.Background(), 100*time.Millisecond, 1<<20, "sh", []string{"-c", "sleep 30"}, nil, nil)
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OperationTimedOut {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestRunInputGroupEscalatesAfterParentExits(t *testing.T) {
	for _, closePipes := range []bool{false, true} {
		t.Run(strconv.FormatBool(closePipes), func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "descendant.pid")
			redirect := ""
			if closePipes {
				redirect = " >/dev/null 2>&1"
			}
			script := `sh -c 'trap "" TERM; echo $$ > "$1"; while :; do sleep 1; done' child "$1"` + redirect + ` & wait`
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := RunInputGroup(ctx, time.Minute, 1024, "sh", []string{"-c", script, "parent", pidFile}, nil, nil)
				done <- err
			}()
			pid := 0
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
				data, _ := os.ReadFile(pidFile)
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				if pid > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid == 0 {
				t.Fatal("descendant did not start")
			}
			defer syscall.Kill(pid, syscall.SIGKILL)
			cancel()
			select {
			case err := <-done:
				var ge *domain.GitError
				if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
					t.Fatalf("cancel: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation blocked")
			}
			for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
				if syscall.Kill(pid, 0) == syscall.ESRCH {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("TERM-ignoring descendant %d survived parent exit", pid)
		})
	}
}
