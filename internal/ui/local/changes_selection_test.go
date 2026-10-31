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
	first := a.Selected
	if first != 0 && first != 1 {
		t.Fatal(a.Selected, a.Preview)
	}
	firstScope := a.Preview.Scope
	for i := 0; i < 8 && a.Selected == first; i++ {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		settle(t, a, q)
		tt.Frame()
	}
	if a.Selected == first || a.Preview.Path != "internal/a.go" {
		t.Fatal(a.Selected, a.Preview)
	}
	if firstScope == a.Preview.Scope || firstScope != domain.WorktreeDiff && firstScope != domain.StagedDiff {
		t.Fatalf("did not navigate between staged and worktree entries: %v then %v", firstScope, a.Preview.Scope)
	}
	selectedAfterNavigation := a.Selected
	if err := tt.Click("List"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Selected != selectedAfterNavigation {
		t.Fatal("changing display mode changed the selected Git entry")
	}
}
