package local_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestGroupedTreeSelectionMapsToGitEntries(t *testing.T) {
	dir := testgit.Repo(t)
	name := filepath.Join(dir, "internal", "a.go")
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, name, "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, name, "staged\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Write(t, name, "unstaged\n")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.ChangesTree = true
	tt := ui.NewTester(a.View, 1400, 850)
	// Both scopes contain the same directory and filename. Their virtual keys
	// must stay distinct, and heading rows must never be used as Git entries.
	if err := tt.Click("a.go"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if a.Selected != 1 || a.Preview.Scope != domain.WorktreeDiff {
		t.Fatal(a.Selected, a.Preview)
	}
	for i := 0; i < 2; i++ {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
	}
	settle(t, a, q)
	tt.Frame()
	if a.Selected != 0 || a.Preview.Scope != domain.StagedDiff || a.Preview.Path != "internal/a.go" {
		t.Fatal(a.Selected, a.Preview)
	}
	if err := tt.Click("List"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Selected != 0 {
		t.Fatal("changing display mode changed the selected Git entry")
	}
}
