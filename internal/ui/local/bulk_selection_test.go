package local

import (
	"strings"
	"testing"

	"gitviewer/internal/domain"
)

func TestSelectedRoutineChangesStartWithoutConfirmation(t *testing.T) {
	a := checkboxApp(t)
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "staged.txt", IndexStatus: "M", WorktreeStatus: " "}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "unstaged.txt", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
	}
	a.selectedDiffEntries()
	for _, entry := range a.Entries {
		checked := true
		a.findState().checked[entryKey(entry)] = &checked
	}
	confirmed := false
	a.Confirm = func(_, _ string) bool { confirmed = true; return false }
	a.runSelectedChanges(domain.Stage)
	if confirmed || a.DialogOpen || !a.Loading || !a.Writing || !strings.Contains(a.Message, "Stage file") {
		t.Fatalf("stage did not start directly: confirmed=%t dialog=%t loading=%t writing=%t message=%q", confirmed, a.DialogOpen, a.Loading, a.Writing, a.Message)
	}
	drainDialog(t, a, queue)
	a.Stale = false
	a.runSelectedChanges(domain.Unstage)
	if confirmed || a.DialogOpen || !a.Loading || !a.Writing || !strings.Contains(a.Message, "Unstage file") {
		t.Fatalf("unstage did not start directly: confirmed=%t dialog=%t loading=%t writing=%t message=%q", confirmed, a.DialogOpen, a.Loading, a.Writing, a.Message)
	}
	drainDialog(t, a, queue)
}

func TestSelectedDiscardRejectsUnsupportedEntryBeforeConfirmation(t *testing.T) {
	a := checkboxApp(t)
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "tracked.txt", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
		{Change: domain.Change{Path: "new.txt", IndexStatus: "?", WorktreeStatus: "?", Untracked: true}, Scope: domain.WorktreeDiff, Group: "Untracked"},
	}
	a.selectedDiffEntries()
	for _, entry := range a.Entries {
		checked := true
		a.findState().checked[entryKey(entry)] = &checked
	}
	called := false
	a.Confirm = func(_, _ string) bool { called = true; return true }
	a.runSelectedChanges(domain.Discard)
	if called || a.DialogOpen || !strings.Contains(a.Message, "safely discardable") {
		t.Fatalf("unsafe mixed selection was not rejected: %q", a.Message)
	}
}

func TestSelectedChangesClearsRemovedRowsAndDifferentRepository(t *testing.T) {
	a := checkboxApp(t)
	entry := domain.StatusEntry{Change: domain.Change{Path: "file", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"}
	a.Entries = []domain.StatusEntry{entry}
	a.selectedDiffEntries()
	checked := true
	a.findState().checked[entryKey(entry)] = &checked
	if len(a.selectedDiffEntries()) != 1 {
		t.Fatal("selection was not retained")
	}
	a.Entries = nil
	a.selectedDiffEntries()
	a.Entries = []domain.StatusEntry{entry}
	if len(a.selectedDiffEntries()) != 0 {
		t.Fatal("selection survived removal and reappearance")
	}
	a.findState().checked[entryKey(entry)] = &checked
	a.Repository.Repository.RepositoryID = "another"
	if len(a.selectedDiffEntries()) != 0 {
		t.Fatal("selection crossed repositories")
	}
}
