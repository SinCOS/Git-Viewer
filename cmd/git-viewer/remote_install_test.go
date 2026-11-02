package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/ui/local"
)

func TestRemoteHelpersConfiguredAfterExecutor(t *testing.T) {
	service := application.LocalService{}
	service.Remote.AllowPassword = true
	service.Remote.ConfigPath = "/isolated/ssh-config"
	service.Remote.Connections = &ssh.Connections{}
	m := newWindowManager(service, false)
	m.configureRemoteHelpers("auto")
	t.Cleanup(m.closeRemote)
	pool := m.service.Remote.Helpers
	if pool == nil || !pool.Executor.AllowPassword || pool.Executor.ConfigPath != service.Remote.ConfigPath || pool.Executor.Connections != service.Remote.Connections {
		t.Fatal("helper pool lost executor settings")
	}
	off := newWindowManager(application.LocalService{}, false)
	off.configureRemoteHelpers("off")
	if off.service.Remote.Helpers != nil {
		t.Fatal("off mode created a helper pool")
	}
}

func TestRemoteHelperPrewarmAndShutdown(t *testing.T) {
	service := application.LocalService{}
	service.Remote.Connections = &ssh.Connections{}
	m := newWindowManager(service, false)
	m.configureRemoteHelpers("auto")
	t.Cleanup(m.closeRemote)
	started := make(chan string, 1)
	stopped := make(chan struct{})
	m.service.Remote.Helpers.Ops = func(ctx context.Context, _ ssh.Executor, host string) (ssh.HelperRemoteOps, error) {
		started <- host
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}
	// Local targets and invalid hosts must not initiate installation.
	m.prewarmRemoteHelper("")
	m.prewarmRemoteHelper("-unsafe")
	select {
	case host := <-started:
		t.Fatalf("local/invalid target warmed %q", host)
	default:
	}
	opened := make(chan struct{})
	go func() {
		m.recordRemoteOpened(&windowState{}, "dev-box", "/srv/project")
		close(opened)
	}()
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("remote-open callback waited for helper installation")
	}
	select {
	case host := <-started:
		if host != "dev-box" {
			t.Fatalf("prewarmed %q", host)
		}
	case <-time.After(time.Second):
		t.Fatal("remote-open callback did not prewarm helper")
	}
	m.closeAll()
	m.closeRemote()
	m.closeRemote()
	select {
	case <-stopped:
	default:
		t.Fatal("shutdown left helper install worker running")
	}
	if _, err := m.service.Remote.Helpers.Acquire(context.Background(), "dev-box"); err == nil {
		t.Fatal("shutdown left helper pool available")
	}
	if _, err := m.service.Remote.Command(context.Background(), "dev-box", ssh.SharedSession, []string{"true"}, ""); err == nil {
		t.Fatal("shutdown left SSH connection pool available")
	} else {
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
			t.Fatalf("closed connections error = %v", err)
		}
	}
	m.prewarmRemoteHelper("dev-box")
}

func TestHelperResourcesResolver(t *testing.T) {
	t.Setenv("GITVIEWER_HELPER_PATH", "")
	oldResources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathResources, oldResources) })
	root := t.TempDir()
	t.Chdir(root)
	packaged := filepath.Join(root, "Contents", "Resources")
	mygo.App.SetPath(mygo.PathResources, packaged)
	if ssh.HelperResourcesDir == nil {
		t.Fatal("native resource resolver was not installed")
	}
	if got, err := ssh.HelperResourcesDir(); err != nil || got != packaged {
		t.Fatalf("resources = %q, %v", got, err)
	}
	rel := filepath.Join("helper", "linux-arm64", "gitviewer-helper")
	development := filepath.Join(root, "resources", rel)
	bundled := filepath.Join(packaged, rel)
	for _, file := range []string{development, bundled} {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("test helper"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := ssh.HelperLocalPath("linux", "arm64"); err != nil || got != bundled {
		t.Fatalf("packaged helper = %q, %v", got, err)
	}
	if err := os.Remove(bundled); err != nil {
		t.Fatal(err)
	}
	if got, err := ssh.HelperLocalPath("linux", "arm64"); err != nil || got != development {
		t.Fatalf("development helper = %q, %v", got, err)
	}
	if err := os.Remove(development); err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.HelperLocalPath("linux", "arm64"); err == nil {
		t.Fatal("missing helper resources did not report unavailable")
	}
}

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
	m.recordRemoteOpened(state, "n1", state.app.SSHPath)
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
	m.recordRemoteOpened(state, "other-host", state.app.SSHPath)
	if m.mostRecent("n1") != nil || m.matchingRemote("other-host", "/srv/project/file") != state {
		t.Fatal("switching remote targets left stale window routing")
	}
}
