package local

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/sshfs"
)

type remoteFolderBrowser struct {
	host        string
	navigation  string
	filter      string
	browseError bool
	path        string
	entries     []domain.RemoteFileInfo
	client      domain.RemoteFileSystem
	ctx         context.Context
	cancel      context.CancelFunc
	loading     bool
	generation  int
}

func (a *App) closeRemoteBrowser() {
	a.remoteFailedHost = ""
	if b := a.remoteBrowser; b != nil {
		b.cancel()
		a.remoteBrowser = nil
	}
}

// OpenRemoteFolderPicker reuses the authenticated browser, independently of
// the workspace's confined file session. Cancelling never replaces a workspace.
func (a *App) OpenRemoteFolderPicker() {
	if a.closed || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	a.RemoteOpenMessage = ""
	a.remoteChooseHost = false
	a.RemoteOpen = true
	if b := a.remoteBrowser; b != nil && b.client != nil && (a.SSHHost == "" || b.host == a.SSHHost) {
		a.RemoteHostInput = b.host
		a.RemotePathInput = b.path
		b.navigation = b.path
		if len(a.RemoteRoots) > 0 && a.SSHPath != "" {
			current := a.SSHPath
			if remotePathKind(current) == "workspace" {
				current = path.Dir(current)
			}
			if current != b.path {
				a.browseRemoteFolder(current)
			}
		}
		return
	}
	if a.SSHHost == "" {
		a.OpenRemoteDialog()
		return
	}
	a.RemoteHostInput = a.SSHHost
	a.connectRemoteHost()
}

func (a *App) connectRemoteHost() {
	host := strings.TrimSpace(a.RemoteHostInput)
	if !ssh.ValidHost(host) {
		a.RemoteOpenMessage = "Enter an SSH Host alias, for example n1"
		return
	}
	if a.Dispatch == nil || a.closed || a.Loading || a.Writing {
		return
	}
	a.closeRemoteBrowser()
	a.remoteFailedHost = ""
	ctx, cancel := context.WithCancel(context.Background())
	b := &remoteFolderBrowser{host: host, ctx: ctx, cancel: cancel, loading: true}
	a.remoteBrowser = b
	// Leave the host form while OpenSSH may ask for a password or passphrase.
	a.RemoteOpen = false
	a.Message = "Connecting to " + host + "…"
	service, dispatch := a.Service, a.Dispatch
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		files, err := sshfs.Open(ctx, service.Remote, host, "/")
		var home string
		var entries []domain.RemoteFileInfo
		if err == nil {
			// The browser owns this session until disconnect or application close.
			a.workers.Add(1)
			go func() { defer a.workers.Done(); <-ctx.Done(); _ = files.Close() }()
			home, err = files.WorkingDirectory(ctx)
			if err == nil {
				entries, err = files.ReadDir(ctx, home)
			}
		}
		dispatch(func() {
			if a.closed || a.remoteBrowser != b {
				cancel()
				return
			}
			b.loading = false
			a.RemoteOpen = true
			if err != nil {
				cancel()
				a.remoteBrowser = nil
				a.remoteFailedHost = host
				a.remoteChooseHost = true
				a.RemoteOpenMessage = "Cannot connect: " + message(err)
				a.Message = a.RemoteOpenMessage
				return
			}
			b.client, b.path, b.entries = files, home, remoteFolderEntries(entries)
			a.RemoteHostInput, a.RemotePathInput = host, home
			b.navigation = home
			a.remoteChooseHost = false
			a.RemoteOpenMessage = ""
			a.Message = "Connected to " + host + " · select a folder"
			a.activateEditorMode()
		})
	}()
}

func remoteFolderEntries(entries []domain.RemoteFileInfo) []domain.RemoteFileInfo {
	result := []domain.RemoteFileInfo{}
	for _, item := range entries {
		if !item.Symlink && (item.Dir || strings.HasSuffix(strings.ToLower(item.Name), ".code-workspace")) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Dir != result[j].Dir {
			return result[i].Dir
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (a *App) browseRemoteFolder(value string) {
	b := a.remoteBrowser
	if b == nil || b.client == nil || a.Dispatch == nil {
		return
	}
	value = strings.TrimSpace(value)
	if !path.IsAbs(value) || strings.ContainsRune(value, 0) {
		a.RemoteOpenMessage = "Enter an absolute remote path"
		return
	}
	b.generation++
	generation := b.generation
	b.loading = true
	a.RemoteOpenMessage = ""
	client, dispatch := b.client, a.Dispatch
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
		defer cancel()
		canonical, err := client.Canonical(ctx, value)
		var entries []domain.RemoteFileInfo
		if err == nil {
			entries, err = client.ReadDir(ctx, canonical)
		}
		dispatch(func() {
			if a.closed || a.remoteBrowser != b || generation != b.generation {
				return
			}
			b.loading = false
			if err != nil {
				b.browseError = true
				a.RemoteOpenMessage = "Cannot browse folder: " + message(err)
				return
			}
			b.path, b.entries = canonical, remoteFolderEntries(entries)
			b.navigation, b.filter, b.browseError = canonical, "", false
			a.RemotePathInput = canonical
		})
	}()
}

// remoteOpenView keeps the host form separate from the authenticated picker.
func (a *App) remoteOpenView(c *ui.Context) {
	ui.Modal(c, &a.RemoteOpen, func() {
		width, height := c.Size()
		ui.Column(c).Label("Remote panel").Width(min(680, max(0, width-40))).MaxHeight(min(620, max(0, height-40))).Padding(20).Gap(14).
			AlignItems(ui.Start).Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(8).Children(func() {
			b := a.remoteBrowser
			choosing := a.remoteChooseHost || b == nil || b.client == nil
			if choosing {
				a.remoteHostForm(c)
			} else {
				a.remoteFolderForm(c, b)
			}
			if a.RemoteOpenMessage != "" {
				ui.Text(c, a.RemoteOpenMessage).FillWidth().FontSize(11).TextColor(c.Theme().Danger)
			}
			ui.Row(c).FillWidth().Padding(12, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				ui.Button(c, "Cancel").Height(30).OnClick(func() { a.RemoteOpen = false })
				if choosing {
					ui.PrimaryButton(c, "Connect").Height(30).Disabled(!ssh.ValidHost(strings.TrimSpace(a.RemoteHostInput)) || a.Loading || a.Writing).OnClick(a.connectRemoteHost)
				} else {
					label := "Open folder"
					if remotePathKind(a.RemotePathInput) == "workspace" {
						label = "Open workspace"
					}
					ui.PrimaryButton(c, label).Height(30).Disabled(b.loading || validateRemoteTarget(b.host, a.RemotePathInput) != "" || a.Loading || a.Writing).OnClick(a.submitRemoteOpen)
				}
			})
		})
	}).Padding(0).Gap(0)
}

func (a *App) remoteHostForm(c *ui.Context) {
	ui.Text(c, "Connect to Host").FontSize(17).Bold()
	ui.Text(c, "Choose an SSH host, then browse its remote folders.").FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Text(c, "SSH HOST").FontSize(10).Bold().TextColor(c.Theme().TextMuted)
	ui.Autocomplete(c, &a.RemoteHostInput, a.SSHHosts).Label("SSH host").Placeholder("Host alias from SSH config").FillWidth().AutoFocus()
	ui.Row(c).FillWidth().Children(func() {
		ui.Text(c, "SSH configuration").FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
		ui.Button(c, "Refresh hosts").Height(26).OnClick(a.refreshRemoteHosts)
	})
	if recent := a.recentRemoteLocations(); len(recent) > 0 {
		ui.Text(c, "RECENT CONNECTIONS").FontSize(10).Bold().TextColor(c.Theme().TextMuted)
		_, height := c.Size()
		ui.Scroll(c).FillWidth().MaxHeight(min(244, max(70, height-440))).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, location := range recent {
					location := location
					row := ui.ButtonBase(c.Key(location.Host+":"+location.Path)).Label("Recent "+location.Host+" "+location.Path).
						Tooltip(location.Host+" · "+location.Path).FillWidth().Height(48).Padding(8, 10).Gap(10).
						AlignItems(ui.Center).Justify(ui.Start).Radius(4).Cursor(ui.CursorPointer).
						OnClick(func() { a.openRecentRemote(location) })
					if row.Hovered() {
						row.Background(c.Theme().SurfaceHover)
					} else {
						row.Background(c.Theme().Surface)
					}
					row.Children(func() {
						ui.Icon(c, remoteIcon).Size(18, 18).TextColor(c.Theme().TextMuted).Shrink(0)
						ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Start).Gap(3).Children(func() {
							ui.Text(c, location.Host).Bold().FontSize(12).NoWrap()
							ui.Text(c, compactLabel(location.Path, 65)).FontSize(11).TextColor(c.Theme().TextMuted).NoWrap()
						})
						ui.Text(c, "›").TextColor(c.Theme().TextMuted)
					})
				}
			})
		})
	}
	ui.Text(c, "Uses your SSH configuration. A password prompt appears only when needed.").FillWidth().FontSize(11).TextColor(c.Theme().TextMuted)
}

func (a *App) openRecentRemote(location domain.SavedLocation) {
	host, remotePath := strings.TrimSpace(location.Host), strings.TrimSpace(location.Path)
	if problem := validateRemoteTarget(host, remotePath); problem != "" {
		a.RemoteOpenMessage = problem
		return
	}
	if a.Loading || a.Writing {
		a.RemoteOpenMessage = "Wait for the current operation to finish"
		return
	}
	for _, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			a.RemoteOpenMessage = "Save or close unsaved files before switching folders"
			return
		}
	}
	a.RemoteHostInput, a.RemotePathInput = host, remotePath
	a.RemoteOpen, a.RemoteOpenMessage = false, ""
	a.closeRemoteBrowser()
	if location.Kind == "ssh-workspace" || remotePathKind(remotePath) == "workspace" {
		a.openRemoteWorkspace(host, remotePath, true)
		return
	}
	a.openRemoteFolder(host, remotePath, true)
}

func (a *App) remoteFolderForm(c *ui.Context, b *remoteFolderBrowser) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Open Remote Folder").FontSize(17).Bold().Grow(1)
		ui.Button(c, "Change host…").Height(26).OnClick(func() { a.remoteChooseHost = true })
	})
	ui.Text(c, "SSH: "+b.host).FontSize(12).TextColor(c.Theme().TextMuted)
	if b.navigation == "" {
		b.navigation = b.path
	}
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		iconButton(c, "Parent folder", "Parent folder", parentFolderIcon).Disabled(b.loading || b.path == "/").OnClick(func() { a.browseRemoteFolder(path.Dir(b.path)) })
		iconButton(c, "Refresh remote folder", "Refresh remote folder", refreshIcon).Disabled(b.loading).OnClick(func() { a.browseRemoteFolder(b.path) })
		input := ui.TextInput(c, &b.navigation).Label("Remote path").Tooltip(b.navigation).Grow(1)
		if input.Submitted() && !b.loading {
			a.browseRemoteFolder(b.navigation)
		}
		ui.Button(c, "Go").Height(28).Disabled(b.loading).OnClick(func() { a.browseRemoteFolder(b.navigation) })
	})
	ui.TextInput(c, &b.filter).Label("Filter folders").Placeholder("Filter this folder").FillWidth()
	ui.Row(c).FillWidth().Padding(6, 10).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, "Name").FontSize(11).Bold().Grow(1)
		ui.Text(c, "Type").FontSize(11).TextColor(c.Theme().TextMuted).Width(100)
	})
	visible := 0
	_, height := c.Size()
	ui.Scroll(c).Height(min(250, max(70, height-390))).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			if b.loading {
				ui.Text(c, "Loading folders…").Padding(12).TextColor(c.Theme().TextMuted)
			}
			for _, entry := range b.entries {
				if !strings.Contains(strings.ToLower(entry.Name), strings.ToLower(b.filter)) {
					continue
				}
				visible++
				entry := entry
				row := ui.ButtonBase(c.Key(entry.Path)).Label("Select remote "+entry.Name).Tooltip(entry.Path).
					Height(32).FillWidth().Padding(0, 10).Gap(10).AlignItems(ui.Center).Justify(ui.Start).
					Radius(2).Focusable().Disabled(b.loading).Cursor(ui.CursorPointer).
					OnClick(func() { a.RemotePathInput = entry.Path })
				if row.DoubleClicked() || row.Shortcut(0, ui.KeyEnter) {
					a.RemotePathInput = entry.Path
					if entry.Dir && !b.loading {
						a.browseRemoteFolder(entry.Path)
					}
				}
				if a.RemotePathInput == entry.Path {
					row.Background(c.Theme().Selection)
				} else if row.Hovered() {
					row.Background(c.Theme().SurfaceHover)
				}
				row.Children(func() {
					icon, kind := folderIcon, "Folder"
					if !entry.Dir {
						icon, kind = codeIcon, "Workspace"
					}
					ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(c.Theme().Accent)
					ui.Text(c, compactLabel(entry.Name, 58)).FontSize(12).NoWrap().Grow(1)
					ui.Text(c, kind).FontSize(11).TextColor(c.Theme().TextMuted).Width(100).NoWrap()
				})
			}
			if !b.loading && visible == 0 {
				label := "No subfolders. You can open this folder."
				if b.filter != "" {
					label = "No matching folders or workspaces."
				}
				ui.Text(c, label).FontSize(12).Padding(12).TextColor(c.Theme().TextMuted)
			}
		})
	})
	ui.Text(c, "Selected: "+compactLabel(a.RemotePathInput, 76)).Tooltip(a.RemotePathInput).FontSize(12).FillWidth().NoWrap()
	ui.Text(c, fmt.Sprintf("%d items · Double-click or press Enter to enter a folder", visible)).FontSize(11).TextColor(c.Theme().TextMuted)
}
