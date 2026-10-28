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
		body, changes, diff := box("Workspace body"), box("Local Changes"), box("Commit diff pane")
		form, source := box("Commit form scroll"), box("SOURCE CONTROL")
		if source.Y < body.Y || source.Y >= form.Y || form.Y+form.H > changes.Y+1 {
			t.Fatalf("source control form is not stacked above changes: source=%+v form=%+v changes=%+v", source, form, changes)
		}
		if diff.Y != body.Y || changes.X >= diff.X {
			t.Fatalf("commit workspace is not split into side bar and diff: changes=%+v diff=%+v", changes, diff)
		}
	}
}
