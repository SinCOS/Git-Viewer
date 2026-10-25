package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func TestNativeShell(t *testing.T) {
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	tt := ui.NewTester(a.View, 1400, 850)
	if !tt.HasText("Git Viewer · Local") || !tt.HasText("Open a repository to view changes") {
		t.Fatal(tt.Texts())
	}
	chosen := false
	a.ChooseDirectory = func() (string, error) { chosen = true; return "", nil }
	if err := tt.Click("Choose repository…"); err != nil {
		t.Fatal(err)
	}
	if !chosen {
		t.Fatal("directory chooser not invoked")
	}
}
