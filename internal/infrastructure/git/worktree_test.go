package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitviewer/internal/testgit"
)

func TestControlledWorktreeLifecycle(t *testing.T) {
	ctx := context.Background()
	r, repo := configured(t)
	testgit.Write(t, filepath.Join(repo.Repository.Path, "file"), "initial")
	testgit.Run(t, repo.Repository.Path, "add", "file")
	testgit.Run(t, repo.Repository.Path, "commit", "-m", "initial")
	testgit.Run(t, repo.Repository.Path, "branch", "other")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "tree 中文\nname")
	result, err := r.CreateWorktree(ctx, repo, path, "other")
	if err != nil || !result.Verified || len(result.Worktrees) != 2 {
		t.Fatalf("create: %+v %v", result, err)
	}
	status, err := r.WorktreeStatus(ctx, repo, path)
	if err != nil || len(status.Changes) != 0 {
		t.Fatalf("status: %+v %v", status, err)
	}
	for _, candidate := range []string{repo.Repository.Path, filepath.Dir(path), path + "/../tree 中文\nname"} {
		if _, err := r.RemoveWorktree(ctx, repo, candidate); err == nil {
			t.Fatalf("removed unsafe path %q", candidate)
		}
	}
	testgit.Write(t, filepath.Join(path, "untracked"), "keep")
	if _, err := r.RemoveWorktree(ctx, repo, path); err == nil {
		t.Fatal("removed dirty tree")
	}
	if err := os.Remove(filepath.Join(path, "untracked")); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(path, ".gitignore"), "ignored\n")
	testgit.Run(t, path, "add", ".gitignore")
	testgit.Run(t, path, "commit", "-m", "ignore")
	testgit.Write(t, filepath.Join(path, "ignored"), "keep")
	if _, err := r.RemoveWorktree(ctx, repo, path); err == nil {
		t.Fatal("removed ignored file")
	}
	if err := os.Remove(filepath.Join(path, "ignored")); err != nil {
		t.Fatal(err)
	}
	testgit.Run(t, repo.Repository.Path, "worktree", "lock", path)
	if _, err := r.RemoveWorktree(ctx, repo, path); err == nil {
		t.Fatal("removed locked tree")
	}
	testgit.Run(t, repo.Repository.Path, "worktree", "unlock", path)
	result, err = r.RemoveWorktree(ctx, repo, path)
	if err != nil || !result.Verified || len(result.Worktrees) != 1 {
		t.Fatalf("remove: %+v %v", result, err)
	}
	if _, err = r.WorktreeStatus(ctx, repo, path); err == nil {
		t.Fatal("status of removed tree")
	}
}

func TestControlledWorktreeRejectsUnsafeInput(t *testing.T) {
	ctx := context.Background()
	r, repo := configured(t)
	testgit.Write(t, filepath.Join(repo.Repository.Path, "file"), "initial")
	testgit.Run(t, repo.Repository.Path, "add", "file")
	testgit.Run(t, repo.Repository.Path, "commit", "-m", "initial")
	testgit.Run(t, repo.Repository.Path, "branch", "other")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "tree")
	for _, branch := range []string{"main", "--force", "missing", "other\nmalicious"} {
		if _, err := r.CreateWorktree(ctx, repo, target, branch); err == nil {
			t.Fatalf("accepted branch %q", branch)
		}
	}
	if _, err := r.CreateWorktree(ctx, repo, "relative", "other"); err == nil {
		t.Fatal("accepted relative path")
	}
	if _, err := r.CreateWorktree(ctx, repo, root, "other"); err == nil {
		t.Fatal("accepted existing path")
	}
	if _, err := r.CreateWorktree(ctx, repo, filepath.Join(repo.CommonDir, "new-tree"), "other"); err == nil {
		t.Fatal("accepted path inside Git metadata")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateWorktree(ctx, repo, filepath.Join(link, "tree"), "other"); err == nil {
		t.Fatal("accepted symlink parent")
	}
	fake := repo
	fake.CommonDir = root
	if _, err := r.CreateWorktree(ctx, fake, target, "other"); err == nil {
		t.Fatal("accepted forged context")
	}
	remote := repo
	remote.Repository.Location = "ssh"
	if _, err := r.CreateWorktree(ctx, remote, target, "other"); err == nil {
		t.Fatal("accepted remote mutation")
	}
	if _, err := r.WorktreeStatus(ctx, remote, target); err == nil {
		t.Fatal("accepted remote status")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.CreateWorktree(canceled, repo, target, "other"); err == nil {
		t.Fatal("accepted cancellation")
	}
}
