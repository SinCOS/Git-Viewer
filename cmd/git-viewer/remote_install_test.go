package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"gitviewer/internal/application"
	"gitviewer/internal/ui/local"
)

func TestRemoteOpenedMakesGUIWindowEligibleForCLIInstall(t *testing.T) {
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	service := application.LocalService{}
	service.Remote.SSHPath = fakeSSH
	m := newWindowManager(service, false)
	state := &windowState{app: &local.App{SSHPath: "/root"}}
	// A window opened locally has no remote target until the GUI callback runs.
	m.windows[nil] = state
	m.order = []*mygo.Window{nil}
	if m.mostRecent("n1") != nil {
		t.Fatal("local window unexpectedly matches a remote host")
	}
	m.recordRemoteOpened(state, "n1")
	if m.matchingRemote("n1", "/root/project") != state {
		t.Fatal("GUI remote target was not recorded for window routing")
	}
	prompted := false
	state.app.Confirm = func(title, _ string) bool {
		prompted = title == "Install remote code command"
		return false
	}
	m.ensureRemoteCLI("n1")
	if !prompted {
		t.Fatal("opening a remote folder from the GUI skipped the install prompt")
	}
	state.app.SSHPath = "/srv/project"
	m.recordRemoteOpened(state, "other-host")
	if m.mostRecent("n1") != nil || m.matchingRemote("other-host", "/srv/project/file") != state {
		t.Fatal("switching remote targets left stale window routing")
	}
}
