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
		body := box("Workspace body")
		toolbar := box("Main toolbar")
		if toolbar.Y >= body.Y {
			t.Fatalf("toolbar must sit above the body: toolbar=%+v body=%+v", toolbar, body)
		}
		for _, label := range []string{"Commit", "Git Log", "Worktree"} {
			mode := box(label)
			if mode.Y >= body.Y || mode.X >= toolbar.X+toolbar.W {
				t.Fatalf("%s must be primary navigation in the toolbar: %+v", label, mode)
			}
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
		if changes.X < body.X || changes.X >= diff.X || form.Y <= changes.Y || form.Y <= diff.Y || form.X >= diff.X {
			t.Fatalf("commit layout at width %v: body=%+v changes=%+v diff=%+v form=%+v", width, body, changes, diff, form)
		}
		if err := tt.Click("Git Log"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		workspace := box("Git history workspace")
		branches := box("Branch navigation")
		if branches.X < body.X || branches.W < 160 || workspace.X < branches.X+branches.W || workspace.X+workspace.W != float32(width) || tt.HasText("Commit message") || tt.HasText("Changed Files") {
			t.Fatalf("branch tree and history must share the body with details hidden: branches=%+v history=%+v", branches, workspace)
		}
		for _, label := range []string{"Search branches", "All Branches", "HEAD (Current Branch)", "Search commits"} {
			box(label)
		}
		search := box("Search branches")
		if search.W > 224 || search.X < branches.X || search.X+search.W > branches.X+branches.W {
			t.Fatalf("branch search must stay compact and inside its panel: search=%+v panel=%+v", search, branches)
		}
		head, local := box("HEAD (Current Branch)"), box("Toggle branch folder Local")
		if head.Y+head.H > local.Y || head.W != branches.W || head.H != 30 {
			t.Fatalf("HEAD must be a full-width navigation row above Local: head=%+v local=%+v", head, local)
		}
		if err := tt.Click("Commit details"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		files := box("Changed Files")
		if files.Y <= box("Git history workspace").Y {
			t.Fatal("details must open below history")
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
