package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
)

func TestOpenRemoteDialogSeedsHostsAndRecentTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ssh_config")
	if err := os.WriteFile(configPath, []byte("Host dev-box\n  HostName 192.0.2.10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(application.LocalService{
		Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")},
		Remote: ssh.Executor{ConfigPath: configPath},
	})
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Locations = []domain.SavedLocation{
		{Kind: "ssh-folder", Host: "dev-box", Path: "/srv/project"},
	}

	a.OpenRemoteDialog()

	if !a.RemoteOpen {
		t.Fatal("remote dialog did not open")
	}
	if len(a.SSHHosts) != 1 || a.SSHHosts[0] != "dev-box" {
		t.Fatalf("host suggestions = %v", a.SSHHosts)
	}
	if a.RemoteHostInput != "dev-box" || a.RemotePathInput != "/srv/project" {
		t.Fatalf("seeded target = %q %q", a.RemoteHostInput, a.RemotePathInput)
	}
}

func TestRemoteTargetValidationAndKind(t *testing.T) {
	tests := []struct {
		host, path string
		want       string
	}{
		{"dev-box", "/srv/project", ""},
		{"bad host", "/srv/project", "Enter an SSH Host alias"},
		{"dev-box", "relative/path", "Enter an absolute remote path"},
	}
	for _, tt := range tests {
		got := validateRemoteTarget(tt.host, tt.path)
		if tt.want == "" && got != "" {
			t.Fatalf("validateRemoteTarget(%q, %q) = %q", tt.host, tt.path, got)
		}
		if tt.want != "" && !strings.Contains(got, tt.want) {
			t.Fatalf("validateRemoteTarget(%q, %q) = %q, want %q", tt.host, tt.path, got, tt.want)
		}
	}
	if remotePathKind("/srv/team.code-workspace") != "workspace" {
		t.Fatal("workspace path not detected")
	}
	if remotePathKind("/srv/project") != "folder" {
		t.Fatal("folder path not detected")
	}
}

func TestRemoteOpenViewUsesWorkspaceAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &App{
		RemoteOpen:      true,
		RemoteHostInput: "dev-box",
		RemotePathInput: "/srv/team.code-workspace",
		SSHHosts:        []string{"dev-box"},
		remoteBrowser:   &remoteFolderBrowser{host: "dev-box", path: "/srv", client: &fakeRemoteFileSystem{}, ctx: ctx, cancel: cancel},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.remoteOpenView(c) }, 900, 700)
	tt.Frame()
	for _, label := range []string{"SSH: dev-box", "Open Remote Folder", "Remote path", "Open workspace"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q in %v", label, tt.Texts())
		}
	}
}

func TestRemoteLoginDoesNotRequireFolder(t *testing.T) {
	a := &App{RemoteOpen: true, remoteChooseHost: true, RemoteHostInput: "n1"}
	tt := ui.NewTester(func(c *ui.Context) { a.remoteOpenView(c) }, 900, 700)
	tt.Frame()
	if !tt.HasText("Connect to Host") || !tt.HasText("Connect") || tt.HasText("Remote path") {
		t.Fatal(tt.Texts())
	}
}

func TestRemoteBrowserNavigatesAndSwitchesWithoutLocalChooser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	files := &fakeRemoteFileSystem{dirs: map[string][]domain.RemoteFileInfo{
		"/root":        {{Name: "source", Path: "/root/source", Dir: true}},
		"/root/source": {{Name: "src", Path: "/root/source/src", Dir: true}},
	}}
	a := newRemoteEditorApp(files)
	a.SSHHost, a.SSHPath = "dev", "/root"
	a.remoteBrowser = &remoteFolderBrowser{host: "dev", path: "/root", client: files, ctx: ctx, cancel: cancel}
	queue := make(chan func(), 4)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.ChooseFolder = func() (string, error) { t.Error("remote switch invoked local chooser"); return "", nil }
	a.OpenFolderDialog()
	if !a.RemoteOpen || a.remoteChooseHost || a.RemoteHostInput != "dev" {
		t.Fatal("did not reuse remote login")
	}
	a.browseRemoteFolder("/root/source")
	select {
	case apply := <-queue:
		apply()
	case <-time.After(time.Second):
		t.Fatal("browse timed out")
	}
	if a.RemotePathInput != "/root/source" || len(a.remoteBrowser.entries) != 1 {
		t.Fatal("navigation failed")
	}
	a.browseRemoteFolder("/missing")
	select {
	case apply := <-queue:
		apply()
	case <-time.After(time.Second):
		t.Fatal("browse timed out")
	}
	if a.remoteBrowser.path != "/root/source" || a.RemoteOpenMessage == "" {
		t.Fatal("failed navigation lost current location")
	}
	a.RemoteOpen = false
	a.SSHPath = "/root/source"
	a.OpenFolderDialog()
	if a.RemotePathInput != "/root/source" || a.remoteBrowser.client != files {
		t.Fatal("switch discarded authenticated session")
	}
}

func TestRemoteBrowserFailureReturnsToLogin(t *testing.T) {
	a := New(application.LocalService{Remote: ssh.Executor{SSHPath: "/usr/bin/false"}})
	queue := make(chan func(), 4)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.RemoteHostInput = "n1"
	a.connectRemoteHost()
	select {
	case apply := <-queue:
		apply()
	case <-time.After(3 * time.Second):
		t.Fatal("login failure timed out")
	}
	if !a.RemoteOpen || !a.remoteChooseHost || a.RemoteOpenMessage == "" || a.remoteBrowser != nil {
		t.Fatal("failed login did not return to host form")
	}
}

func TestRemoteFolderSwitchPreservesUnsavedBuffers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const file = "/srv/project/main.go"
	files := &fakeRemoteFileSystem{
		info:  map[string]domain.RemoteFileInfo{file: {Path: file, Size: 3, Mode: 0o600}},
		files: map[string][]byte{file: []byte("old")},
	}
	a := newRemoteEditorApp(files)
	if err := a.openRemoteEditorFile(remoteFileRef("dev", "/srv/project", file)); err != nil {
		t.Fatal(err)
	}
	a.ActiveEditorBuffer().Set("unsaved")
	a.remoteBrowser = &remoteFolderBrowser{host: "dev", client: files, ctx: ctx, cancel: cancel}
	a.RemoteOpen, a.RemoteHostInput, a.RemotePathInput = true, "dev", "/other"
	a.submitRemoteOpen()
	if !a.RemoteOpen || !strings.Contains(a.RemoteOpenMessage, "unsaved") || a.ActiveEditorBuffer().String() != "unsaved" {
		t.Fatal("switching folders discarded unsaved edits")
	}
}
