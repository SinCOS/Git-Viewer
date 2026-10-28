package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	a := &App{
		RemoteOpen:      true,
		RemoteHostInput: "dev-box",
		RemotePathInput: "/srv/team.code-workspace",
		SSHHosts:        []string{"dev-box"},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.remoteOpenView(c) }, 900, 700)
	tt.Frame()
	for _, label := range []string{"Open Remote", "SSH host", "Remote path", "Open workspace"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q in %v", label, tt.Texts())
		}
	}
}
