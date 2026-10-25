package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
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
