package local

import (
	"context"
	"fmt"
	"strings"

	"gitviewer/internal/domain"
)

// worktreeAction confirms mutations before calling the local-only, validated
// worktree service. The service serializes writes and verifies the resulting list.
func (a *App) worktreeAction(create bool) {
	if a.syncDisabled() || a.Loading || a.Repository.Repository.Location != "local" || a.Confirm == nil {
		return
	}
	path, branch := a.WorktreePathInput, strings.TrimSpace(a.WorktreeBranchInput)
	if !create {
		if a.SelectedWorktree < 0 || a.SelectedWorktree >= len(a.Worktrees) {
			return
		}
		path = a.Worktrees[a.SelectedWorktree].Path
	}
	if path == "" || create && branch == "" {
		a.Message = "Enter an absolute path and an existing local branch"
		return
	}
	label := "Remove worktree"
	if create {
		label = "Create worktree"
	}
	detail := label + "\n" + pathLabel(path)
	if create {
		detail += "\nBranch: " + branch
	}
	repo, service, confirm := a.Repository, a.Service, a.Confirm
	a.showDialog(func() func() {
		if !confirm("Confirm Git operation", detail) {
			return nil
		}
		return func() {
			if a.closed || a.Stale || a.Repository.Repository.RepositoryID != repo.Repository.RepositoryID {
				return
			}
			a.start(func(ctx context.Context) (func(), error) {
				var trees []domain.WorktreeRecord
				var verified bool
				var err error
				if create {
					result, e := service.CreateWorktree(ctx, repo, path, branch)
					trees, verified, err = result.Worktrees, result.Verified, e
				} else {
					result, e := service.RemoveWorktree(ctx, repo, path)
					trees, verified, err = result.Worktrees, result.Verified, e
				}
				return func() {
					a.Writing = false
					if verified {
						a.Worktrees = trees
						a.SelectedWorktree = 0
					} else {
						a.Stale = true
					}
					if err != nil {
						a.Message = message(err)
					} else {
						a.Message = label + " completed"
					}
				}, nil
			})
			if a.Loading {
				a.Writing = true
				a.Message = label + " · waiting for Git"
			}
		}
	})
}

func (a *App) loadWorktreeStatus() {
	if a.Loading || a.Repository.Repository.Location != "local" || a.SelectedWorktree < 0 || a.SelectedWorktree >= len(a.Worktrees) {
		return
	}
	path := a.Worktrees[a.SelectedWorktree].Path
	service, repo := a.Service, a.Repository
	a.start(func(ctx context.Context) (func(), error) {
		status, err := service.WorktreeStatus(ctx, repo, path)
		if err != nil {
			return nil, err
		}
		return func() {
			if a.SelectedWorktree >= 0 && a.SelectedWorktree < len(a.Worktrees) && a.Worktrees[a.SelectedWorktree].Path == path {
				a.WorktreeStatusText = fmt.Sprintf("%d changed files", len(status.Changes))
			}
		}, nil
	})
}
