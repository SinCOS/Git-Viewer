package local

import (
	posixpath "path"
	"strings"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
)

// OpenRemoteDialog shows the in-app remote target form. Key/agent
// authentication stays in OpenSSH; password and keyboard-interactive prompts
// are routed back to this window by the askpass broker.
func (a *App) OpenRemoteDialog() {
	if a.closed || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	if b := a.remoteBrowser; b != nil && b.client == nil {
		a.closeRemoteBrowser()
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
	a.remoteChooseHost = true
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
	if a.remoteBrowser == nil || a.remoteBrowser.client == nil || a.remoteBrowser.host != host {
		a.RemoteOpenMessage = "Connect to the host first"
		return
	}
	for _, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			a.RemoteOpenMessage = "Save or close unsaved files before switching folders"
			return
		}
	}
	a.RemoteOpen = false
	a.RemoteOpenMessage = ""
	if remotePathKind(remotePath) == "workspace" {
		a.openRemoteWorkspace(host, remotePath, true)
		return
	}
	a.openRemoteFolder(host, remotePath, true)
}

// submitRemoteWorkspace opens the typed remote path as a workspace file even
// when it lacks the .code-workspace suffix, so the picker's Open workspace
// action always builds a multi-root workspace rather than a single root.
func (a *App) submitRemoteWorkspace() {
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
	if a.dirtyEditorTabs() {
		a.RemoteOpenMessage = "Save or close unsaved files before switching workspaces"
		return
	}
	a.RemoteOpen = false
	a.RemoteOpenMessage = ""
	a.openRemoteWorkspace(host, remotePath, true)
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
