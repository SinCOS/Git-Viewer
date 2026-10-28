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
		label := "More actions for " + strconv.QuoteToGraphic(e.Change.Path)
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
	confirmed := false
	a.Confirm = func(_, _ string) bool { confirmed = true; return false }
	// The file menu starts routine writes immediately, like VS Code.
	if err := tt.Click(`More actions for "unstaged.txt"`); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stage file"); err != nil {
		t.Fatal(err)
	}
	if confirmed || a.DialogOpen || !a.Loading || !a.Writing || !strings.Contains(a.Message, "Stage file") {
		t.Fatalf("stage did not start directly: confirmed=%t dialog=%t loading=%t writing=%t message=%q", confirmed, a.DialogOpen, a.Loading, a.Writing, a.Message)
	}
	drainDialog(t, a, queue)
	a.Stale = false
	// Unstage also starts directly and keeps the current row selected.
	if err := tt.Click(`More actions for "staged.txt"`); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unstage file"); err != nil {
		t.Fatal(err)
	}
	if confirmed || a.DialogOpen || !a.Loading || !a.Writing || !strings.Contains(a.Message, "Unstage file") {
		t.Fatalf("unstage did not start directly: confirmed=%t dialog=%t loading=%t writing=%t message=%q", confirmed, a.DialogOpen, a.Loading, a.Writing, a.Message)
	}
	if a.Selected != 1 {
		t.Fatalf("routine write changed selection: %d", a.Selected)
	}
	drainDialog(t, a, queue)
}

func TestChangesContextMenuUsesFileOperations(t *testing.T) {
	a := checkboxApp(t)
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "src/staged.txt", IndexStatus: "M"}, Scope: domain.StagedDiff, Group: "Staged"},
		{Change: domain.Change{Path: "src/unstaged.txt", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
	}
	confirmed := 0
	a.Confirm = func(_, _ string) bool { confirmed++; return false }
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	for _, tc := range []struct{ file, action string }{
		{"staged.txt", "Unstage file"},
		{"unstaged.txt", "Stage file"},
	} {
		if err := tt.RightClick(tc.file); err != nil {
			t.Fatal(err)
		}
		if got := tt.Menu(); !slices.Equal(got, []string{tc.action, "Discard changes", "-", "Copy path"}) {
			t.Fatalf("menu for %s: %v", tc.file, got)
		}
		if err := tt.ChooseMenuItem(tc.action); err != nil {
			t.Fatal(err)
		}
		if confirmed != 0 || a.DialogOpen || !a.Loading || !a.Writing || !strings.Contains(a.Message, tc.action) {
			t.Fatalf("%s did not start directly: confirmed=%d dialog=%t loading=%t writing=%t message=%q", tc.action, confirmed, a.DialogOpen, a.Loading, a.Writing, a.Message)
		}
		drainDialog(t, a, queue)
		a.Stale = false
	}
	if err := tt.RightClick("unstaged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Copy path"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "src/unstaged.txt" {
		t.Fatalf("clipboard = %q", got)
	}
	if err := tt.RightClick("unstaged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Discard changes"); err != nil {
		t.Fatal(err)
	}
	if confirmed != 1 || !a.DialogOpen {
		t.Fatalf("discard must still confirm: confirmed=%d dialog=%t", confirmed, a.DialogOpen)
	}
	drainDialog(t, a, queue)
	a.Writing = true
	tt.Frame()
	if err := tt.RightClick("unstaged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stage file"); err == nil {
		t.Fatal("stage enabled during write")
	}
}
