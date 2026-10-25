package local_test

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestWorkspacePanelPositions(t *testing.T) {
	for _, width := range []int{1100, 1400} {
		a, _ := setup(t)
		tt := ui.NewTester(a.View, width, 850)
		box := func(label string) ui.Rect {
			t.Helper()
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("missing %q", label)
			}
			return r
		}
		body, navigation := box("Workspace body"), box("Mode navigation")
		if navigation.X != body.X || navigation.Y != body.Y || navigation.H != body.H || navigation.W < 180 {
			t.Fatalf("navigation must fill the left edge: body=%+v navigation=%+v", body, navigation)
		}
		for _, label := range []string{"Fetch", "Pull (fast-forward)", "Push"} {
			button := box(label)
			if button.Y >= body.Y {
				t.Fatalf("%s must remain visible in the toolbar: %+v", label, button)
			}
		}
		if tt.HasText("Commit object ID") || tt.HasText("View commit diff") {
			t.Fatal("commit object controls leaked into the workspace")
		}
		for _, pair := range [][2]string{{"List", "List view icon"}, {"Tree", "Tree view icon"}, {"Refresh changes", "Refresh changes icon"}, {"Fetch", "Fetch icon"}, {"Pull (fast-forward)", "Pull (fast-forward only) icon"}, {"Push", "Push icon"}} {
			button, icon := box(pair[0]), box(pair[1])
			if button.W != 28 || button.H != 28 || icon.W != 16 || icon.H != 16 || icon.X != button.X+6 || icon.Y != button.Y+6 {
				t.Fatalf("icon must be centered inside its button: button=%+v icon=%+v", button, icon)
			}
		}
		changes, diff, form := box("Local Changes"), box("Diff Preview"), box("Commit message")
		rail := box("Git Log")
		if rail.X >= changes.X || changes.X >= diff.X || form.Y <= changes.Y || form.Y <= diff.Y || form.X >= diff.X {
			t.Fatalf("commit layout at width %v: rail=%+v changes=%+v diff=%+v form=%+v", width, rail, changes, diff, form)
		}
		if err := tt.Click("Git Log"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		workspace := box("Git history workspace")
		branches := box("Branch navigation")
		if branches.X < navigation.X+navigation.W || branches.W < 160 || workspace.X < branches.X+branches.W || workspace.X+workspace.W != float32(width) || tt.HasText("Commit message") || tt.HasText("Changed Files") {
			t.Fatalf("branch tree and history must share the body with details hidden: branches=%+v history=%+v", branches, workspace)
		}
		for _, label := range []string{"Search branches", "All Branches", "HEAD (Current Branch)", "Search commits"} {
			box(label)
		}
		if err := tt.Click("Commit details"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		files := box("Changed Files")
		if files.X <= box("Git history workspace").X {
			t.Fatal("details must open beside history")
		}
		if err := tt.Click("Worktree"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		list, details := box("Worktree List"), box("Worktree Details")
		if list.X >= details.X || tt.HasText("Diff Preview") || tt.HasText("Commit message") {
			t.Fatalf("worktree layout: list=%+v details=%+v", list, details)
		}
	}
}
