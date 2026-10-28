package local

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestCommitFormBatchActionsStartWithoutConfirmation(t *testing.T) {
	for _, tc := range []struct{ button, action string }{
		{"Stage all changes", "Stage file"},
		{"Unstage all changes", "Unstage file"},
	} {
		t.Run(tc.button, func(t *testing.T) {
			a := checkboxApp(t)
			queue := make(chan func(), 32)
			a.Dispatch = func(apply func()) { queue <- apply }
			a.Entries = []domain.StatusEntry{
				{Change: domain.Change{Path: "staged.txt", IndexStatus: "M"}, Scope: domain.StagedDiff, Group: "Staged"},
				{Change: domain.Change{Path: "worktree.txt", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
			}
			confirmed := false
			a.Confirm = func(_, _ string) bool { confirmed = true; return false }
			tt := ui.NewTester(a.View, 1400, 850)
			tt.Frame()
			if err := tt.Click(tc.button); err != nil {
				t.Fatal(err)
			}
			if confirmed || a.DialogOpen || !a.Loading || !a.Writing {
				t.Fatalf("%s did not start directly: confirmed=%t dialog=%t loading=%t writing=%t", tc.button, confirmed, a.DialogOpen, a.Loading, a.Writing)
			}
			if !strings.Contains(a.Message, tc.action) {
				t.Fatalf("%s did not start %s: %q", tc.button, tc.action, a.Message)
			}
			drainDialog(t, a, queue)
		})
	}
}

func TestCommitFormReadiness(t *testing.T) {
	a := checkboxApp(t)
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if !tt.HasText("Stage at least one file to commit") || !tt.HasText("Commit") {
		t.Fatalf("missing empty staging guidance: %v", tt.Texts())
	}
	a.Entries = []domain.StatusEntry{{Change: domain.Change{Path: "file", IndexStatus: "M"}, Scope: domain.StagedDiff, Group: "Staged"}}
	tt.Frame()
	if !tt.HasText("Enter a commit message") || !tt.HasText("Commit (1)") {
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
	if called || a.DialogOpen || a.Loading || a.Writing {
		t.Fatal("stale commit must remain disabled")
	}
}

func TestOnlyDestructiveWritesRequireConfirmation(t *testing.T) {
	for _, op := range []domain.Operation{
		domain.Stage,
		domain.Unstage,
		domain.Commit,
		domain.Amend,
		domain.Fetch,
		domain.PullFFOnly,
		domain.Push,
	} {
		if requiresConfirmation(op) {
			t.Errorf("%s should run without confirmation", op)
		}
	}
	for _, op := range []domain.Operation{domain.Discard, domain.SwitchBranch} {
		if !requiresConfirmation(op) {
			t.Errorf("%s should still require confirmation", op)
		}
	}
}
