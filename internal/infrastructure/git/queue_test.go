package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteWriteUnknownIsVerifiedWithoutRetry(t *testing.T) {
	writes := 0
	reads := 0
	reader := gitexec.LocalReader{Transport: func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
		if args[0] == "status" {
			reads++
			return domain.GitResult{Stdout: []byte("?? file\x00")}, nil
		}
		if args[0] == "add" {
			writes++
			return domain.GitResult{}, &domain.GitError{Code: domain.RemoteUnavailable}
		}
		t.Fatalf("unexpected argv %v", args)
		return domain.GitResult{}, nil
	}}
	repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "repo", Location: "ssh", Path: "/repo"}, SSHHost: "host", CommonDir: "/common"}
	result, err := reader.Write(context.Background(), repo, domain.GitOperation{RepositoryID: "repo", Operation: domain.Stage, Paths: []string{"file"}})
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OperationStateUnknown || !result.Unknown || !result.Verified || writes != 1 || reads != 2 {
		t.Fatal(result, err, writes, reads)
	}
}
func TestCommonDirectoryWriteSerialization(t *testing.T) {
	var active, peak atomic.Int32
	reader := gitexec.LocalReader{Transport: func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
		switch args[0] {
		case "status":
			return domain.GitResult{Stdout: []byte("?? file\x00")}, nil
		case "add":
			n := active.Add(1)
			for {
				old := peak.Load()
				if old >= n || peak.CompareAndSwap(old, n) {
					break
				}
			}
			defer active.Add(-1)
			select {
			case <-ctx.Done():
				return domain.GitResult{}, &domain.GitError{Code: domain.OperationCancelled}
			case <-time.After(5 * time.Millisecond):
			}
			return domain.GitResult{}, nil
		}
		t.Errorf("unexpected command %v", args)
		return domain.GitResult{}, nil
	}}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "repo", Location: "ssh", Path: "/one"}, SSHHost: "host", CommonDir: "/shared"}
			if i%2 == 0 {
				repo.Repository.Path = "/two"
			}
			result, err := reader.Write(context.Background(), repo, domain.GitOperation{RepositoryID: "repo", Operation: domain.Stage, Paths: []string{"file"}})
			if err != nil || !result.Verified {
				t.Error(result, err)
			}
		}(i)
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatal("worktrees overlapped", peak.Load())
	}
}

func TestRemoteStageHunkSendsPatchToApply(t *testing.T) {
	patch := "diff --git a/file b/file\n--- a/file\n+++ b/file\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	var applied []byte
	reader := gitexec.LocalReader{Transport: func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
		switch args[0] {
		case "status":
			return domain.GitResult{Stdout: []byte(" M file\x00")}, nil
		case "apply":
			applied = append([]byte(nil), input...)
			if !strings.Contains(strings.Join(args, " "), "--cached") {
				t.Fatalf("apply did not target the index: %v", args)
			}
			return domain.GitResult{}, nil
		}
		t.Fatalf("unexpected argv %v", args)
		return domain.GitResult{}, nil
	}}
	repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "repo", Location: "ssh", Path: "/repo"}, SSHHost: "host", CommonDir: "/common"}
	result, err := reader.Write(context.Background(), repo, domain.GitOperation{
		RepositoryID: "repo", Operation: domain.StageHunk, Paths: []string{"file"}, Patch: patch,
	})
	if err != nil || !result.Verified || string(applied) != patch {
		t.Fatalf("remote stage hunk failed: %+v %v %q", result, err, applied)
	}
}

func TestRemotePullAndPushTargetRequestedBranch(t *testing.T) {
	cases := []struct {
		name string
		op   domain.Operation
		ref  string
		want string
	}{
		{"pull from", domain.PullFrom, "origin:refs/heads/main", "pull --ff-only --no-rebase --progress -- origin refs/heads/main"},
		{"push to", domain.PushTo, "origin:refs/heads/main", "HEAD:refs/heads/main"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			reader := gitexec.LocalReader{Transport: func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
				switch {
				case args[0] == "status":
					return domain.GitResult{}, nil
				case strings.Contains(strings.Join(args, " "), "pull") || strings.Contains(strings.Join(args, " "), "push"):
					got = strings.Join(args, " ")
					return domain.GitResult{}, nil
				}
				t.Fatalf("unexpected argv %v", args)
				return domain.GitResult{}, nil
			}}
			repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "repo", Location: "ssh", Path: "/repo"}, SSHHost: "host", CommonDir: "/common"}
			result, err := reader.Write(context.Background(), repo, domain.GitOperation{
				RepositoryID: "repo", Operation: tc.op, Ref: tc.ref,
			})
			if err != nil || !result.Verified || !strings.Contains(got, tc.want) {
				t.Fatalf("%s failed: %+v %v %q", tc.name, result, err, got)
			}
		})
	}
}
