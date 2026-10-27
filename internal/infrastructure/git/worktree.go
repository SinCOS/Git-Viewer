package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitviewer/internal/domain"
)

// WorktreeWriteResult contains the refreshed list after an attempted mutation.
type WorktreeWriteResult struct {
	Worktrees []domain.WorktreeRecord
	Verified  bool
}

func worktreeUnsafe(reason string) error {
	return &domain.GitError{Code: domain.UnsafeOperation, Reason: reason}
}

// localWorktreeRepository reopens the repository so callers cannot substitute a
// different repository or lock key in an execution context.
func (r LocalReader) localWorktreeRepository(ctx context.Context, repo domain.RepositoryContext) error {
	if repo.Repository.Location != "local" || repo.SSHHost != "" || r.Transport != nil || repo.Repository.RepositoryID == "" {
		return worktreeUnsafe("Worktree operations require a local repository")
	}
	opened, err := r.Open(ctx, repo.Repository.Path)
	if err != nil {
		return err
	}
	if opened.Repository.RepositoryID != repo.Repository.RepositoryID || opened.Repository.Path != repo.Repository.Path || opened.GitDir != repo.GitDir || opened.CommonDir != repo.CommonDir {
		return worktreeUnsafe("Repository context changed")
	}
	return nil
}

// WorktreeStatus reads the status of a currently registered local worktree.
func (r LocalReader) WorktreeStatus(ctx context.Context, repo domain.RepositoryContext, path string) (domain.GitStatus, error) {
	if err := r.localWorktreeRepository(ctx, repo); err != nil {
		return domain.GitStatus{}, err
	}
	trees, err := r.WorktreeList(ctx, repo)
	if err != nil {
		return domain.GitStatus{}, err
	}
	found := false
	for _, tree := range trees {
		if tree.Path == path && !tree.Bare && !tree.Prunable {
			found = true
			break
		}
	}
	if !found {
		return domain.GitStatus{}, worktreeUnsafe("Select a registered worktree")
	}
	target, err := r.Open(ctx, path)
	if err != nil {
		return domain.GitStatus{}, err
	}
	if target.Repository.Path != path || target.CommonDir != repo.CommonDir {
		return domain.GitStatus{}, worktreeUnsafe("Worktree identity changed")
	}
	return r.Status(ctx, target)
}

// CreateWorktree checks out an existing, unoccupied local branch at a new path.
func (r LocalReader) CreateWorktree(ctx context.Context, repo domain.RepositoryContext, path, branch string) (WorktreeWriteResult, error) {
	return r.changeWorktree(ctx, repo, path, branch, true)
}

// RemoveWorktree removes a clean, unlocked linked worktree without force.
func (r LocalReader) RemoveWorktree(ctx context.Context, repo domain.RepositoryContext, path string) (WorktreeWriteResult, error) {
	return r.changeWorktree(ctx, repo, path, "", false)
}

func (r LocalReader) changeWorktree(ctx context.Context, repo domain.RepositoryContext, path, branch string, create bool) (WorktreeWriteResult, error) {
	var result WorktreeWriteResult
	if err := r.localWorktreeRepository(ctx, repo); err != nil {
		return result, err
	}
	release, err := acquire(ctx, repo.Repository.Location+"\x00"+repo.SSHHost+"\x00"+repo.CommonDir)
	if err != nil {
		return result, err
	}
	defer release()
	if err = r.localWorktreeRepository(ctx, repo); err != nil {
		return result, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) || path == string(filepath.Separator) {
		return result, worktreeUnsafe("Select an absolute canonical worktree path")
	}
	trees, err := r.WorktreeList(ctx, repo)
	if err != nil {
		return result, err
	}
	var args []string
	if create {
		for _, gitDir := range []string{repo.GitDir, repo.CommonDir} {
			rel, e := filepath.Rel(gitDir, path)
			if e != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return result, worktreeUnsafe("Worktree path cannot be inside Git metadata")
			}
		}
		parent, e := filepath.EvalSymlinks(filepath.Dir(path))
		if e != nil || parent != filepath.Dir(path) {
			return result, worktreeUnsafe("Worktree parent must exist and be canonical")
		}
		if _, e = os.Lstat(path); !os.IsNotExist(e) {
			return result, worktreeUnsafe("Worktree path must not exist")
		}
		if branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
			return result, worktreeUnsafe("Select an existing local branch")
		}
		branches, e := r.BranchList(ctx, repo)
		if e != nil {
			return result, e
		}
		found := false
		for _, b := range branches {
			if b.Name == "refs/heads/"+branch {
				found = true
			}
		}
		if !found {
			return result, worktreeUnsafe("Select an existing local branch")
		}
		for _, tree := range trees {
			if tree.Branch == "refs/heads/"+branch {
				return result, worktreeUnsafe("Branch is already checked out")
			}
		}
		args = []string{"worktree", "add", "--", path, branch}
	} else {
		found := false
		for i, tree := range trees {
			if tree.Path == path && i > 0 && !tree.Bare && !tree.Locked && !tree.Prunable && path != repo.Repository.Path {
				found = true
				break
			}
		}
		if !found {
			return result, worktreeUnsafe("Select an unlocked linked worktree")
		}
		target, e := r.Open(ctx, path)
		if e != nil {
			return result, e
		}
		if target.Repository.Path != path || target.CommonDir != repo.CommonDir {
			return result, worktreeUnsafe("Worktree identity changed")
		}
		status, e := r.run(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored")
		if e != nil {
			return result, e
		}
		if len(status.Stdout) != 0 {
			return result, worktreeUnsafe("Worktree must have no changes or ignored files")
		}
		args = []string{"worktree", "remove", "--", path}
	}
	r.WriteMode = true
	r.Timeout = 2 * time.Minute
	_, err = r.run(ctx, repo.Repository.Path, args...)
	verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	r.WriteMode = false
	r.Timeout = 10 * time.Second
	var errVerify error
	result.Worktrees, errVerify = r.WorktreeList(verifyCtx, repo)
	result.Verified = errVerify == nil
	if err == nil {
		err = errVerify
	}
	if err == nil {
		present := false
		for _, tree := range result.Worktrees {
			if tree.Path == path {
				present = true
			}
		}
		if present != create {
			err = worktreeUnsafe("Worktree verification failed")
		}
	}
	return result, err
}
