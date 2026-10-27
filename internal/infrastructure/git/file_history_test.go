package git_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
)

func TestFileHistoryAndBlameUseLiteralHeadPath(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	name := "中文 ' $(touch INJECTED)\n.txt"
	testgit.Write(t, filepath.Join(dir, name), "first line\n")
	testgit.Run(t, dir, "--literal-pathspecs", "add", "--", name)
	testgit.Run(t, dir, "commit", "-m", "first file change")
	testgit.Write(t, filepath.Join(dir, "unrelated"), "unrelated\n")
	testgit.Run(t, dir, "add", "unrelated")
	testgit.Run(t, dir, "commit", "-m", "unrelated change")
	testgit.Write(t, filepath.Join(dir, name), "first line\nsecond line\n")
	testgit.Run(t, dir, "--literal-pathspecs", "add", "--", name)
	testgit.Run(t, dir, "commit", "-m", "second file change")
	testgit.Write(t, filepath.Join(dir, name), "worktree only\n")
	before := string(testgit.Run(t, dir, "status", "--porcelain=v1", "-z"))
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	history, err := r.FileHistory(ctx, repo, name)
	if err != nil || len(history.Commits) != 2 || history.More {
		t.Fatalf("history = %+v, error = %v", history, err)
	}
	if history.Commits[0].Subject != "second file change" || history.Commits[1].Subject != "first file change" {
		t.Fatalf("unexpected commits: %+v", history.Commits)
	}
	blame, err := r.FileBlame(ctx, repo, name)
	if err != nil || !strings.Contains(blame, "first line") || !strings.Contains(blame, "second line") || strings.Contains(blame, "worktree only") {
		t.Fatalf("HEAD blame = %q, error = %v", blame, err)
	}
	if after := string(testgit.Run(t, dir, "status", "--porcelain=v1", "-z")); after != before {
		t.Fatal("history or blame changed the worktree or index")
	}
	for _, invalid := range []string{"../secret", "/tmp/secret", "bad\x00path", "untracked"} {
		if _, err := r.FileHistory(ctx, repo, invalid); err == nil {
			t.Errorf("history accepted %q", invalid)
		}
		if _, err := r.FileBlame(ctx, repo, invalid); err == nil {
			t.Errorf("blame accepted %q", invalid)
		}
	}
}

func TestFileBlameRejectsBinaryAndUnbornHead(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.FileBlame(ctx, repo, "binary"); err == nil {
		t.Fatal("blame accepted an unborn HEAD")
	}
	testgit.Write(t, filepath.Join(dir, "binary"), "binary\x00data")
	testgit.Run(t, dir, "add", "binary")
	testgit.Run(t, dir, "commit", "-m", "binary file")
	if _, err := r.FileBlame(ctx, repo, "binary"); err == nil {
		t.Fatal("blame accepted binary content")
	}
}
