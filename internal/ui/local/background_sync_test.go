package local

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func syncApp(t *testing.T, dir string) (*App, chan func()) {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 64)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Open(dir)
	settleSync(t, a, queue)
	return a, queue
}

func settleSync(t *testing.T, a *App, queue chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for a.Loading || a.Writing || a.DialogOpen || a.syncInFlight {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatal("background operation did not finish")
		}
	}
}

func TestBackgroundRefreshPreservesSelectionAndScroll(t *testing.T) {
	dir := testgit.Repo(t)
	for i := 0; i < 24; i++ {
		testgit.Write(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), fmt.Sprintf("base %d\n", i))
	}
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	for i := 0; i < 24; i++ {
		testgit.Write(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), fmt.Sprintf("changed %d\n", i))
	}
	a, queue := syncApp(t, dir)
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if len(a.Entries) != 24 {
		t.Fatal(a.Entries)
	}
	a.Select(12)
	settleSync(t, a, queue)
	tt.Frame()
	if a.Selected != 12 || a.Preview.Path != "f12.txt" {
		t.Fatal(a.Selected, a.Preview)
	}
	// Scroll the changes list to its end so a reset to the top is observable.
	header, ok := tt.Find("Local Changes")
	if !ok {
		t.Fatal(tt.Texts())
	}
	x, y := header.X+header.W/2, header.Y+260
	found := false
	var bottom ui.Rect
	for i := 0; i < 40; i++ {
		tt.Scroll(x, y, 0, 480)
		tt.Frame()
		if r, ok := tt.Find("f23.txt"); ok {
			found, bottom = true, r
			break
		}
	}
	if !found {
		t.Fatal("could not scroll to the end of the changes list", tt.Texts())
	}
	// A fixture change alters the status: f05 moves from Unstaged to Staged.
	// The staged copy and the removed unstaged copy sit on opposite sides of
	// the selection, so f12 keeps its index while its key must stay selected.
	testgit.Run(t, dir, "add", "f05.txt")
	a.refresh(true)
	settleSync(t, a, queue)
	tt.Frame()
	if a.Selected < 0 || a.Selected >= len(a.Entries) || a.Entries[a.Selected].Change.Path != "f12.txt" || a.Entries[a.Selected].Group != "Unstaged" {
		t.Fatal("selection was not remapped by key", a.Selected)
	}
	if a.Entries[0].Group != "Staged" || a.Entries[0].Change.Path != "f05.txt" || len(a.Entries) != 24 {
		t.Fatal("changed status was not applied", a.Entries)
	}
	if a.Preview.Path != "f12.txt" || a.Preview.Scope != domain.WorktreeDiff {
		t.Fatal("preview was not kept for the selected entry", a.Preview)
	}
	rows := a.changeRows()
	if a.StatusRowSelected < 0 || a.StatusRowSelected >= len(rows) || rows[a.StatusRowSelected].entry != a.Selected {
		t.Fatal("rendered selection does not match the selected entry", a.StatusRowSelected)
	}
	after, ok := tt.Find("f23.txt")
	if !ok || absDiff(int(after.Y-bottom.Y)) > 2 || absDiff(int(after.X-bottom.X)) > 2 {
		t.Fatal("changes list scroll was reset by the background refresh", bottom, after)
	}
	if a.Message != "Ready" || a.Stale || a.Loading {
		t.Fatal("background refresh wrote user-visible state", a.Message, a.Stale, a.Loading)
	}
}

func absDiff(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestBackgroundRefreshKeepsCommitDiff(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	oid := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Write(t, filepath.Join(dir, "other.txt"), "changed\n")
	a, queue := syncApp(t, dir)
	a.Pane = ""
	a.Mode = "log"
	a.railPane = ""
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	settleSync(t, a, queue)
	a.CommitInput = oid
	a.CommitDiff()
	settleSync(t, a, queue)
	tt.Frame()
	settleSync(t, a, queue)
	before := a.Preview
	if before.Scope != domain.CommitDiff || !strings.Contains(before.Text, "+base") {
		t.Fatal(before)
	}
	testgit.Write(t, filepath.Join(dir, "another.txt"), "new\n")
	a.refresh(true)
	settleSync(t, a, queue)
	tt.Frame()
	if a.Preview != before || a.Selected != -1 || a.Mode != "log" {
		t.Fatal("commit diff did not survive the background refresh", a.Preview, a.Selected, a.Mode)
	}
	if len(a.Entries) != 2 {
		t.Fatal("changed status was not applied", a.Entries)
	}
	if a.Message != "Ready" || a.Stale || a.Loading {
		t.Fatal("background refresh wrote user-visible state", a.Message, a.Stale, a.Loading)
	}
}

func TestBackgroundRefreshHasNoBusyState(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "busy base")
	tree := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD^{tree}")))
	tip := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	for i := 0; i < 119; i++ {
		tip = strings.TrimSpace(string(testgit.Run(t, dir, "commit-tree", tree, "-p", tip, "-m", fmt.Sprintf("busy commit %02d", i))))
	}
	testgit.Run(t, dir, "update-ref", "refs/heads/main", tip)
	a, queue := syncApp(t, dir)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settleSync(t, a, queue)
	tt.Frame()
	if !a.HistoryPage.More || !tt.HasText("Load more") {
		t.Fatal("history page has no more commits to load", a.HistoryPage.More, tt.Texts())
	}
	message, stale := a.Message, a.Stale
	testgit.Write(t, filepath.Join(dir, "untracked.txt"), "new\n")
	results := make(chan error, 2)
	a.autoResults = results
	a.refresh(true)
	if a.Loading || a.quietLoading {
		t.Fatal("background sync set the global busy flags")
	}
	tt.Frame()
	if _, ok := tt.Find("Git operation in progress"); ok {
		t.Fatal("status bar shows a spinner during background sync")
	}
	// "Load more" must stay enabled: clicking it starts a user read that
	// supersedes the background sync.
	if err := tt.Click("Load more"); err != nil {
		t.Fatal(err)
	}
	if !a.Loading {
		t.Fatal("Load more was disabled during the background sync")
	}
	settleSync(t, a, queue)
	tt.Frame()
	if len(a.GraphRows) != 120 {
		t.Fatal("load more did not apply", len(a.GraphRows))
	}
	if a.Message != message || a.Stale != stale || a.lastSyncError != "" {
		t.Fatal("background sync wrote user-visible state", a.Message, a.Stale, a.lastSyncError)
	}
	// User operations feed the backoff channel with nil too; the cancelled
	// background read must not contribute an error.
	for len(results) > 0 {
		if err := <-results; err != nil {
			t.Fatal("superseded background error reached the backoff feed", err)
		}
	}
}

func TestBackgroundRefreshFailureIsSilent(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	a, queue := syncApp(t, dir)
	results := make(chan error, 1)
	a.autoResults = results
	message, stale, entries := a.Message, a.Stale, len(a.Entries)
	// Point the next sync at a directory that is no longer a repository.
	broken := a.Repository
	broken.Repository.Path = filepath.Join(dir, "missing")
	a.Repository = broken
	a.refresh(true)
	settleSync(t, a, queue)
	if a.Loading || a.Stale != stale || a.Message != message || len(a.Entries) != entries {
		t.Fatal("background failure became a user event", a.Message, a.Stale)
	}
	if a.lastSyncError == "" {
		t.Fatal("background failure was not recorded silently")
	}
	select {
	case err := <-results:
		if err == nil {
			t.Fatal("backoff feed received no error")
		}
	default:
		t.Fatal("backoff feed received no result")
	}
}

func TestForegroundRefreshDetectsExternalEdit(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "main.go"), "package main\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	a, queue := syncApp(t, dir)
	if len(a.Entries) != 0 {
		t.Fatalf("repository should start clean: %+v", a.Entries)
	}

	testgit.Write(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	a.SetBackground(false)
	settleSync(t, a, queue)
	if len(a.Entries) != 1 || a.Entries[0].Change.Path != "main.go" {
		t.Fatalf("foreground refresh did not detect external edit: %+v", a.Entries)
	}
}

func TestCommitModeSwitchDetectsExternalEdit(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "main.go"), "package main\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	a, queue := syncApp(t, dir)
	a.Mode, a.Pane, a.railPane = "editor", "Editor", "Editor"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if len(a.Entries) != 0 {
		t.Fatalf("repository should start clean: %+v", a.Entries)
	}

	testgit.Write(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	a.Pane = "Status"
	tt.Frame()
	settleSync(t, a, queue)
	if len(a.Entries) != 1 || a.Entries[0].Change.Path != "main.go" {
		t.Fatalf("switching to Source Control did not detect external edit: %+v", a.Entries)
	}
}

func TestApplyStatusPatchIgnoresIdenticalSnapshot(t *testing.T) {
	a := checkboxApp(t)
	old := "renamed-from.txt"
	a.Status = domain.GitStatus{RepositoryID: "mock", Changes: []domain.Change{
		{Path: "a.txt", WorktreeStatus: "M"},
		{Path: "b.txt", IndexStatus: "M", WorktreeStatus: " "},
		{Path: "c.txt", WorktreeStatus: "M", OldPath: &old},
	}}
	a.Entries = domain.StatusEntries(a.Status)
	a.Selected = 2
	a.StatusRowSelected = 4
	a.statusRenderedSelection = 2
	a.Preview = domain.DiffPreview{Scope: domain.WorktreeDiff, Path: "c.txt", Text: "diff"}
	list := a.StatusList
	a.applyStatusPatch(a.Status, domain.StatusEntries(a.Status), a.Preview, true)
	if a.Selected != 2 || a.StatusRowSelected != 4 || a.statusRenderedSelection != 2 {
		t.Fatal("identical snapshot disturbed the selection", a.Selected, a.StatusRowSelected)
	}
	if a.Preview.Scope != domain.WorktreeDiff || a.Preview.Path != "c.txt" {
		t.Fatal("identical snapshot disturbed the preview", a.Preview)
	}
	if !reflect.DeepEqual(a.StatusList, list) {
		t.Fatal("identical snapshot reset the list state")
	}
}

func TestApplyStatusPatchRemapsSelectionByKey(t *testing.T) {
	newStatus := func(changes ...domain.Change) domain.GitStatus {
		return domain.GitStatus{RepositoryID: "mock", Changes: changes}
	}
	unstaged := func(path string) domain.Change {
		return domain.Change{Path: path, WorktreeStatus: "M"}
	}
	type scenario struct {
		name      string
		selected  int
		changes   []domain.Change
		wantSel   int
		wantPath  string
		wantReset bool
	}
	renamed := "b-previous.txt"
	for _, s := range []scenario{
		{name: "insertion above shifts by key", selected: 1, changes: []domain.Change{unstaged("0.txt"), unstaged("a.txt"), unstaged("b.txt"), unstaged("c.txt")}, wantSel: 2, wantPath: "b.txt"},
		{name: "removal above shifts by key", selected: 1, changes: []domain.Change{unstaged("b.txt"), unstaged("c.txt")}, wantSel: 0, wantPath: "b.txt"},
		{name: "removal at selection clears it", selected: 2, changes: []domain.Change{unstaged("a.txt"), unstaged("b.txt")}, wantSel: -1, wantReset: true},
		{name: "changed row keeps the selection", selected: 1, changes: []domain.Change{unstaged("a.txt"), {Path: "b.txt", WorktreeStatus: "M", OldPath: &renamed}, unstaged("c.txt")}, wantSel: 1, wantPath: "b.txt"},
	} {
		t.Run(s.name, func(t *testing.T) {
			a := checkboxApp(t)
			a.Status = newStatus(unstaged("a.txt"), unstaged("b.txt"), unstaged("c.txt"))
			a.Entries = domain.StatusEntries(a.Status)
			a.Selected = s.selected
			a.StatusRowSelected = s.selected + 1
			a.statusRenderedSelection = s.selected
			status := newStatus(s.changes...)
			a.applyStatusPatch(status, domain.StatusEntries(status), a.Preview, false)
			if a.Selected != s.wantSel {
				t.Fatal("selection not remapped", a.Selected)
			}
			if s.wantPath != "" && a.Entries[a.Selected].Change.Path != s.wantPath {
				t.Fatal("selection points at the wrong entry", a.Entries[a.Selected])
			}
			if s.wantReset {
				if a.statusRenderedSelection != -2 {
					t.Fatal("cleared selection does not re-derive the rendered row", a.statusRenderedSelection)
				}
			} else if a.StatusRowSelected < 0 || a.StatusRowSelected >= len(a.Entries) {
				t.Fatal("rendered selection lost", a.StatusRowSelected)
			}
			if !reflect.DeepEqual(a.Status.Changes, status.Changes) {
				t.Fatal("status snapshot not applied")
			}
			if a.Preview.Scope != "" {
				t.Fatal("preview changed without a replacement", a.Preview)
			}
		})
	}
}
