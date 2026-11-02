package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/process"
	"gitviewer/internal/remotehelper"
	"gitviewer/internal/testgit"
)

// TestReaderArgvShapesPassTheHelperWhitelist drives every product read and
// write against a real repository and validates the exact argv each one sends
// through the transport seam. It keeps the helper whitelist from drifting away
// from the reader.
func TestReaderArgvShapesPassTheHelperWhitelist(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	testgit.Run(t, dir, "add", "--", "a.txt")
	testgit.Run(t, dir, "commit", "-m", "one")
	testgit.Write(t, filepath.Join(dir, "b.txt"), "two\n")
	testgit.Run(t, dir, "add", "--", "b.txt")
	testgit.Run(t, dir, "commit", "-m", "two")
	testgit.Run(t, dir, "tag", "v1")
	testgit.Run(t, dir, "branch", "topic")
	testgit.Write(t, filepath.Join(dir, "c.txt"), "stashed\n")
	testgit.Run(t, dir, "stash", "push", "--include-untracked", "-m", "saved")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one changed\n")
	testgit.Write(t, filepath.Join(dir, "u.txt"), "untracked\n")

	// Keep identity in the fixture so writes exercise WriteEnvironment without
	// inheriting testgit's per-command identity environment.
	testgit.Run(t, dir, "config", "user.name", "Helper Coverage")
	testgit.Run(t, dir, "config", "user.email", "helper@example.invalid")
	var (
		mu     sync.Mutex
		seen   [][]string
		record bool
	)
	transport := func(ctx context.Context, workdir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
		mu.Lock()
		if record {
			seen = append(seen, append([]string(nil), args...))
		}
		mu.Unlock()
		argv := append([]string{}, gitargs.BasePrefix()...)
		argv = append(argv, "-C", workdir)
		argv = append(argv, args...)
		env := gitargs.CleanEnvironment()
		if gitargs.IsWriteArgs(args) {
			env = gitargs.WriteEnvironment()
		}
		return process.RunInput(ctx, 2*time.Minute, limit, "git", argv, env, input)
	}
	reader := git.LocalReader{
		OutputLimit: 2 << 20,
		Timeout:     2 * time.Minute,
		Transport:   transport,
		Batch: func(ctx context.Context, workdir string, limit int, commands [][]string) ([]domain.GitResult, error) {
			results := make([]domain.GitResult, len(commands))
			for i, command := range commands {
				result, err := transport(ctx, workdir, limit, nil, command...)
				if err != nil {
					return nil, err
				}
				results[i] = result
			}
			return results, nil
		},
	}
	repo, err := reader.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	head := string(testgit.Run(t, dir, "rev-parse", "HEAD"))
	head = head[:len(head)-1]

	check := func(name string, fn func() error) {
		t.Helper()
		mu.Lock()
		seen = nil
		record = true
		mu.Unlock()
		err := fn()
		mu.Lock()
		record = false
		commands := append([][]string(nil), seen...)
		mu.Unlock()
		if err != nil {
			var ge *domain.GitError
			if errors.As(err, &ge) {
				t.Fatalf("%s: %v (reason %q, cause %v), status=%q", name, err, ge.Reason, ge.Cause, testgit.Run(t, dir, "status", "--porcelain=v1"))
			}
			t.Fatalf("%s: %v", name, err)
		}
		if len(commands) == 0 {
			t.Fatalf("%s sent no Git command", name)
		}
		for _, args := range commands {
			if err := remotehelper.Validate(args); err != nil {
				t.Fatalf("%s: helper would reject %v", name, args)
			}
		}
	}

	ctx := context.Background()
	check("open", func() error { _, err := reader.Open(ctx, dir); return err })
	check("status", func() error { _, err := reader.Status(ctx, repo); return err })
	check("history", func() error { _, err := reader.History(ctx, repo, "", 0, 10); return err })
	check("head-history", func() error { _, err := reader.HeadHistory(ctx, repo, 10); return err })
	check("branches", func() error { _, err := reader.BranchList(ctx, repo); return err })
	check("tags", func() error { _, err := reader.TagList(ctx, repo); return err })
	check("stashes", func() error { _, err := reader.StashList(ctx, repo); return err })
	check("refs", func() error { _, err := reader.ListRefs(ctx, repo); return err })
	check("worktrees", func() error { _, err := reader.WorktreeList(ctx, repo); return err })
	check("commit-details", func() error { _, err := reader.CommitDetails(ctx, repo, head); return err })
	check("file-history", func() error { _, err := reader.FileHistory(ctx, repo, "a.txt"); return err })
	check("file-blame", func() error { _, err := reader.FileBlame(ctx, repo, "a.txt"); return err })
	check("worktree-diff", func() error {
		_, err := reader.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "a.txt"})
		return err
	})
	check("untracked-diff", func() error {
		_, err := reader.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "u.txt"})
		return err
	})
	check("commit-diff", func() error {
		_, err := reader.Diff(ctx, repo, domain.DiffRequest{Scope: domain.CommitDiff, Commit: head, Path: "a.txt", OldPath: "a.txt", NewPath: "a.txt"})
		return err
	})
	testgit.Run(t, dir, "add", "--", "a.txt")
	check("staged-diff", func() error {
		_, err := reader.Diff(ctx, repo, domain.DiffRequest{Scope: domain.StagedDiff, Path: "a.txt"})
		return err
	})

	write := func(op domain.GitOperation) error {
		op.RepositoryID = repo.Repository.RepositoryID
		_, err := reader.Write(ctx, repo, op)
		return err
	}
	check("unstage", func() error { return write(domain.GitOperation{Operation: domain.Unstage, Paths: []string{"a.txt"}}) })
	check("stage", func() error { return write(domain.GitOperation{Operation: domain.Stage, Paths: []string{"a.txt"}}) })
	check("commit", func() error { return write(domain.GitOperation{Operation: domain.Commit, Message: "commit from test"}) })
	check("amend", func() error { return write(domain.GitOperation{Operation: domain.Amend, Message: "amended"}) })
	_ = os.Remove(filepath.Join(dir, "u.txt"))
	check("create-branch", func() error {
		return write(domain.GitOperation{Operation: domain.CreateBranch, Ref: "review", Commit: head})
	})
	check("switch-branch", func() error {
		return write(domain.GitOperation{Operation: domain.SwitchBranch, Ref: "main"})
	})
	testgit.Write(t, filepath.Join(dir, "a.txt"), "discard me\n")
	check("discard", func() error { return write(domain.GitOperation{Operation: domain.Discard, Paths: []string{"a.txt"}}) })
	testgit.Write(t, filepath.Join(dir, "d.txt"), "stash me\n")
	check("stash-push", func() error { return write(domain.GitOperation{Operation: domain.StashPush}) })
	check("stash-pop", func() error { return write(domain.GitOperation{Operation: domain.StashPop}) })
	_ = os.Remove(filepath.Join(dir, "d.txt"))
	_ = os.Remove(filepath.Join(dir, "c.txt"))

	// Worktree writes are only reachable through the SSH reader in production.
	// Use the same transport and identity with the SSH location so the shared
	// argv is exercised without a second host.
	sshRepo := repo
	sshRepo.Repository.Location = "ssh"
	sshRepo.SSHHost = "fixture"
	worktreeReader := reader
	worktreeReader.WorktreeOpen = func(ctx context.Context, workdir string) (domain.RepositoryContext, error) {
		return reader.Open(ctx, workdir)
	}
	worktreeReader.WorktreeCheckNewPath = func(ctx context.Context, target string) error {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			return &domain.GitError{Code: domain.UnsafeOperation}
		}
		return nil
	}
	worktreeBase, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(worktreeBase, "linked")
	check("worktree-add", func() error {
		_, err := worktreeReader.CreateWorktree(ctx, sshRepo, worktree, "topic")
		return err
	})
	check("worktree-remove", func() error {
		_, err := worktreeReader.RemoveWorktree(ctx, sshRepo, worktree)
		return err
	})

	// Hunk actions cover every apply argv shape against a real index/worktree.
	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-one changed\n+hunk changed\n"
	testgit.Write(t, filepath.Join(dir, "a.txt"), "hunk changed\n")
	check("stage-hunk", func() error {
		return write(domain.GitOperation{Operation: domain.StageHunk, Paths: []string{"a.txt"}, Patch: patch})
	})
	check("unstage-hunk", func() error {
		return write(domain.GitOperation{Operation: domain.UnstageHunk, Paths: []string{"a.txt"}, Patch: patch})
	})
	check("discard-hunk", func() error {
		return write(domain.GitOperation{Operation: domain.DiscardHunk, Paths: []string{"a.txt"}, Patch: patch})
	})
	testgit.Write(t, filepath.Join(dir, "untracked-stash.txt"), "stash\n")
	check("stash-untracked", func() error { return write(domain.GitOperation{Operation: domain.StashUntracked}) })
	// Stash pop is covered above; discard this fixture entry before continuing.
	testgit.Run(t, dir, "stash", "drop")
	if err := os.Remove(filepath.Join(dir, "untracked-stash.txt")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "extra.txt"), "extra\n")
	testgit.Run(t, dir, "add", "extra.txt")
	testgit.Run(t, dir, "commit", "-m", "extra commit")
	extra := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	check("undo-commit", func() error { return write(domain.GitOperation{Operation: domain.UndoCommit}) })
	testgit.Run(t, dir, "restore", "--staged", "--", "extra.txt")
	if err := os.Remove(filepath.Join(dir, "extra.txt")); err != nil {
		t.Fatal(err)
	}
	check("cherry-pick", func() error { return write(domain.GitOperation{Operation: domain.CherryPick, Commit: extra}) })
	picked := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	check("revert", func() error { return write(domain.GitOperation{Operation: domain.Revert, Commit: picked}) })

	// Only local filesystem remotes: validate actual network write argv without
	// making any live network connection.
	remote := filepath.Join(t.TempDir(), "remote.git")
	testgit.Run(t, dir, "init", "--bare", remote)
	testgit.Run(t, dir, "remote", "add", "origin", remote)
	testgit.Run(t, dir, "push", "-u", "origin", "main")
	for _, op := range []domain.Operation{domain.Fetch, domain.FetchPrune, domain.FetchAll, domain.PullFFOnly, domain.PullRebase, domain.Push} {
		check(string(op), func() error { return write(domain.GitOperation{Operation: op}) })
	}
	for _, op := range []domain.Operation{domain.PullFrom, domain.PushTo} {
		check(string(op), func() error { return write(domain.GitOperation{Operation: op, Ref: "origin:refs/heads/main"}) })
	}

	// An unborn index covers the rm --cached unstage variant.
	unbornDir := testgit.Repo(t)
	unborn, err := reader.Open(ctx, unbornDir)
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(unbornDir, "new.txt"), "new\n")
	testgit.Run(t, unbornDir, "add", "new.txt")
	check("unstage-unborn", func() error {
		_, err := reader.Write(ctx, unborn, domain.GitOperation{RepositoryID: unborn.Repository.RepositoryID, Operation: domain.Unstage, Paths: []string{"new.txt"}})
		return err
	})

	// A conflicted merge covers ls-files --unmerged and cat-file blob.
	testgit.Run(t, dir, "switch", "--", "topic")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "topic side\n")
	testgit.Run(t, dir, "commit", "-am", "topic change")
	testgit.Run(t, dir, "switch", "--", "main")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "main side\n")
	testgit.Run(t, dir, "commit", "-am", "main change")
	merge := exec.Command("git", "-C", dir, "merge", "topic")
	merge.Env = append(gitargs.CleanEnvironment(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	if output, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("expected a merge conflict: %s", output)
	}
	check("conflict-file", func() error {
		_, err := reader.ConflictFile(ctx, repo, "a.txt")
		return err
	})
	testgit.Run(t, dir, "merge", "--abort")
}
