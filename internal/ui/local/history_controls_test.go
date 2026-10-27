package local_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestHistoryControlsPreserveListState(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "control base")
	tree := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD^{tree}")))
	tip := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	for i := 0; i < 119; i++ {
		tip = strings.TrimSpace(string(testgit.Run(t, dir, "commit-tree", tree, "-p", tip, "-m", fmt.Sprintf("control commit %02d", i))))
	}
	testgit.Run(t, dir, "update-ref", "refs/heads/main", tip)
	testgit.Run(t, dir, "branch", "feat/side")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	a.LoadHistory(true)
	settle(t, a, q)
	tt.Frame()
	a.HistoryList.Columns.Widths = map[string]float32{"author": 95}
	a.HistoryList.ScrollTo(35, ui.Start)
	tt.Frame()
	first, _ := a.HistoryList.Visible()
	subject := a.GraphRows[40].Commit.Subject
	if err := tt.Click(subject); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	selectedOID := a.CommitDetails.Commit.OID
	if selectedOID == "" {
		t.Fatal("no selected commit")
	}
	assertStable := func() {
		t.Helper()
		tt.Frame()
		if a.Loading || len(q) != 0 || len(a.GraphRows) != 120 || a.CommitDetails.Commit.OID != selectedOID || a.GraphRows[a.HistorySelected].Commit.OID != selectedOID || a.HistoryList.Columns.Widths["author"] != 95 {
			t.Fatal("control reset history state", a.Message, a.HistorySelected)
		}
		if current, _ := a.HistoryList.Visible(); current != first {
			t.Fatal("control lost scroll anchor", first, current)
		}
		if _, ok := tt.Find("Commit history list"); !ok {
			t.Fatal("history table disappeared")
		}
	}
	for _, folder := range []string{"Local/feat", "Local/feat", "Toggle section Remote", "Toggle section Remote"} {
		if err := tt.Click("Toggle branch folder " + folder); err != nil {
			if folder == "Local/feat" {
				t.Fatal(err)
			}
			if err := tt.Click(folder); err != nil {
				t.Fatal(err)
			}
		}
		assertStable()
	}
	for i := 0; i < 4; i++ {
		if err := tt.Click("Commit details"); err != nil {
			t.Fatal(err)
		}
		assertStable()
	}
	if err := tt.Click("Refresh history"); err != nil {
		t.Fatal(err)
	}
	if a.CommitDetails.Commit.OID != selectedOID {
		t.Fatal("refresh cleared selection before completion")
	}
	settle(t, a, q)
	assertStable()
	// A refreshed page can insert rows before the selection. Preserve by OID.
	tip = strings.TrimSpace(string(testgit.Run(t, dir, "commit-tree", tree, "-p", tip, "-m", "new control tip")))
	testgit.Run(t, dir, "update-ref", "refs/heads/main", tip)
	if err := tt.Click("Refresh history"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if a.CommitDetails.Commit.OID != selectedOID || a.HistorySelected != 41 || a.HistoryList.Columns.Widths["author"] != 95 {
		t.Fatal("refresh failed to remap selected OID", a.HistorySelected)
	}
}

func TestHistoryDetailsDividerStillResizes(t *testing.T) {
	a, _ := setup(t)
	a.Pane = "History"
	a.HistoryDetailsOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	divider, ok := tt.Find("Resize commit details")
	if !ok {
		t.Fatal("missing divider")
	}
	before, ok := tt.Find("Git history workspace")
	if !ok {
		t.Fatal("missing history")
	}
	x, y := divider.X+divider.W/2, divider.Y+divider.H/2
	tt.Press(x, y)
	tt.Move(x, y-50)
	tt.Release(x, y-50)
	tt.Frame()
	after, _ := tt.Find("Git history workspace")
	if after.H >= before.H {
		t.Fatal("dragging details divider did not grow the details panel", before, after)
	}
	divider, _ = tt.Find("Resize commit details")
	tt.ClickAt(divider.X+divider.W/2, divider.Y+divider.H/2)
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	afterKey, _ := tt.Find("Git history workspace")
	if afterKey.H <= after.H {
		t.Fatal("keyboard resizing no longer works", after, afterKey)
	}
}

func TestBranchTreeDividerResizes(t *testing.T) {
	a, _ := setup(t)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	divider, ok := tt.Find("Resize branch tree")
	if !ok {
		t.Fatal("missing tree divider")
	}
	tree, ok := tt.Find("Branch navigation")
	if !ok {
		t.Fatal("missing branch tree")
	}
	x, y := divider.X+divider.W/2, divider.Y+divider.H/2
	tt.Press(x, y)
	tt.Move(x+60, y)
	tt.Release(x+60, y)
	tt.Frame()
	grown, _ := tt.Find("Branch navigation")
	if grown.W <= tree.W {
		t.Fatal("dragging tree divider did not resize", tree, grown)
	}
	if err := tt.Click("Toggle branch tree"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Branch navigation"); ok {
		t.Fatal("branch tree should be hidden after toggle")
	}
	if err := tt.Click("Toggle branch tree"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	restored, ok := tt.Find("Branch navigation")
	if !ok {
		t.Fatal("branch tree should be restored after toggle")
	}
	if restored.W <= tree.W {
		t.Fatal("branch tree lost its resized width across hide/restore", tree, restored)
	}
}
