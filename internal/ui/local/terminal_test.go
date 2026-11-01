package local

import (
	"bytes"
	"path/filepath"
	"testing"

	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
)

func TestTerminalTabsCreateSwitchAndClose(t *testing.T) {
	a := &App{Settings: defaultAppSettings(), TerminalActive: -1}
	a.NewLocalTerminal()
	a.NewLocalTerminal()
	if len(a.TerminalTabs) != 2 || a.TerminalActive != 1 || a.TerminalError == "" {
		t.Fatalf("tabs=%d active=%d error=%q", len(a.TerminalTabs), a.TerminalActive, a.TerminalError)
	}
	a.PreviousTerminalTab()
	if a.TerminalActive != 0 {
		t.Fatalf("previous active=%d", a.TerminalActive)
	}
	a.NextTerminalTab()
	if a.TerminalActive != 1 {
		t.Fatalf("next active=%d", a.TerminalActive)
	}
	a.CloseActiveTerminal()
	if len(a.TerminalTabs) != 1 || a.TerminalActive != 0 {
		t.Fatalf("close tabs=%d active=%d", len(a.TerminalTabs), a.TerminalActive)
	}
}

func TestRemoteTerminalUsesStrictSSHConfig(t *testing.T) {
	a := &App{Service: application.LocalService{Remote: ssh.Executor{
		SSHPath: "/opt/ssh", ConfigPath: "/tmp/ssh_config", KnownHostsFile: "/tmp/known_hosts",
	}}}
	command, err := a.remoteTerminalCommand("dev-box")
	if err != nil {
		t.Fatal(err)
	}
	// UI-thread preparation must not launch SSH; the background worker
	// replaces these with the unified TerminalSession arguments.
	if len(command) != 2 || command[0] != "/opt/ssh" || command[1] != "dev-box" {
		t.Fatalf("command = %v", command)
	}
	if _, err := a.remoteTerminalCommand("-oProxyCommand=bad"); err == nil {
		t.Fatal("invalid host was accepted")
	}
}

func TestTerminalSessionMetadataAndSnapshotRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	service := application.LocalService{Recent: storage.RecentRepositories{Path: path}}
	a := &App{Service: service, Settings: defaultAppSettings(), TerminalActive: -1}
	a.newTerminal("local", "", "/tmp", "Local", []byte("local screen"))
	a.newTerminal("remote", "dev-box", "", "dev-box", []byte("remote screen"))
	a.TerminalActive = 1
	a.saveTerminalSessions()

	b := &App{Service: service, Settings: defaultAppSettings(), TerminalActive: -1}
	b.loadTerminalSessions()
	if len(b.TerminalTabs) != 2 || b.TerminalActive != 1 {
		t.Fatalf("restored tabs=%d active=%d", len(b.TerminalTabs), b.TerminalActive)
	}
	if b.TerminalTabs[0].Kind != "local" || b.TerminalTabs[1].Host != "dev-box" ||
		string(b.TerminalTabs[1].Snapshot) != "remote screen" {
		t.Fatalf("restored sessions = %+v", b.TerminalTabs)
	}
}

func TestTerminalSnapshotCapKeepsEscapeBoundary(t *testing.T) {
	raw := append(bytes.Repeat([]byte("x"), maxTerminalSnapshot+100), []byte("\x1b[31mrestored")...)
	got := boundedSnapshot(raw)
	if len(got) > maxTerminalSnapshot || !bytes.HasPrefix(got, []byte("\x1b[31m")) {
		t.Fatalf("bounded snapshot length=%d prefix=%q", len(got), got[:min(8, len(got))])
	}
}
