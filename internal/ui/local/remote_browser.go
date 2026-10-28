package local

import (
	"context"
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
	host       string
	path       string
	entries    []domain.RemoteFileInfo
	client     domain.RemoteFileSystem
	ctx        context.Context
	cancel     context.CancelFunc
	loading    bool
	generation int
}

func (a *App) closeRemoteBrowser() {
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
		files, err := sshfs.Open(ctx, sshfs.Config{
			SSHPath: service.Remote.SSHPath, ConfigPath: service.Remote.ConfigPath,
			KnownHostsFile: service.Remote.KnownHostsFile, AllowPassword: service.Remote.AllowPassword,
		}, host, "/")
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
				a.remoteChooseHost = true
				a.RemoteOpenMessage = "Cannot connect: " + message(err)
				a.Message = a.RemoteOpenMessage
				return
			}
			b.client, b.path, b.entries = files, home, remoteFolderEntries(entries)
			a.RemoteHostInput, a.RemotePathInput = host, home
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
				a.RemoteOpenMessage = "Cannot browse folder: " + message(err)
				return
			}
			b.path, b.entries = canonical, remoteFolderEntries(entries)
			a.RemotePathInput = canonical
		})
	}()
}

func (a *App) remoteOpenView(c *ui.Context) {
	ui.Modal(c, &a.RemoteOpen, func() {
		ui.Column(c).Width(620).MaxHeight(580).Padding(14).Gap(10).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(6).Children(func() {
			b := a.remoteBrowser
			if a.remoteChooseHost || b == nil || b.client == nil {
				ui.Text(c, "Connect to Host").FontSize(15).Bold()
				ui.Autocomplete(c, &a.RemoteHostInput, a.SSHHosts).Label("SSH host").
					Placeholder("Host alias, for example n1").FillWidth().AutoFocus()
				ui.Text(c, "Connect first, then choose a folder on the remote host.").FontSize(11).TextColor(c.Theme().TextMuted)
				for _, location := range a.recentRemoteLocations() {
					location := location
					ui.Button(c, location.Host+"  "+location.Path).Height(24).OnClick(func() {
						a.RemoteHostInput = location.Host
					})
				}
				ui.Button(c, "Refresh hosts").Height(24).OnClick(a.refreshRemoteHosts)
			} else {
				ui.Row(c).FillWidth().Children(func() {
					ui.Text(c, "SSH: "+b.host).Bold().Grow(1)
					ui.Button(c, "Change host…").Height(24).OnClick(func() { a.remoteChooseHost = true })
				})
				ui.Text(c, "Open Remote Folder").FontSize(15).Bold()
				ui.Row(c).FillWidth().Gap(6).Children(func() {
					ui.Button(c, "Parent folder").Height(26).Disabled(b.loading || b.path == "/").OnClick(func() { a.browseRemoteFolder(path.Dir(b.path)) })
					ui.TextInput(c, &a.RemotePathInput).Label("Remote path").Grow(1)
					ui.Button(c, "Go").Height(26).Disabled(b.loading).OnClick(func() { a.browseRemoteFolder(a.RemotePathInput) })
				})
				ui.Text(c, b.path).FontSize(11).TextColor(c.Theme().TextMuted)
				ui.Scroll(c).Height(260).FillWidth().Children(func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						if b.loading {
							ui.Text(c, "Loading folders…")
						} else if len(b.entries) == 0 {
							ui.Text(c, "No subfolders. You can open this folder.")
						}
						for _, entry := range b.entries {
							entry := entry
							ui.Button(c, entry.Name).Tooltip(entry.Path).Height(26).FillWidth().Disabled(b.loading).OnClick(func() {
								if entry.Dir {
									a.browseRemoteFolder(entry.Path)
								} else {
									a.RemotePathInput = entry.Path
								}
							})
						}
					})
				})
			}
			if a.RemoteOpenMessage != "" {
				ui.Text(c, a.RemoteOpenMessage).FontSize(11).TextColor(c.Theme().Danger)
			}
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				ui.Button(c, "Cancel").Height(26).OnClick(func() { a.RemoteOpen = false })
				if a.remoteChooseHost || b == nil || b.client == nil {
					ui.PrimaryButton(c, "Connect").Height(26).Disabled(!ssh.ValidHost(strings.TrimSpace(a.RemoteHostInput))).OnClick(a.connectRemoteHost)
				} else {
					label := "Open folder"
					if remotePathKind(a.RemotePathInput) == "workspace" {
						label = "Open workspace"
					}
					ui.PrimaryButton(c, label).Height(26).Disabled(b.loading || validateRemoteTarget(b.host, a.RemotePathInput) != "" || a.Loading || a.Writing).OnClick(a.submitRemoteOpen)
				}
			})
		})
	})
}
