package git

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gitviewer/internal/domain"
)

func TestRemoteUntrackedPreviewReadsContentOnce(t *testing.T) {
	var queries []string
	repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "remote", Location: "ssh", Path: "/repo"}, SSHHost: "fixture"}
	r := LocalReader{Transport: func(_ context.Context, _ string, _ int, _ []byte, args ...string) (domain.GitResult, error) {
		queries = append(queries, args[0])
		if args[0] == "status" {
			return domain.GitResult{Stdout: []byte("?? file\x00")}, nil
		}
		return domain.GitResult{ExitCode: 1, Stdout: []byte("diff --git a/file b/file\nnew file mode 100644\n--- /dev/null\n+++ b/file\n@@ -0,0 +1,2 @@\n+first\n+last\n\\ No newline at end of file\n")}, &domain.GitError{Code: domain.CommandFailed}
	}}
	p, err := r.Diff(context.Background(), repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "file"})
	if err != nil || !p.FullText || p.OldText != "" || p.NewText != "first\nlast" {
		t.Fatalf("preview=%+v err=%v", p, err)
	}
	if !reflect.DeepEqual(queries, []string{"status", "diff"}) {
		t.Fatalf("duplicate remote content read: %v", queries)
	}
}

func TestRemoteUntrackedFailureIsNotHidden(t *testing.T) {
	repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "remote", Location: "ssh", Path: "/repo"}}
	r := LocalReader{Transport: func(_ context.Context, _ string, _ int, _ []byte, args ...string) (domain.GitResult, error) {
		if args[0] == "status" {
			return domain.GitResult{Stdout: []byte("?? file\x00")}, nil
		}
		return domain.GitResult{ExitCode: 255}, &domain.GitError{Code: domain.RemoteUnavailable}
	}}
	_, err := r.Diff(context.Background(), repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "file"})
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
		t.Fatal(err)
	}
}
