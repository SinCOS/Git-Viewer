package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func TestNativeShell(t *testing.T) {
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 4)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	tt := ui.NewTester(a.View, 1400, 850)
	if !tt.HasText("Git Viewer") || !tt.HasText("Open a repository to view changes") {
		t.Fatal(tt.Texts())
	}
	chosen := false
	a.ChooseDirectory = func() (string, error) { chosen = true; return "", nil }
	if err := tt.Click("Choose repository…"); err != nil {
		t.Fatal(err)
	}
	select {
	case apply := <-queue:
		apply()
	case <-time.After(time.Second):
		t.Fatal("directory chooser did not return")
	}
	if !chosen {
		t.Fatal("directory chooser not invoked")
	}
}
