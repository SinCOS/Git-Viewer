package local

import (
	posixpath "path"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
)

// OpenRemoteDialog shows the in-app remote target form. Authentication and
// host-key trust stay in the user's OpenSSH configuration.
func (a *App) OpenRemoteDialog() {
	if a.closed || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	a.RemoteOpenMessage = ""
	a.refreshRemoteHosts()
	if a.RemoteHostInput == "" {
		a.RemoteHostInput = a.SSHHost
	}
	if a.RemotePathInput == "" {
		a.RemotePathInput = a.SSHPath
	}
	if a.RemoteHostInput == "" && a.RemotePathInput == "" {
		for _, location := range a.Locations {
			if isRemoteLocation(location) {
				a.RemoteHostInput = location.Host
				a.RemotePathInput = location.Path
				break
			}
		}
	}
	a.RemoteOpen = true
}

func (a *App) refreshRemoteHosts() {
	hosts, err := ssh.Hosts(a.Service.Remote.ConfigPath)
	if err != nil {
		a.RemoteOpenMessage = "Cannot read the OpenSSH config"
		return
	}
	a.SSHHosts = hosts
	a.RemoteOpenMessage = ""
}

func isRemoteLocation(location domain.SavedLocation) bool {
	return (location.Kind == "ssh" || location.Kind == "ssh-folder" || location.Kind == "ssh-workspace") &&
		location.Host != "" && posixpath.IsAbs(location.Path)
}

func remotePathKind(value string) string {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(value)), ".code-workspace") {
		return "workspace"
	}
	return "folder"
}

func validateRemoteTarget(host, remotePath string) string {
	if !ssh.ValidHost(strings.TrimSpace(host)) {
		return "Enter an SSH Host alias, for example dev-box"
	}
	if !posixpath.IsAbs(strings.TrimSpace(remotePath)) || strings.ContainsRune(remotePath, 0) {
		return "Enter an absolute remote path"
	}
	return ""
}

func (a *App) submitRemoteOpen() {
	host := strings.TrimSpace(a.RemoteHostInput)
	remotePath := strings.TrimSpace(a.RemotePathInput)
	if problem := validateRemoteTarget(host, remotePath); problem != "" {
		a.RemoteOpenMessage = problem
		return
	}
	if a.Loading || a.Writing {
		a.RemoteOpenMessage = "Wait for the current operation to finish"
		return
	}
	a.SSHHost, a.SSHPath = host, remotePath
	a.RemoteOpen = false
	a.RemoteOpenMessage = ""
	if remotePathKind(remotePath) == "workspace" {
		a.OpenRemoteWorkspace(host, remotePath)
		return
	}
	a.OpenRemoteFolder(host, remotePath)
}

func (a *App) recentRemoteLocations() []domain.SavedLocation {
	result := make([]domain.SavedLocation, 0, 5)
	seen := map[string]bool{}
	for _, location := range a.Locations {
		if !isRemoteLocation(location) {
			continue
		}
		key := location.Kind + "\x00" + location.Host + "\x00" + location.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, location)
		if len(result) == 5 {
			break
		}
	}
	return result
}

func (a *App) remoteOpenView(c *ui.Context) {
	ui.Modal(c, &a.RemoteOpen, func() {
		kind := remotePathKind(a.RemotePathInput)
		openLabel := "Open folder"
		if kind == "workspace" {
			openLabel = "Open workspace"
		}
		canOpen := validateRemoteTarget(a.RemoteHostInput, a.RemotePathInput) == "" && !a.Loading && !a.Writing
		ui.Column(c).Width(620).MaxHeight(520).Padding(14).Gap(10).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(6).Children(func() {
			ui.Row(c).FillWidth().Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "Open Remote").FontSize(15).Bold()
					ui.Text(c, "Use an SSH Host alias and an absolute path on that host.").
						FontSize(11).TextColor(c.Theme().TextMuted)
				})
				ui.Button(c, "Refresh hosts").Height(24).OnClick(a.refreshRemoteHosts)
			})

			ui.Autocomplete(c, &a.RemoteHostInput, a.SSHHosts).
				Label("SSH host").Placeholder("Host alias from ~/.ssh/config").
				FillWidth().AutoFocus()
			ui.TextInput(c, &a.RemotePathInput).
				Label("Remote path").Placeholder("/srv/project or /srv/project.code-workspace").
				FillWidth()
			ui.Text(c, "OpenSSH handles identity, ssh-agent and host-key checking. The host key must already be trusted.").
				FontSize(11).TextColor(c.Theme().TextMuted)

			if message := strings.TrimSpace(a.RemoteOpenMessage); message != "" {
				ui.Text(c, message).FontSize(11).TextColor(c.Theme().Danger)
			}

			if recent := a.recentRemoteLocations(); len(recent) > 0 {
				ui.Text(c, "RECENT REMOTE LOCATIONS").FontSize(10).Bold().TextColor(c.Theme().TextMuted)
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for _, location := range recent {
						location := location
						label := location.Host + "  " + location.Path
						ui.ButtonBase(c).Label(label).Tooltip(label).FillWidth().Height(26).Radius(2).
							AlignItems(ui.Center).OnClick(func() {
							a.RemoteHostInput = location.Host
							a.RemotePathInput = location.Path
							a.RemoteOpenMessage = ""
						}).Children(func() {
							ui.Text(c, label).FontSize(11).NoWrap().TextColor(c.Theme().Text)
						})
					}
				})
			}

			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				ui.Button(c, "Cancel").Height(26).OnClick(func() {
					a.RemoteOpen = false
					a.RemoteOpenMessage = ""
				})
				ui.PrimaryButton(c, openLabel).Height(26).Disabled(!canOpen).OnClick(a.submitRemoteOpen)
			})
		})
	})
}
