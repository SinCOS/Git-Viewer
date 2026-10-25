package poc

import (
	"context"
	"errors"
	"sync"

	"gitviewer/internal/domain"
)

type Task func(context.Context) (string, error)

// Tasks state is owned by the UI thread. Workers only invoke Dispatch;
// they never retain ui.Context or ui.Element values.
type Tasks struct {
	Dispatch   func(func())
	Loading    bool
	Stale      bool
	Connection string
	Message    string
	cancel     context.CancelFunc
	generation uint64
	closed     bool
	workers    sync.WaitGroup
}

func (t *Tasks) Start(task Task) {
	if t.closed || t.Dispatch == nil {
		return
	}
	t.Cancel()
	t.generation++
	generation := t.generation
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.Loading = true
	t.Message = "Loading"
	t.Connection = "checking"
	dispatch := t.Dispatch
	t.workers.Add(1)
	go func() {
		defer t.workers.Done()
		message, err := task(ctx)
		dispatch(func() {
			if t.closed || generation != t.generation {
				return
			}
			t.Loading = false
			t.cancel = nil
			cancel()
			if err != nil {
				t.Stale = true
				t.Connection = "offline"
				var ge *domain.GitError
				if errors.As(err, &ge) {
					t.Message = string(ge.Code)
				} else {
					t.Message = "Health check failed"
				}
				return
			}
			t.Stale = false
			t.Connection = "online"
			t.Message = message
		})
	}()
}
func (t *Tasks) Cancel() {
	t.generation++
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	if t.Loading {
		t.Loading = false
		t.Stale = true
		t.Message = "OperationCancelled"
		t.Connection = "cancelled"
	}
}
func (t *Tasks) Disconnect() {
	t.Cancel()
	t.Stale = true
	t.Connection = "offline"
	t.Message = "RemoteUnavailable (simulated)"
}
func (t *Tasks) Close() { t.Cancel(); t.closed = true }

// Wait is called after App.Run, so UI callbacks cannot deadlock shutdown.
func (t *Tasks) Wait() { t.workers.Wait() }
