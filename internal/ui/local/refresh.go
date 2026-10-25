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
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		watch.Run(ctx, repo, watch.DefaultPolicy(), background, results, func() {
			dispatch(func() {
				if !a.closed && ctx.Err() == nil && a.Repository.Repository.RepositoryID == repo.Repository.RepositoryID && !a.Loading {
					a.refresh(true)
				}
			})
		})
	}()
}
func (a *App) SetBackground(background bool) {
	a.background = background
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
