package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configured(t *testing.T) (gitexec.LocalReader, domain.RepositoryContext) {
	t.Helper()
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	r := gitexec.LocalReader{}
	repo, err := r.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, repo
}
func perform(t *testing.T, r gitexec.LocalReader, repo domain.RepositoryContext, op domain.GitOperation) gitexec.WriteResult {
	t.Helper()
	op.RepositoryID = repo.Repository.RepositoryID
	out, err := r.Write(context.Background(), repo, op)
	if err != nil || !out.Verified {
		t.Fatal(op.Operation, out, err)
	}
	return out
}
func TestSafeWriteHistoryAndHooks(t *testing.T) {
	ctx := context.Background()
	r, repo := configured(t)
	dir := repo.Repository.Path
	p, err := r.History(ctx, repo, "", 0, 2)
	if err != nil || len(p.Commits) != 0 {
		t.Fatal(p, err)
	}
	for _, name := range []string{"-中文\nfile", "other"} {
		testgit.Write(t, filepath.Join(dir, name), "one\n")
	}
	out := perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"-中文\nfile"}})
	if len(out.Status.Changes) != 2 {
		t.Fatal(out)
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Unstage, Paths: []string{"-中文\nfile"}})
	perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"-中文\nfile"}})
	perform(t, r, repo, domain.GitOperation{Operation: domain.Commit, Message: "initial 中文\n\nbody $(touch injected)"})
	p, err = r.History(ctx, repo, "", 0, 1)
	if err != nil || len(p.Commits) != 1 || p.More {
		t.Fatal(p, err)
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"other"}})
	hook := filepath.Join(repo.GitDir, "hooks", "pre-commit")
	testgit.Write(t, hook, "#!/bin/sh\nexit 1\n")
	if err = os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := r.Write(ctx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Commit, Message: "rejected"})
	if err == nil || !result.Verified {
		t.Fatal(result, err)
	}
	os.Remove(hook)
	perform(t, r, repo, domain.GitOperation{Operation: domain.Commit, Message: "second"})
	pinned, err := r.History(ctx, repo, p.Next, p.Offset, 1)
	if err != nil || len(pinned.Commits) != 0 {
		t.Fatal("new commit shifted anchored page", pinned, err)
	}
	p, err = r.History(ctx, repo, "", 0, 1)
	if err != nil || !p.More {
		t.Fatal(p, err)
	}
	next, err := r.History(ctx, repo, p.Next, p.Offset, 1)
	if err != nil || len(next.Commits) != 1 || next.More {
		t.Fatal(next, err)
	}
	branches, err := r.BranchList(ctx, repo)
	if err != nil || len(branches) != 1 || !branches[0].Current {
		t.Fatal(branches, err)
	}
	trees, err := r.WorktreeList(ctx, repo)
	if err != nil || len(trees) != 1 {
		t.Fatal(trees, err)
	}
	for _, op := range []domain.GitOperation{{Operation: domain.Stage, Paths: []string{"../escape"}}, {Operation: domain.SwitchBranch, Ref: "--detach"}, {Operation: domain.Push, Ref: "--force"}, {Operation: domain.Push}} {
		op.RepositoryID = repo.Repository.RepositoryID
		if _, err = r.Write(ctx, repo, op); err == nil {
			t.Fatal("unsafe operation accepted", op)
		}
	}
}
func TestSwitchDirtyAndOccupiedWorktrees(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	testgit.Write(t, filepath.Join(dir, "f"), "a")
	testgit.Run(t, dir, "add", "f")
	testgit.Run(t, dir, "commit", "-m", "initial")
	testgit.Run(t, dir, "branch", "other")
	perform(t, r, repo, domain.GitOperation{Operation: domain.SwitchBranch, Ref: "other"})
	perform(t, r, repo, domain.GitOperation{Operation: domain.SwitchBranch, Ref: "main"})
	other := filepath.Join(t.TempDir(), "worktree")
	testgit.Run(t, dir, "worktree", "add", other, "other")
	_, err := r.Write(context.Background(), repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.SwitchBranch, Ref: "other"})
	if err == nil {
		t.Fatal("occupied branch accepted")
	}
	testgit.Write(t, filepath.Join(dir, "f"), "dirty")
	_, err = r.Write(context.Background(), repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.SwitchBranch, Ref: "main"})
	if err == nil {
		t.Fatal("dirty switch accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Write(ctx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Stage, Paths: []string{"f"}})
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
		t.Fatal(err)
	}
}
func TestSyncFastForwardAndRejectedPush(t *testing.T) {
	ctx := context.Background()
	r, repo := configured(t)
	dir := repo.Repository.Path
	remote := filepath.Join(t.TempDir(), "remote.git")
	testgit.Run(t, dir, "init", "--bare", remote)
	testgit.Write(t, filepath.Join(dir, "f"), "one")
	testgit.Run(t, dir, "add", "f")
	testgit.Run(t, dir, "commit", "-m", "initial")
	testgit.Run(t, dir, "remote", "add", "origin", remote)
	testgit.Run(t, dir, "push", "-u", "origin", "main")
	testgit.Run(t, dir, "branch", "accidental")
	headOID := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Run(t, remote, "update-ref", "refs/heads/protected", headOID)
	testgit.Run(t, dir, "config", "remote.origin.mirror", "true")
	testgit.Run(t, dir, "config", "remote.origin.push", "+refs/heads/*:refs/heads/*")
	testgit.Run(t, dir, "config", "push.default", "matching")
	perform(t, r, repo, domain.GitOperation{Operation: domain.Fetch})
	perform(t, r, repo, domain.GitOperation{Operation: domain.PullFFOnly})
	perform(t, r, repo, domain.GitOperation{Operation: domain.Push})
	testgit.Run(t, remote, "show-ref", "--verify", "refs/heads/protected")
	if out := testgit.Run(t, remote, "for-each-ref", "--format=%(refname)", "refs/heads/accidental"); len(out) != 0 {
		t.Fatal("push widened to other branches", string(out))
	}
	peer := filepath.Join(t.TempDir(), "peer")
	testgit.Run(t, dir, "clone", "-b", "main", remote, peer)
	testgit.Write(t, filepath.Join(peer, "f"), "peer")
	testgit.Run(t, peer, "add", "f")
	testgit.Run(t, peer, "commit", "-m", "peer")
	testgit.Run(t, peer, "push")
	perform(t, r, repo, domain.GitOperation{Operation: domain.PullFFOnly})
	if out, err := os.ReadFile(filepath.Join(dir, "f")); err != nil || string(out) != "peer" {
		t.Fatal("fast-forward did not update worktree", string(out), err)
	}
	testgit.Write(t, filepath.Join(peer, "f"), "peer divergence")
	testgit.Run(t, peer, "commit", "-am", "peer divergence")
	testgit.Run(t, peer, "push")
	testgit.Write(t, filepath.Join(dir, "f"), "local")
	testgit.Run(t, dir, "add", "f")
	testgit.Run(t, dir, "commit", "-m", "local")
	for _, op := range []domain.Operation{domain.Push, domain.PullFFOnly} {
		out, err := r.Write(ctx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: op})
		if err == nil || !out.Verified {
			t.Fatal(op, out, err)
		}
	}
	head := strings.TrimSpace(string(testgit.Run(t, dir, "log", "-1", "--format=%s")))
	if head != "local" {
		t.Fatal("failed sync changed HEAD", head)
	}
}
