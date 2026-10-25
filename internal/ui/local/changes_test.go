package local

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
)

func checkboxApp(t *testing.T) *App {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: t.TempDir() + "/recent.json"}})
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Pane = "Status"
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "mock", Location: "local", Path: "/tmp/mock", DisplayName: "Mock"}}
	return a
}

func drainDialog(t *testing.T, a *App, queue chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for a.Loading || a.DialogOpen {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatal("dialog did not finish")
		}
	}
}

func TestChangesFileRowsAndGroups(t *testing.T) {
	a := checkboxApp(t)
	old := "old.txt"
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "staged.txt", IndexStatus: "M", WorktreeStatus: " "}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "unstaged.txt", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
		{Change: domain.Change{Path: "renamed.txt", IndexStatus: "R", WorktreeStatus: " ", OldPath: &old}, Scope: domain.StagedDiff, Group: "Staged"},
	}
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	texts := tt.Texts()
	for _, e := range a.Entries {
		if !slices.Contains(texts, e.Change.Path) {
			t.Fatalf("file name missing: %s in %v", e.Change.Path, texts)
		}
		action := "Stage "
		if e.Scope == domain.StagedDiff {
			action = "Unstage "
		}
		label := action + strconv.QuoteToGraphic(e.Change.Path)
		if e.Change.OldPath != nil {
			label += " ← " + strconv.QuoteToGraphic(*e.Change.OldPath)
		}
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("action missing: %s", label)
		}
	}
	if !slices.Contains(texts, "Staged Changes") || !slices.Contains(texts, "Changes") {
		t.Fatalf("group headings missing: %v", texts)
	}
}

func TestChangesActionsThroughRunWrite(t *testing.T) {
	a := checkboxApp(t)
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "staged.txt", IndexStatus: "M", WorktreeStatus: " "}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "unstaged.txt", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
	}
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	a.Selected = 1
	var details []string
	a.Confirm = func(title, detail string) bool {
		details = append(details, title+"\n"+detail)
		return false
	}
	// Checking the unstaged row's box requests Stage for that path only.
	if err := tt.Click(`Stage "unstaged.txt"`); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	drainDialog(t, a, queue)
	tt.Frame()
	if len(details) != 1 || !strings.Contains(details[0], "Stage file") || !strings.Contains(details[0], `"unstaged.txt"`) {
		t.Fatalf("stage confirmation not requested: %v", details)
	}
	if a.Stale || a.Writing || a.DialogOpen || a.Selected != 1 || a.Loading {
		t.Fatalf("declined checkbox write changed state: stale=%t writing=%t dialog=%t selected=%d msg=%s", a.Stale, a.Writing, a.DialogOpen, a.Selected, a.Message)
	}
	// Unchecking the staged row's box requests Unstage for that path only.
	if err := tt.Click(`Unstage "staged.txt"`); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	drainDialog(t, a, queue)
	tt.Frame()
	if len(details) != 2 || !strings.Contains(details[1], "Unstage file") || !strings.Contains(details[1], `"staged.txt"`) {
		t.Fatalf("unstage confirmation not requested: %v", details)
	}
	if a.Stale || a.Selected != 1 {
		t.Fatalf("declined checkbox write changed state: stale=%t selected=%d", a.Stale, a.Selected)
	}
}
