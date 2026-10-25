package local_test

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func TestModeRailSwitchesPanes(t *testing.T) {
	a := local.New(application.LocalService{})
	tt := ui.NewTester(a.View, 1400, 850)
	if a.Mode != "log" || a.Pane != "" {
		t.Fatal(a.Mode, a.Pane)
	}
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Pane != "Status" || !tt.HasText("Status") {
		t.Fatal(a.Pane, tt.Texts())
	}
	if err := tt.Click("Worktree"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Pane != "Branches" {
		t.Fatal(a.Pane)
	}
	if err := tt.Click("Git Log"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Pane != "" || !tt.HasText("No history loaded yet") {
		t.Fatal(a.Pane, tt.Texts())
	}
	// Toolbar buttons keep working alongside the rail.
	a.Pane = "Status"
	tt.Frame()
	if a.Mode != "commit" {
		t.Fatal(a.Mode)
	}
}
func TestPrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(dir, "recent.json")}})
	a.SidebarWidth = 300
	a.StatusWidth = 420
	a.Mode = "commit"
	a.Close() // saves prefs next to the recent path
	second := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(dir, "recent.json")}})
	if second.PrefsPathForTest() != filepath.Join(dir, "ui-prefs.json") {
		t.Fatal(second.PrefsPathForTest())
	}
	if second.SidebarWidth != 300 || second.StatusWidth != 420 || second.Mode != "commit" || second.Pane != "Status" {
		t.Fatal(second.SidebarWidth, second.StatusWidth, second.Mode, second.Pane)
	}
	second.Close()
}
