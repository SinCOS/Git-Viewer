package local

import "context"

// showDialog must return to the native event handler before waiting for an OS
// sheet. MyGo's synchronous Dialog APIs pump native events on the main thread;
// calling them during a view callback can reenter the view that is still built.
// Only work runs off-thread; its returned continuation always runs via Dispatch.
func (a *App) showDialog(work func() func()) {
	if a.closed || a.Dispatch == nil || a.DialogOpen || a.Loading || a.Writing {
		return
	}
	a.DialogOpen = true
	previousMessage := a.Message
	a.Message = "Waiting for dialog"
	dispatch := a.Dispatch
	ctx, cancel := context.WithCancel(context.Background())
	a.dialogCancel = cancel
	// The OS call has no context API and may never deliver its callback once
	// AppKit stops. Its buffered result cannot hold up controller shutdown.
	result := make(chan func(), 1)
	go func() { result <- work() }()
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		defer cancel()
		var apply func()
		select {
		case apply = <-result:
		case <-ctx.Done():
			return
		}
		dispatch(func() {
			if a.closed {
				return
			}
			a.DialogOpen = false
			a.dialogCancel = nil
			a.Message = previousMessage
			if apply != nil {
				apply()
			}
		})
	}()
}

func (a *App) ChooseWorkspaceFile() {
	choose := a.ChooseWorkspace
	if choose == nil {
		a.OpenWorkspace(a.WorkspaceInput)
		return
	}
	a.showDialog(func() func() {
		path, err := choose()
		return func() {
			if err != nil {
				a.Message = "Cannot open workspace dialog"
				return
			}
			if path != "" {
				a.OpenWorkspace(path)
			}
		}
	})
}
