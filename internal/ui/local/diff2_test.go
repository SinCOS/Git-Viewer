package local

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
)

// diff2_test.go is the UL006 layout test file (diff_test.go is the shared
// baseline and must not change). It fills App state manually, without Git.

func newDiffTestApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a, ui.NewTester(a.View, 1400, 850)
}

func TestDiffEmptyStateLayout(t *testing.T) {
	_, tt := newDiffTestApp(t)
	tt.Frame()
	if !tt.HasText("Diff Preview") {
		t.Fatal("missing Diff Preview header")
	}
	if !tt.HasText("Select a changed file to review its diff") {
		t.Fatal("missing empty-state prompt")
	}
	if err := tt.Click("Diff settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Copy diff") {
		t.Fatal("missing disabled copy control in the empty state")
	}
}

func TestDiffCommitPageDoesNotDuplicateCommitDetails(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.CommitDetails = gitexec.CommitDetails{
		Commit:  domain.CommitRecord{OID: "0123456789abcdef", Author: "A U Thor <author@example.com>", AuthoredAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)},
		Message: "subject line\n\nbody line\n",
		Files:   []gitexec.CommitFile{{Status: "M", Path: "a.txt"}, {Status: "A", Path: "b.txt"}},
	}
	a.Mode = "log"
	a.Pane = ""
	a.HistoryDetailsOpen = true
	a.applyPreview(domain.DiffPreview{
		Scope:    domain.CommitDiff,
		Path:     "a.txt",
		Text:     "+added\n context\n-removed\n@@ -1 +1 @@\n",
		OldText:  "removed\ncontext\n",
		NewText:  "added\ncontext\n",
		FullText: true,
	})
	tt.Frame()
	if !tt.HasText("COMMIT DETAILS") {
		t.Fatal("history mode did not show its commit details panel")
	}
	if !tt.HasText("a.txt") {
		t.Fatal("missing diff toolbar file identity")
	}
	a.Mode = "commit"
	a.Pane = "Status"
	tt.Frame()
	if tt.HasText("COMMIT DETAILS") || tt.HasText("Author: A U Thor") || tt.HasText("Changed files: 2") {
		t.Fatal("commit details overlapped the commit diff pane")
	}
	if tt.HasText("Copy diff") || tt.HasText("a.txt") {
		t.Fatal("commit diff controls leaked into Source Control")
	}
}

func TestOpenPreviewInEditorOpensWorktreeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt := newDiffTestApp(t)
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{
		RepositoryID: "repo", Location: "local", Path: dir,
	}}
	a.Mode = "commit"
	a.Pane = "Status"
	a.applyPreview(domain.DiffPreview{
		Scope: domain.WorktreeDiff,
		Path:  "main.go",
		Text:  "@@ -1 +1 @@\n-package main\n+package main\n",
	})
	tt.Frame()
	if err := tt.Click("Open in Editor"); err != nil {
		t.Fatal(err)
	}
	if a.Mode != "editor" || len(a.EditorTabs) != 1 || a.ActiveEditorBuffer().String() != "package main\n" {
		t.Fatalf("worktree file was not opened in the in-app editor: mode=%s tabs=%d", a.Mode, len(a.EditorTabs))
	}
	if a.Message != "Ready" {
		t.Fatalf("unexpected message: %s", a.Message)
	}
}

func TestDiffPreviousNextChangeControls(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{
		Scope:    domain.CommitDiff,
		Path:     "a.txt",
		Text:     "@@ -2 +2 @@\n-two\n+TWO\n@@ -6 +6 @@\n-six\n+SIX\n",
		OldText:  "one\ntwo\nthree\nfour\nfive\nsix\nseven\n",
		NewText:  "one\nTWO\nthree\nfour\nfive\nSIX\nseven\n",
		FullText: true,
	})
	tt.Frame()
	if !tt.HasText("1 of 2") || !tt.HasText("Previous change") || !tt.HasText("Next change") {
		t.Fatal("missing change navigation controls", tt.Texts())
	}
	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("native diff was not built")
	}
	for i := 0; i < 2; i++ {
		if err := tt.Click("Next change"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if current, inside := state.CurrentHunk(); current != 1 || !inside || !tt.HasText("2 of 2") {
		t.Fatalf("next change did not reach the second hunk: current=%d inside=%t", current, inside)
	}
	if err := tt.Click("Previous change"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if current, inside := state.CurrentHunk(); current != 0 || !inside || !tt.HasText("1 of 2") {
		t.Fatalf("previous change did not return to the first hunk: current=%d inside=%t", current, inside)
	}
}

func TestDiffSingleChangeDoesNotShowNavigation(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{
		Scope:    domain.CommitDiff,
		Path:     "a.txt",
		Text:     "@@ -2 +2 @@\n-two\n+TWO\n",
		OldText:  "one\ntwo\nthree\n",
		NewText:  "one\nTWO\nthree\n",
		FullText: true,
	})
	tt.Frame()
	if !tt.HasText("1 change") {
		t.Fatalf("missing single change count: %v", tt.Texts())
	}
	if tt.HasText("Previous change") || tt.HasText("Next change") {
		t.Fatalf("single change should not expose navigation: %v", tt.Texts())
	}
}

func TestCommitAndHistoryDiffsShareControls(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{
		Scope:    domain.CommitDiff,
		Path:     "a.txt",
		Text:     "@@ -2 +2 @@\n-two\n+TWO\n@@ -6 +6 @@\n-six\n+SIX\n",
		OldText:  "one\ntwo\nthree\nfour\nfive\nsix\nseven\n",
		NewText:  "one\nTWO\nthree\nfour\nfive\nSIX\nseven\n",
		FullText: true,
	})
	assertControls := func(mode string) {
		t.Helper()
		tt.Frame()
		for _, text := range []string{"a.txt", "1 of 2", "Previous change", "Next change", "Unified", "Side-by-Side"} {
			if !tt.HasText(text) {
				t.Fatalf("%s diff is missing %q: %v", mode, text, tt.Texts())
			}
		}
	}
	a.Mode = "commit"
	assertControls("commit")
	a.Mode = "log"
	a.Pane = ""
	a.HistoryDetailsOpen = true
	assertControls("history")
}

func TestDiffFindBarToggle(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{Scope: domain.WorktreeDiff, Path: "a.txt", Text: "alpha\nbeta\nalpha again\ngamma\n"})
	tt.Frame()
	if a.findState().open {
		t.Fatal("find bar starts open")
	}
	if err := tt.Click("Diff settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Find"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.findState().open {
		t.Fatal("Find button did not open the bar")
	}
	if !tt.HasText("Done") {
		t.Fatal("find bar not visible")
	}
	tt.Type("alpha")
	tt.Frame()
	if !tt.HasText("1 of 2") {
		t.Fatal("find bar did not count two matches")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.findState().current != 1 {
		t.Fatalf("Enter did not move to the next match: current=%d", a.findState().current)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.findState().open {
		t.Fatal("Escape did not close the bar")
	}
}
