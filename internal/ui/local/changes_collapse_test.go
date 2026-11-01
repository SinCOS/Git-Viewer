package local

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestChangesDisclosureGroupsDirectoriesAndPanel(t *testing.T) {
	a := checkboxApp(t)
	a.ChangesTree = true
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "src/staged.go", IndexStatus: "M", WorktreeStatus: " "}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "src/unstaged.go", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
	}
	a.Selected = 1
	tt := ui.NewTester(a.View, 1400, 850)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("Toggle Changes")
	if tt.HasText(`Toggle directory Unstaged "src"`) || !tt.HasText("staged.go") {
		t.Fatal(tt.Texts())
	}
	click("Toggle Changes")
	if !tt.HasText("unstaged.go") {
		t.Fatal("group did not reopen", tt.Texts())
	}
	click(`Toggle directory Unstaged "src"`)
	if len(a.changeRows()) != 5 {
		t.Fatal("directory collapse leaked to other group", tt.Texts())
	}
	click("Toggle Staged Changes")
	click("Toggle Staged Changes")
	if len(a.changeRows()) != 5 {
		t.Fatal("unrelated group reset directory fold state")
	}
	click(`Toggle directory Unstaged "src"`)
	if !tt.HasText("unstaged.go") {
		t.Fatal("directory did not reopen")
	}
	if a.Selected != 1 || a.Loading || a.DialogOpen || a.Entries[1].Scope != domain.WorktreeDiff {
		t.Fatal("folding changed Git state or selected entry", a.Selected, a.Message)
	}
}

func TestChangesTreeShowsRootFilesAndGroupsDirectories(t *testing.T) {
	a := checkboxApp(t)
	a.ChangesTree = true
	for _, name := range []string{"a.go", "src/b.go", "z.go"} {
		a.Entries = append(a.Entries, domain.StatusEntry{Change: domain.Change{Path: name}, Scope: domain.WorktreeDiff, Group: "Unstaged"})
	}
	roots := 0
	for _, row := range a.changeRows() {
		if row.dir && row.dirPath == "." {
			roots++
		}
	}
	if roots != 0 {
		t.Fatalf("root directory appears %d times", roots)
	}
	a.toggleChanges("dir\x00Unstaged\x00src")
	for _, row := range a.changeRows() {
		if row.entry == 1 {
			t.Fatal("folded directory file is visible")
		}
	}
}

func TestChangesTreeIncludesUntrackedFiles(t *testing.T) {
	a := checkboxApp(t)
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "src/one.go"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
		{Change: domain.Change{Path: "src/new.go", Untracked: true}, Scope: domain.WorktreeDiff, Group: "Untracked"},
		{Change: domain.Change{Path: "src/staged.go"}, Scope: domain.StagedDiff, Group: "Staged"},
	}
	rows := a.changeRows()
	if len(rows) != 7 || rows[0].group != "Staged" || !rows[1].dir || rows[2].entry != 2 || rows[3].group != "Unstaged" || rows[3].count != 2 || !rows[4].dir || rows[4].count != 2 {
		t.Fatalf("unexpected grouped tree: %+v", rows)
	}
}
