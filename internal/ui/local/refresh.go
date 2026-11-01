package local

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/watch"
)

func (a *App) startAuto() {
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
	if a.sync != nil {
		a.sync.stop()
		a.sync = nil
	}
	if !a.EnableAuto || a.closed || a.Dispatch == nil || a.Repository.Repository.RepositoryID == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	results := make(chan error, 1)
	a.autoResults = results
	background := make(chan bool, 1)
	a.autoBackground = background
	background <- a.background
	repo, dispatch := a.Repository, a.Dispatch
	policy := watch.DefaultPolicy()
	s := newSyncScheduler(a, repo.Repository.RepositoryID, dispatch, policy.Debounce)
	a.sync = s
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		watch.Run(ctx, repo, policy, background, results, func() {
			dispatch(func() {
				if a.closed || a.sync != s || a.Repository.Repository.RepositoryID != s.repoID {
					return
				}
				s.markDirty()
			})
		})
	}()
}

// requestBackgroundRefresh refreshes Source Control after an in-app file
// change. The auto-refresh scheduler coalesces the request with watcher
// events; when auto refresh is disabled, perform one quiet read directly.
func (a *App) requestBackgroundRefresh() {
	if a.Repository.Repository.RepositoryID == "" || a.closed {
		return
	}
	if a.sync != nil {
		a.sync.markDirty()
		return
	}
	a.refresh(true)
}

// requestBackgroundRefreshFor updates the repository that owns ref. In a
// multi-root workspace this refreshes all workspace badges when the saved file
// belongs to a repository other than the currently displayed one.
func (a *App) requestBackgroundRefreshFor(ref fileRef) {
	repos := append([]domain.RepositoryContext(nil), a.WorkspaceRepositories...)
	if a.Repository.Repository.RepositoryID != "" {
		found := false
		for _, repo := range repos {
			if repo.Repository.RepositoryID == a.Repository.Repository.RepositoryID {
				found = true
				break
			}
		}
		if !found {
			repos = append(repos, a.Repository)
		}
	}
	for _, repo := range repos {
		if !refWithinRepository(ref, repo) {
			continue
		}
		if repo.Repository.RepositoryID == a.Repository.Repository.RepositoryID {
			a.requestBackgroundRefresh()
		} else {
			a.loadWorkspaceStatuses()
		}
		return
	}
	a.requestBackgroundRefresh()
}

func refWithinRepository(ref fileRef, repo domain.RepositoryContext) bool {
	if repo.Repository.RepositoryID == "" || repo.Repository.Path == "" {
		return false
	}
	switch ref.Location {
	case domain.LocationSSH:
		if repo.Repository.Location != "ssh" || repo.SSHHost != ref.Host {
			return false
		}
		root := strings.TrimSuffix(path.Clean(repo.Repository.Path), "/")
		file := path.Clean(ref.Path)
		return file == root || strings.HasPrefix(file, root+"/")
	case domain.LocationLocal:
		if repo.Repository.Location != "local" {
			return false
		}
		rel, err := filepath.Rel(repo.Repository.Path, ref.Path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	default:
		return false
	}
}

func (a *App) SetBackground(background bool) {
	a.background = background
	if !background {
		// A list laid out while the window was occluded keeps geometry that
		// draws past its bounds (seen as the history table covering the
		// status bar). Re-keying on reveal rebuilds the list widgets.
		a.revealGen++
		// Returning to the app is the reliable moment to reconcile changes
		// made in another editor, even if a filesystem notification was missed.
		a.requestBackgroundRefresh()
	}
	if a.autoBackground != nil {
		select {
		case <-a.autoBackground:
		default:
		}
		select {
		case a.autoBackground <- background:
		default:
		}
	}
}
