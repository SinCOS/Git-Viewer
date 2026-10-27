package local

import (
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
	if !tt.HasText("Copy diff") {
		t.Fatal("missing disabled copy control in the empty state")
	}
}

func TestDiffCommitDetailsLayoutAndCopy(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{Scope: domain.CommitDiff, Text: "+added\n context\n-removed\n@@ -1 +1 @@\n"})
	a.CommitDetails = gitexec.CommitDetails{
		Commit:  domain.CommitRecord{OID: "0123456789abcdef", Author: "A U Thor <author@example.com>", AuthoredAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)},
		Message: "subject line\n\nbody line\n",
		Files:   []gitexec.CommitFile{{Status: "M", Path: "a.txt"}, {Status: "A", Path: "b.txt"}},
	}
	tt.Frame()
	if !tt.HasText("COMMIT DETAILS") {
		t.Fatal("missing COMMIT DETAILS header")
	}
	if !tt.HasText("Author: A U Thor <author@example.com>") || !tt.HasText("Authored: 2026-10-25 12:00:00") {
		t.Fatal("missing author or time line")
	}
	if !tt.HasText("subject line") || !tt.HasText("body line") {
		t.Fatal("missing message lines")
	}
	if !tt.HasText("Changed files: 2") || !tt.HasText("M \"a.txt\"") || !tt.HasText("A \"b.txt\"") {
		t.Fatal("missing changed file grouping")
	}
	if !tt.HasText("Diff Preview") {
		t.Fatal("missing Diff Preview header")
	}
	if err := tt.Click("Copy diff"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != a.Preview.Text {
		t.Fatal("copy did not retain full preview")
	}
}

func TestDiffFindBarToggle(t *testing.T) {
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{Scope: domain.WorktreeDiff, Path: "a.txt", Text: "alpha\nbeta\nalpha again\ngamma\n"})
	tt.Frame()
	if a.findState().open {
		t.Fatal("find bar starts open")
	}
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
