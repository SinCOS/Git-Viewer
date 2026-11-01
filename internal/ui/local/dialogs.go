package local

import (
	"context"
	"strings"

	"gitviewer/internal/infrastructure/ssh"
)

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

// ChooseWorkspaceFile is the Workspace menu's Open workspace… entry. The OS
// sheet only sees local volumes, so a session that is already remote opens the
// remote picker instead of a dialog that cannot reach the host.
func (a *App) ChooseWorkspaceFile() {
	if a.SSHHost != "" || a.remoteBrowser != nil {
		a.OpenRemoteWorkspacePicker()
		return
	}
	choose := a.ChooseWorkspace
	if choose == nil {
		a.OpenRemoteWorkspacePicker()
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

// OpenRemoteWorkspacePicker browses .code-workspace files on the SSH host. It
// reuses the signed-in session, or connects the configured host first so the
// user does not have to open the remote panel by hand.
func (a *App) OpenRemoteWorkspacePicker() {
	if a.closed || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	if a.dirtyEditorTabs() {
		a.Message = "Save or close unsaved files before switching workspaces"
		return
	}
	host := strings.TrimSpace(a.SSHHost)
	if host == "" {
		host = strings.TrimSpace(a.RemoteHostInput)
	}
	a.RemoteOpenMessage = ""
	if a.remoteBrowser == nil && ssh.ValidHost(host) {
		a.RemoteHostInput = host
		a.connectRemoteHost()
		return
	}
	a.OpenRemoteDialog()
}

func (a *App) dirtyEditorTabs() bool {
	for _, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			return true
		}
	}
	return false
}
