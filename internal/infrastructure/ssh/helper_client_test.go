//go:build unix

package ssh

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/remotehelper"
	"gitviewer/internal/testgit"
)

// TestHelperClientServer runs only as a re-executed test binary and speaks the
// real helper protocol on stdin/stdout.
func TestHelperClientServer(t *testing.T) {
	if os.Getenv("GITVIEWER_HELPER_SERVER") != "1" {
		return
	}
	if os.Getenv("GITVIEWER_HELPER_BAD_RESPONSE") == "1" {
		if _, err := remotehelper.ReadFrame(os.Stdin); err != nil {
			os.Exit(2)
		}
		remotehelper.WriteResponse(os.Stdout, remotehelper.Response{Type: remotehelper.TypeReady, Proto: remotehelper.Proto, Platform: "test/test", PID: os.Getpid()})
		payload, err := remotehelper.ReadFrame(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		req, err := remotehelper.DecodeRequest(payload)
		if err != nil {
			os.Exit(2)
		}
		remotehelper.WriteResponse(os.Stdout, remotehelper.Response{Type: remotehelper.TypeBatchResult, ID: req.ID, Results: nil})
		os.Exit(0)
	}
	engine := remotehelper.NewEngine(os.Getenv("GITVIEWER_HELPER_GIT"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	_ = remotehelper.Serve(ctx, os.Stdin, os.Stdout, engine)
	os.Exit(0)
}

func startTestHelper(t *testing.T, gitPath string) *HelperClient {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperClientServer$")
	cmd.Env = append(os.Environ(), "GITVIEWER_HELPER_SERVER=1", "GITVIEWER_HELPER_GIT="+gitPath)
	client, err := startHelperClient(context.Background(), cmd)
	if err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func slowGit(t *testing.T, delay time.Duration) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "git")
	testgit.Write(t, script, "#!/bin/sh\nsleep "+strconv.FormatFloat(delay.Seconds(), 'f', 3, 64)+"\n")
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestHelperClientExecBatchAndConcurrency(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	client := startTestHelper(t, "")
	ctx := context.Background()

	status, err := client.Exec(ctx, dir, 1<<20, 5*time.Second, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}, nil)
	if err != nil || !strings.Contains(string(status.Stdout), "a.txt") {
		t.Fatalf("status: %v %q", err, status.Stdout)
	}
	batch, err := client.Batch(ctx, dir, 1<<20, 5*time.Second, [][]string{
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "--absolute-git-dir"},
	})
	if err != nil || len(batch) != 2 || strings.TrimSpace(string(batch[0].Stdout)) == "" {
		t.Fatalf("batch: err=%v len=%d data=%#v", err, len(batch), batch)
	}

	var wg sync.WaitGroup
	results := make([]domain.GitResult, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = client.Exec(ctx, dir, 1<<20, 5*time.Second, []string{"rev-parse", "--show-toplevel"}, nil)
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil || strings.TrimSpace(string(results[i].Stdout)) == "" {
			t.Fatalf("concurrent %d: %v", i, errs[i])
		}
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestHelperClientCancelKeepsHelperAlive(t *testing.T) {
	client := startTestHelper(t, slowGit(t, 30*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.Exec(ctx, "/tmp", 1<<20, time.Minute, []string{"--version"}, nil)
	var ge *domain.GitError
	if !errors.As(err, &ge) || (ge.Code != domain.OperationCancelled && ge.Code != domain.OperationTimedOut) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if time.Since(started) > 6*time.Second {
		t.Fatal("cancellation took too long")
	}
	if !client.Alive() {
		t.Fatal("helper died after a canceled request")
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := client.Ping(pingCtx); err != nil {
		t.Fatalf("helper did not stay usable: %v", err)
	}
}

func TestHelperClientWriteSerialization(t *testing.T) {
	client := startTestHelper(t, slowGit(t, 150*time.Millisecond))
	ctx := context.Background()
	started := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.Exec(ctx, "/tmp", 0, 10*time.Second, []string{"stash", "push"}, nil)
		}()
	}
	wg.Wait()
	if elapsed := time.Since(started); elapsed < 280*time.Millisecond {
		t.Fatalf("writes overlapped: %s", elapsed)
	}
}

func TestHelperClientDeathFailsPending(t *testing.T) {
	client := startTestHelper(t, slowGit(t, 30*time.Second))
	done := make(chan error, 1)
	go func() {
		_, err := client.Exec(context.Background(), "/tmp", 0, time.Minute, []string{"--version"}, nil)
		done <- err
	}()
	time.Sleep(150 * time.Millisecond)
	if client.cmd.Process == nil {
		t.Fatal("helper process missing")
	}
	_ = client.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-done:
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
			t.Fatalf("expected RemoteUnavailable, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending request did not fail")
	}
}

func TestHelperClientRejectsMalformedBatchResponse(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperClientServer$")
	cmd.Env = append(os.Environ(), "GITVIEWER_HELPER_SERVER=1", "GITVIEWER_HELPER_BAD_RESPONSE=1")
	client, err := startHelperClient(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Batch(context.Background(), "/tmp", 1024, time.Second, [][]string{{"--version"}})
	if helperErrorCode(err) != domain.ProtocolError {
		t.Fatalf("malformed batch accepted: %v", err)
	}
}

func TestHelperClientBlockedPipeRespectsDeadline(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exec sleep 30")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := startHelperClient(ctx, cmd); err == nil {
		t.Fatal("unresponsive helper accepted")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("handshake deadline blocked")
	}
}

func TestHelperClientStartupClassification(t *testing.T) {
	for _, tc := range []struct {
		script    string
		code      domain.ErrorCode
		permanent bool
	}{
		{"printf 'Permission denied (publickey).' >&2; exit 255", domain.SshAuthFailed, false},
		{"printf 'Host key verification failed.' >&2; exit 255", domain.HostKeyRejected, false},
		{"printf 'Permission denied' >&2; exit 126", "", true},
		{"exit 127", "", true},
	} {
		t.Run(tc.script, func(t *testing.T) {
			_, err := startHelperClient(context.Background(), exec.Command("sh", "-c", tc.script))
			if tc.permanent {
				if !errors.Is(err, ErrHelperPermanent) {
					t.Fatalf("expected permanent: %v", err)
				}
			} else if helperErrorCode(err) != tc.code {
				t.Fatalf("got %v want %s", err, tc.code)
			}
		})
	}
}

func TestHelperClientHandshakeFailure(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf 'not a frame'")
	if _, err := startHelperClient(context.Background(), cmd); err == nil {
		t.Fatal("garbage handshake was accepted")
	}
}

func TestHelperClientCloseIdempotent(t *testing.T) {
	client := startTestHelper(t, "")
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if client.Alive() {
		t.Fatal("closed client reports alive")
	}
	_, err := client.Exec(context.Background(), "/tmp", 0, time.Second, []string{"--version"}, nil)
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
		t.Fatalf("expected RemoteUnavailable after close, got %v", err)
	}
}
