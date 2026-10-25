package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func TestRepositoryAndRecent(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	sub := filepath.Join(dir, "中文 space\nsub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	r := git.LocalReader{}
	root, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := r.Open(ctx, sub)
	if err != nil || nested != root {
		t.Fatalf("%+v %v", nested, err)
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	alias, err := r.Open(ctx, link)
	if err != nil || alias != root {
		t.Fatalf("%+v %v", alias, err)
	}
	for _, bad := range []string{"", t.TempDir(), filepath.Join(dir, ".git"), "/missing\x00"} {
		if _, err := r.Open(ctx, bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	bare := filepath.Join(t.TempDir(), "bare")
	testgit.Run(t, filepath.Dir(bare), "init", "--bare", bare)
	if _, err := r.Open(ctx, bare); err == nil {
		t.Fatal("bare accepted")
	}
	testgit.Write(t, filepath.Join(dir, "base"), "base")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	wt := filepath.Join(t.TempDir(), "worktree")
	testgit.Run(t, dir, "worktree", "add", "-b", "second", wt)
	worktree, err := r.Open(ctx, wt)
	if err != nil || worktree.CommonDir != root.CommonDir || worktree.GitDir == root.GitDir || worktree.Repository.RepositoryID == root.Repository.RepositoryID {
		t.Fatalf("%+v %v", worktree, err)
	}
	store := storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "app", "recent.json")}
	for _, repo := range []domain.Repository{root.Repository, worktree.Repository, root.Repository} {
		if _, err := store.Remember(repo); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := store.Load()
	if err != nil || len(recent) != 2 || recent[0] != root.Repository {
		t.Fatalf("%+v %v", recent, err)
	}
	testgit.Write(t, store.Path, "invalid")
	if _, err := store.Load(); err == nil {
		t.Fatal("corruption accepted")
	}
}
