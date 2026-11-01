package local_test

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/ui/local"
)

func TestNativeMenuDialogsReturnWhileNativeCallWaits(t *testing.T) {
	cases := []struct {
		name    string
		trigger func(*local.App)
		install func(*local.App, func())
	}{
		{"folder", func(a *local.App) { a.OpenFolderDialog() }, func(a *local.App, wait func()) {
			a.ChooseFolder = func() (string, error) { wait(); return "", nil }
		}},
		{"workspace", func(a *local.App) { a.ChooseWorkspaceFile() }, func(a *local.App, wait func()) {
			path := filepath.Join(filepath.Dir(a.Service.Recent.Path), "late.code-workspace")
			a.ChooseWorkspace = func() (string, error) { wait(); return path, nil }
		}},
		{"save-as", func(a *local.App) { a.SaveWorkspace(true) }, func(a *local.App, wait func()) {
			path := filepath.Join(filepath.Dir(a.Service.Recent.Path), "late.code-workspace")
			a.ChooseWorkspaceSave = func() (string, error) { wait(); return path, nil }
		}},
		{"confirmation", func(a *local.App) {
			a.BranchInput = "next"
			a.RunWrite(domain.SwitchBranch)
		}, func(a *local.App, wait func()) {
			a.Confirm = func(string, string) bool { wait(); return true }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, queue := setup(t)
			a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: t.TempDir(), DisplayName: "Probe"}}
			started, returned, release := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			var calls atomic.Int32
			tc.install(a, func() { calls.Add(1); started <- struct{}{}; <-release; close(returned) })
			triggered := make(chan struct{})
			go func() {
				tc.trigger(a)
				close(triggered)
			}()
			select {
			case <-triggered:
			case <-time.After(time.Second):
				unblock()
				<-triggered
				t.Fatal("menu action blocked while waiting for dialog")
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("dialog was not called")
			}
			if !a.DialogOpen {
				t.Fatal("dialog wait was not tracked")
			}
			tc.trigger(a)
			if calls.Load() != 1 {
				t.Fatal("duplicate dialog")
			}
			a.Close()
			stopped := make(chan struct{})
			go func() { a.Wait(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(time.Second):
				unblock()
				<-stopped
				t.Fatal("shutdown waited for the OS dialog callback")
			}
			unblock()
			<-returned
			for len(queue) > 0 {
				(<-queue)()
			}
			if a.Loading || a.Writing || a.DialogOpen || a.Workspace.FilePath != "" {
				t.Fatal("late dialog result started work after close")
			}
		})
	}
}

func TestOpenWorkspaceEntryReachesRemoteHost(t *testing.T) {
	a, _ := setup(t)
	a.SSHHost = "dev-box"
	a.SSHHosts = []string{"dev-box"}
	dialog := 0
	a.ChooseWorkspace = func() (string, error) { dialog++; return "", nil }
	a.ChooseWorkspaceFile()
	if dialog != 0 {
		t.Fatal("a remote session must not open the local workspace sheet")
	}
	// Connecting reports its result through Dispatch; an unconfigured host
	// cannot resolve here, so the started connect is what this checks.
	if !strings.HasPrefix(a.Message, "Connecting to") {
		t.Fatal("workspace entry did not reach the remote host", a.Message)
	}
	if a.RemoteHostInput != "dev-box" {
		t.Fatal("workspace entry lost the host", a.RemoteHostInput)
	}
}

func TestLocalWorkspaceEntryKeepsNativeSheet(t *testing.T) {
	a, queue := setup(t)
	path := filepath.Join(t.TempDir(), "local.code-workspace")
	a.ChooseWorkspace = func() (string, error) { return path, nil }
	a.ChooseWorkspaceFile()
	if !a.DialogOpen {
		t.Fatal("local workspace sheet did not open")
	}
	for a.Loading || a.DialogOpen {
		select {
		case apply := <-queue:
			apply()
		case <-time.After(5 * time.Second):
			t.Fatal("workspace dialog never returned")
		}
	}
	if a.RemoteOpen {
		t.Fatal("a local workspace must not open the remote panel")
	}
}
