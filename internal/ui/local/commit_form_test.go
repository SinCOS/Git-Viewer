package local

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestCommitFormBatchActionsConfirmPaths(t *testing.T) {
	a := checkboxApp(t)
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "staged.txt", IndexStatus: "M"}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "worktree.txt", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
	}
	var detail string
	a.Confirm = func(_, text string) bool { detail = text; return false }
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	for _, tc := range []struct{ button, action, path string }{
		{"Stage all changes", "Stage file", "worktree.txt"},
		{"Unstage all changes", "Unstage file", "staged.txt"},
	} {
		if err := tt.Click(tc.button); err != nil {
			t.Fatal(err)
		}
		drainDialog(t, a, queue)
		tt.Frame()
		if !strings.Contains(detail, tc.action) || !strings.Contains(detail, tc.path) {
			t.Fatalf("%s confirmation: %q", tc.button, detail)
		}
	}
}

func TestCommitFormReadiness(t *testing.T) {
	a := checkboxApp(t)
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if !tt.HasText("Stage at least one file to commit") || !tt.HasText("Commit 0 file(s)") {
		t.Fatalf("missing empty staging guidance: %v", tt.Texts())
	}
	a.Entries = []domain.StatusEntry{{Change: domain.Change{Path: "file", IndexStatus: "M"}, Scope: domain.StagedDiff, Group: "Staged"}}
	tt.Frame()
	if !tt.HasText("Enter a commit message") || !tt.HasText("Commit 1 file(s)") {
		t.Fatalf("missing message guidance or count: %v", tt.Texts())
	}
	a.MessageInput = "message"
	a.Stale = true
	tt.Frame()
	if !tt.HasText("Repository status is out of date. Refresh changes before committing.") {
		t.Fatalf("missing stale guidance: %v", tt.Texts())
	}
	called := false
	a.Confirm = func(_, _ string) bool { called = true; return false }
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	if called || a.DialogOpen {
		t.Fatal("stale commit must remain disabled")
	}
}
