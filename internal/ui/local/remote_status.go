package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// remoteStatus describes established sessions, not merely a saved host name.
// It deliberately does not claim continuous network reachability.
func (a *App) remoteStatus() (string, bool) {
	if len(a.remoteFiles) > 0 && len(a.RemoteRoots) > 0 {
		host := a.RemoteRoots[0].SSHHost
		if b := a.remoteBrowser; b != nil && b.host == host && b.browseError {
			return "SSH: " + host + " · browse failed", true
		}
		return "SSH: " + host, true
	}
	if b := a.remoteBrowser; b != nil {
		if b.client == nil && b.loading {
			return "Connecting: " + b.host + "…", true
		}
		if b.client != nil {
			if b.browseError {
				return "SSH: " + b.host + " · browse failed", true
			}
			return "SSH: " + b.host, true
		}
	}

	if a.remoteFailedHost != "" {
		return "SSH: " + a.remoteFailedHost + " · connection failed", false
	}
	return "Connect to Host…", false
}

func (a *App) remoteStatusView(c *ui.Context) {
	label, session := a.remoteStatus()
	busy := a.Loading || a.Writing || a.DialogOpen || (a.remoteBrowser != nil && a.remoteBrowser.client == nil && a.remoteBrowser.loading)
	ui.Row(c).Label("Remote status bar").FillWidth().Height(24).Shrink(0).
		Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		button := ui.ButtonBase(c).Label("Remote connection").Tooltip(label+" · Remote actions").Height(24).MaxWidth(360).
			Padding(0, 10).Gap(7).Radius(0).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer)
		if session {
			button.Background(c.Theme().Accent).TextColor(ui.Hex("#ffffff"))
		} else if button.Hovered() {
			button.Background(c.Theme().SurfaceHover)
		}
		button.Menu(func(m *ui.Menu) {
			if m.Item("Connect to Host…").Disabled(busy).Chosen() {
				a.OpenRemoteDialog()
			}
			if m.Item("Open Remote Folder…").Disabled(busy || !session).Chosen() {
				a.OpenRemoteFolderPicker()
			}
			m.Separator()
			if m.Item("Disconnect Remote").Disabled(busy || (!session && a.remoteFailedHost == "")).Chosen() {
				a.disconnectRemote()
			}
		}).Children(func() {
			ui.Icon(c, remoteIcon).Size(14, 14).Shrink(0)
			ui.Text(c, compactLabel(label, 42)).FontSize(11).NoWrap()
		})
		ui.Box(c).Grow(1)
	})
}

func (a *App) disconnectRemote() {
	if a.Loading || a.Writing || a.DialogOpen {
		return
	}
	for _, tab := range a.EditorTabs {
		if tab.ref.Location == domain.LocationSSH && tab.state.Buffer.Modified() {
			a.RemoteOpenMessage = "Save or close unsaved files before disconnecting"
			a.Message = a.RemoteOpenMessage
			a.remoteChooseHost, a.RemoteOpen = true, true
			return
		}
	}
	a.closeRemoteBrowser()
	a.closeRemoteClients()
	for i := len(a.EditorTabs) - 1; i >= 0; i-- {
		if a.EditorTabs[i].ref.Location == domain.LocationSSH {
			a.closeEditorTab(i)
		}
	}
	if len(a.RemoteRoots) > 0 || a.RemoteWorkspaceHost != "" {
		a.CloseWorkspace()
	}
	if a.Repository.Repository.Location == "ssh" {
		a.Cancel()
		if a.watchCancel != nil {
			a.watchCancel()
			a.watchCancel = nil
		}
		if a.sync != nil {
			a.sync.stop()
			a.sync = nil
		}
		a.Repository = domain.RepositoryContext{}
		a.applyStatus(domain.GitStatus{})
		a.resetHistory()
		a.PathInput = ""
	}
	a.SSHHost, a.SSHPath, a.remoteFailedHost = "", "", ""
	a.RemoteOpen, a.RemoteOpenMessage = false, ""
	a.QuickOpen = false
	a.resetExplorerCache()
	a.notifyRemoteOpened("", "")
	a.Message = "Remote disconnected"
}
