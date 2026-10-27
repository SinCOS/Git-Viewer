package local

import (
	"context"

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
	if !a.EnableAuto || a.closed || a.Dispatch == nil {
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
func (a *App) SetBackground(background bool) {
	a.background = background
	if !background {
		// A list laid out while the window was occluded keeps geometry that
		// draws past its bounds (seen as the history table covering the
		// status bar). Re-keying on reveal rebuilds the list widgets.
		a.revealGen++
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
