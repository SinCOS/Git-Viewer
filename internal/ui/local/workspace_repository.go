package local

import (
	"context"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
)

// A workspace has one active Git repository. Keep it only while it remains a
// member; otherwise select the first discovered repository (or none).
func workspaceRepository(repos []domain.RepositoryContext, current string) domain.RepositoryContext {
	for _, repo := range repos {
		if repo.Repository.RepositoryID == current {
			return repo
		}
	}
	if len(repos) > 0 {
		return repos[0]
	}
	return domain.RepositoryContext{}
}

func workspaceGit(ctx context.Context, service application.LocalService, repos []domain.RepositoryContext, current string) (domain.RepositoryContext, domain.GitStatus, error) {
	repo := workspaceRepository(repos, current)
	if repo.Repository.RepositoryID == "" {
		return repo, domain.GitStatus{}, nil
	}
	status, err := service.Status(ctx, repo)
	return repo, status, err
}

// installWorkspaceGit publishes repository, status and derived-view invalidation
// together. It also retargets the watcher instead of watching the old workspace.
func (a *App) installWorkspaceGit(repo domain.RepositoryContext, status domain.GitStatus) {
	if a.Repository.Repository.RepositoryID != repo.Repository.RepositoryID {
		a.fileHistory.Release()
		a.MessageInput = ""
		a.Amend = false
	}
	a.Repository = repo
	a.PathInput = repo.Repository.Path
	a.commitFormError = ""
	a.changesCollapsed = nil
	a.applyStatus(status)
	a.resetRepositoryViews()
	a.startAuto()
	if repo.Repository.RepositoryID != "" {
		if a.workspaceChanges == nil {
			a.workspaceChanges = map[string]int{}
		}
		a.workspaceChanges[repo.Repository.RepositoryID] = len(status.Changes)
	}
}

func (a *App) selectWorkspaceRepository(id string) {
	if a.Writing || a.DialogOpen {
		return
	}
	for _, repo := range a.WorkspaceRepositories {
		if repo.Repository.RepositoryID != id {
			continue
		}
		service := a.Service
		a.start(func(ctx context.Context) (func(), error) {
			status, err := service.Status(ctx, repo)
			if err != nil {
				return nil, err
			}
			return func() { a.installWorkspaceGit(repo, status) }, nil
		})
		return
	}
}

func (a *App) workspaceRepositorySelector(c *ui.Context) {
	if len(a.WorkspaceRepositories) < 2 {
		return
	}
	ui.MenuButton(c, a.Repository.Repository.DisplayName, func(m *ui.Menu) {
		for _, repo := range a.WorkspaceRepositories {
			if m.Item(repo.Repository.Path).Checked(repo.Repository.RepositoryID == a.Repository.Repository.RepositoryID).
				Disabled(a.Writing || a.DialogOpen).Chosen() {
				a.selectWorkspaceRepository(repo.Repository.RepositoryID)
			}
		}
	}).Label("Select Git repository").Tooltip(a.Repository.Repository.Path).
		FillWidth().Height(28).Shrink(0)
}
