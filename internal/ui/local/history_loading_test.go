package local_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestHistoryAutomaticallyLoadsAndDoesNotStealNavigation(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "automatically loaded")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	tt := ui.NewTester(a.View, 1400, 850)
	if err := tt.Click("Git Log"); err != nil {
		t.Fatal(err)
	}
	if !a.Loading {
		t.Fatal("entering Git Log did not start a read")
	}
	// Navigate away before applying the result. Finishing the read must not
	// send the user back to Git Log.
	a.Pane = "Status"
	tt.Frame()
	settle(t, a, q)
	tt.Frame()
	if a.Mode != "commit" || a.Pane != "Status" {
		t.Fatal(a.Mode, a.Pane)
	}
	if len(a.GraphRows) != 1 || len(a.Branches) != 1 {
		t.Fatal(a.Message, a.GraphRows, a.Branches)
	}
	if err := tt.Click("Git Log"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.Loading || !tt.HasText("automatically loaded") {
		t.Fatal("cached history was not reused", tt.Texts())
	}
	if tt.HasText("Load history") {
		t.Fatal("manual initial load button remains")
	}
}

func TestHistoryEmptyAndFailureDoNotRetryEveryFrame(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "failed"}[invalid], func(t *testing.T) {
			dir := testgit.Repo(t)
			a, q := setup(t)
			if invalid {
				a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "invalid", Location: "local", Path: t.TempDir()}}
			} else {
				a.Open(dir)
				settle(t, a, q)
			}
			a.Pane = "History"
			tt := ui.NewTester(a.View, 1400, 850)
			settle(t, a, q)
			for i := 0; i < 5; i++ {
				tt.Frame()
			}
			if a.Loading || len(q) != 0 {
				t.Fatal("empty/failed read retried during rendering")
			}
			want := "No commits yet"
			if invalid {
				want = "History unavailable · refresh to retry"
			}
			if !tt.HasText(want) {
				t.Fatal(a.Message, tt.Texts())
			}
		})
	}
}

func TestHistorySearchSelectsOriginalCommit(t *testing.T) {
	dir := testgit.Repo(t)
	for _, subject := range []string{"first commit", "unique second"} {
		testgit.Write(t, filepath.Join(dir, "file"), subject+"\n")
		testgit.Run(t, dir, "add", ".")
		testgit.Run(t, dir, "commit", "-m", subject)
	}
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	tt.Frame()
	a.HistoryQuery = "first commit"
	tt.Frame()
	if err := tt.Click("first commit"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if a.HistorySelected != 1 || !strings.Contains(a.CommitDetails.Message, "first commit") {
		t.Fatal(a.HistorySelected, a.CommitDetails.Message)
	}
	if a.Preview.Scope != domain.CommitDiff || a.Preview.Path != "" {
		t.Fatal("commit selection should show the combined diff without choosing a file", a.Preview)
	}
	a.HistoryQuery = "no match"
	tt.Frame()
	if !tt.HasText("No matching commits") {
		t.Fatal(tt.Texts())
	}
}
