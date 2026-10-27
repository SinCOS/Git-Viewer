package local_test

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"testing"
)

func TestMultiRepositoryCommitAlignment(t *testing.T) {
	for _, height := range []int{550, 850} {
		a, _ := setup(t)
		a.Repository.Repository.RepositoryID = "layout"
		a.WorkspaceRepositories = []domain.RepositoryContext{{Repository: domain.Repository{DisplayName: "first"}}, {Repository: domain.Repository{DisplayName: "second"}}}
		a.Entries = []domain.StatusEntry{{Change: domain.Change{Path: "file", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"}}
		tt := ui.NewTester(a.View, 1000, height)
		if tt.HasText("Workspace repositories") || tt.HasText("Refresh workspace statuses") {
			t.Fatal("workspace switcher leaked into body")
		}
		box := func(label string) ui.Rect {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("missing %s", label)
			}
			return r
		}
		body, changes, diff, form := box("Workspace body"), box("Local Changes"), box("Commit diff pane"), box("Commit form scroll")
		header, group := box("Commit diff header"), box("Changes controls")
		if header.Y != group.Y || header.H != group.H {
			t.Fatalf("headers misaligned: %+v %+v", header, group)
		}
		if changes.Y != diff.Y || changes.Y != body.Y || form.Y+form.H > diff.Y+diff.H+1 {
			t.Fatal("workspace commit panels do not align")
		}
	}
}
