//go:build unix

package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestReaderUsesHelperWithoutStartingSSHCLI(t *testing.T) {
	f := newPoolFixture(t)
	marker := filepath.Join(t.TempDir(), "cli-used")
	script := filepath.Join(t.TempDir(), "ssh")
	testgit.Write(t, script, "#!/bin/sh\ntouch "+marker+"\nexit 1\n")
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	executor := Executor{SSHPath: script, Helpers: f.pool, OutputLimit: 1 << 20}
	ctx := context.Background()
	repo, err := executor.Open(ctx, "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	reader := executor.Reader("fixture")
	status, err := reader.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Changes) != 1 || status.Changes[0].Path != "a.txt" {
		t.Fatalf("unexpected status: %+v", status.Changes)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the SSH CLI was started even though the helper was ready")
	}
}

func TestReaderFallsBackToCLIWhenHelperIsPermanentlyUnavailable(t *testing.T) {
	f := newPoolFixture(t)
	f.pool.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		return nil, errors.Join(ErrHelperPermanent, errors.New("no sftp"))
	}
	marker := filepath.Join(t.TempDir(), "cli-used")
	script := filepath.Join(t.TempDir(), "ssh")
	testgit.Write(t, script, `#!/bin/sh
marker='`+marker+`'
master=0
check=0
socket=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|-F) shift 2 ;;
    -S) socket=$2; shift 2 ;;
    -O) check=1; shift 2 ;;
    -N) master=1; shift ;;
    -T) shift ;;
    *) break ;;
  esac
done
if [ "$check" = 1 ]; then test -e "$socket"; exit $?; fi
if [ "$master" = 1 ]; then touch "$socket"; exec sleep 30; fi
touch "$marker"
printf '?? a.txt\0'
`)
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	pool := &Connections{}
	t.Cleanup(pool.Close)
	executor := Executor{SSHPath: script, Helpers: f.pool, Connections: pool, OutputLimit: 1 << 20}
	repo := domain.RepositoryContext{
		Repository: domain.Repository{RepositoryID: "r", Location: "ssh", Path: "/repo"},
		SSHHost:    "fixture",
	}
	status, err := executor.Reader("fixture").Status(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Changes) != 1 || status.Changes[0].Path != "a.txt" {
		t.Fatalf("unexpected status: %+v", status.Changes)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the CLI fallback never ran")
	}
}

func TestHelperFallbackRules(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"permanent", ErrHelperPermanent, true},
		{"unavailable", &domain.GitError{Code: domain.RemoteUnavailable}, true},
		{"auth", &domain.GitError{Code: domain.SshAuthFailed}, false},
		{"host key", &domain.GitError{Code: domain.HostKeyRejected}, false},
		{"protocol", &domain.GitError{Code: domain.ProtocolError}, false},
		{"cancelled", &domain.GitError{Code: domain.OperationCancelled}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := helperAcquireFallback(tc.err); got != tc.want {
			t.Errorf("acquire %s: got %t want %t", tc.name, got, tc.want)
		}
	}
	if !helperReadFallback(&domain.GitError{Code: domain.RemoteUnavailable}) {
		t.Fatal("dead read should fall back")
	}
	if helperReadFallback(&domain.GitError{Code: domain.CommandFailed}) {
		t.Fatal("command failure must not be retried")
	}
}

func TestHelperRejectsUnsafeWithoutCLIFallback(t *testing.T) {
	f := newPoolFixture(t)
	executor := Executor{SSHPath: "/must-not-start-ssh", Helpers: f.pool}
	_, err := executor.runGit(context.Background(), "fixture", "/tmp", 1024, nil, "reset", "--hard")
	if helperErrorCode(err) != domain.UnsafeOperation {
		t.Fatalf("unsafe rejection bypassed: %v", err)
	}
	_, err = executor.runGit(context.Background(), "bad;host", "/tmp", 1024, nil, "--version")
	if helperErrorCode(err) != domain.InvalidRepository {
		t.Fatalf("invalid host accepted: %v", err)
	}
}

func TestRemoteWorktreeWriteFailureIsUnknownAndNeverRetried(t *testing.T) {
	f := newPoolFixture(t)
	dir, err := filepath.EvalSymlinks(testgit.Repo(t))
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "file"), "one\n")
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "initial")
	testgit.Run(t, dir, "branch", "other")
	executor := Executor{Helpers: f.pool}
	ctx := context.Background()
	repo, err := executor.Open(ctx, "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []domain.ErrorCode{domain.RemoteUnavailable, domain.ProtocolError, domain.OperationCancelled, domain.OperationTimedOut, domain.OutputLimitExceeded} {
		t.Run(string(code), func(t *testing.T) {
			reader := executor.Reader("fixture")
			transport := reader.Transport
			writes := 0
			reader.WorktreeCheckNewPath = func(context.Context, string) error { return nil }
			reader.Transport = func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
				if len(args) > 1 && args[0] == "worktree" && args[1] == "add" {
					writes++
					return domain.GitResult{}, &domain.GitError{Code: code}
				}
				return transport(ctx, dir, limit, input, args...)
			}
			result, err := reader.CreateWorktree(ctx, repo, filepath.Join(filepath.Dir(dir), "new-tree"), "other")
			if helperErrorCode(err) != domain.OperationStateUnknown || writes != 1 || !result.Verified {
				t.Fatalf("writes=%d verified=%t err=%v", writes, result.Verified, err)
			}
		})
	}
}

func TestOpenTimeoutUsesHelper(t *testing.T) {
	f := newPoolFixture(t)
	dir := testgit.Repo(t)
	executor := Executor{Helpers: f.pool, OutputLimit: 1 << 20}
	started := time.Now()
	repo, err := executor.Open(context.Background(), "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Repository.Location != "ssh" || repo.Repository.RepositoryID == "" {
		t.Fatalf("unexpected repository: %+v", repo)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("helper open took too long")
	}
}
