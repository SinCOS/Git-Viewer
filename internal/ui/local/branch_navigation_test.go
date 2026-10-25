package local_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestBranchNavigationLoadsOldRefAndPinsPages(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "old base")
	base := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Run(t, dir, "branch", "feat/old", base)
	testgit.Run(t, dir, "update-ref", "refs/remotes/origin/feat/old", base)
	tree := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD^{tree}")))
	tip := base
	for i := 0; i < 105; i++ {
		tip = strings.TrimSpace(string(testgit.Run(t, dir, "commit-tree", tree, "-p", tip, "-m", fmt.Sprintf("newer %03d", i))))
	}
	testgit.Run(t, dir, "update-ref", "refs/heads/main", tip)
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	tt.Frame()
	if len(a.GraphRows) != 100 || !a.HistoryPage.More {
		t.Fatal("initial page", len(a.GraphRows), a.HistoryPage)
	}
	for _, row := range a.GraphRows {
		if row.Commit.OID == base {
			t.Fatal("fixture old tip must be outside first page")
		}
	}
	for _, ref := range []string{"refs/heads/feat/old", "refs/remotes/origin/feat/old"} {
		if err := tt.Click("Browse " + ref); err != nil {
			t.Fatal(err)
		}
		settle(t, a, q)
		tt.Frame()
		if a.HistoryRef != ref || len(a.GraphRows) != 1 || a.GraphRows[0].Commit.OID != base {
			t.Fatal("old ref not loaded", a.Message, a.HistoryRef, len(a.GraphRows))
		}
		if got := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD"))); got != tip {
			t.Fatal("browsing changed HEAD")
		}
	}
	if err := tt.Click("Browse refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	newer := strings.TrimSpace(string(testgit.Run(t, dir, "commit-tree", tree, "-p", tip, "-m", "after page one")))
	testgit.Run(t, dir, "update-ref", "refs/heads/main", newer)
	a.LoadHistory(true)
	settle(t, a, q)
	tt.Frame()
	if len(a.GraphRows) != 106 || a.GraphRows[0].Commit.OID != tip || a.GraphRows[105].Commit.OID != base {
		t.Fatal("page drift", len(a.GraphRows))
	}
	if err := tt.Click("HEAD (Current Branch)"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if a.GraphRows[0].Commit.OID != newer {
		t.Fatal("HEAD did not resolve afresh")
	}
	if err := tt.Click("Browse refs/heads/feat/old"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	testgit.Run(t, dir, "branch", "-D", "feat/old")
	a.LoadHistory(false)
	settle(t, a, q)
	tt.Frame()
	if a.HistoryRef != "" || len(a.GraphRows) != 100 {
		t.Fatal("deleted branch did not fall back to all")
	}
	other := testgit.Repo(t)
	a.BranchQuery = "old"
	a.Open(other)
	settle(t, a, q)
	if a.HistoryRef != "" || a.BranchQuery != "" || len(a.Branches) != 0 {
		t.Fatal("repository state leaked")
	}
}
