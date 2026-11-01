package local

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
)

func TestCommitFileNavigationReusesDetails(t *testing.T) {
	oid := strings.Repeat("a", 40)
	var calls []string
	reader := gitexec.LocalReader{Transport: func(_ context.Context, _ string, _ int, _ []byte, args ...string) (domain.GitResult, error) {
		calls = append(calls, args[0])
		switch args[0] {
		case "show":
			if len(args) < 2 || args[1] != "--format=" {
				return domain.GitResult{}, fmt.Errorf("unnecessary commit metadata query: %v", args)
			}
			return domain.GitResult{Stdout: []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n")}, nil
		case "rev-list":
			return domain.GitResult{Stdout: []byte(oid + "\n")}, nil
		case "cat-file":
			return domain.GitResult{Stdout: []byte("new\n")}, nil
		default:
			return domain.GitResult{}, fmt.Errorf("unnecessary commit metadata query: %v", args)
		}
	}}
	a := New(application.LocalService{Reader: reader, Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 8)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "cache-test", Location: "local", Path: t.TempDir()}}
	a.CommitDetails = gitexec.CommitDetails{Commit: domain.CommitRecord{OID: oid}, Message: "cached message", Files: []gitexec.CommitFile{{Status: "M", Path: "a.txt"}, {Status: "M", Path: "b.txt"}}}
	a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: oid, Path: "a.txt"})
	for a.Loading {
		select {
		case apply := <-queue:
			apply()
		case <-time.After(5 * time.Second):
			t.Fatal("diff did not finish")
		}
	}
	a.Wait()
	if a.Stale || a.CommitDetails.Message != "cached message" || len(a.CommitDetails.Files) != 2 || a.Preview.Path != "a.txt" {
		t.Fatalf("cached details lost: message=%q preview=%+v error=%s", a.CommitDetails.Message, a.Preview, a.Message)
	}
	if len(calls) == 0 {
		t.Fatal("diff was not requested")
	}
}
