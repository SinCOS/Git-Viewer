package local_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/ui/local"
)

func TestDialogButtonsReturnWhileNativeCallWaits(t *testing.T) {
	cases := []struct {
		name, button string
		install      func(*local.App, func())
	}{
		{"directory", "Choose repository…", func(a *local.App, wait func()) {
			a.ChooseDirectory = func() (string, error) { wait(); return "", nil }
		}},
		{"workspace", "Open workspace", func(a *local.App, wait func()) {
			path := filepath.Join(filepath.Dir(a.Service.Recent.Path), "late.code-workspace")
			a.ChooseWorkspace = func() (string, error) { wait(); return path, nil }
		}},
		{"save-as", "New / Save workspace as…", func(a *local.App, wait func()) {
			path := filepath.Join(filepath.Dir(a.Service.Recent.Path), "late.code-workspace")
			a.ChooseWorkspaceSave = func() (string, error) { wait(); return path, nil }
		}},
		{"confirmation", "Fetch", func(a *local.App, wait func()) {
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
			tt := ui.NewTester(a.View, 1400, 850)
			clicked := make(chan error, 1)
			go func() { clicked <- tt.Click(tc.button) }()
			select {
			case err := <-clicked:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				unblock()
				<-clicked
				t.Fatal("button blocked while waiting for dialog")
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("dialog was not called")
			}
			if !a.DialogOpen {
				t.Fatal("dialog wait was not tracked")
			}
			tt.Frame() // A pending chooser must allow another UI frame.
			if err := tt.Click(tc.button); err != nil {
				t.Fatal(err)
			}
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
