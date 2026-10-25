package local

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

func TestBranchTreeSearchAndCollapse(t *testing.T) {
	branches := []gitexec.BranchRecord{
		{Name: "refs/heads/main", Current: true},
		{Name: "refs/heads/feat/local"},
		{Name: "refs/remotes/origin/feat/Delivery"},
		{Name: "refs/remotes/origin/main"},
		{Name: "refs/remotes/origin/HEAD", Symbolic: "refs/remotes/origin/main"},
		{Name: "refs/remotes/upstream/feat/other"},
	}
	collapsed := map[string]bool{"Remote/origin/feat": true}
	rows := branchTreeRows(branches, "", collapsed)
	for _, row := range rows {
		if row.node.label == "Delivery" || row.node.label == "HEAD" {
			t.Fatal("collapsed or symbolic ref displayed", row)
		}
	}
	rows = branchTreeRows(branches, "ORIGIN/FEAT/del", collapsed)
	if len(rows) != 4 || rows[0].node.label != "Remote" || rows[3].node.label != "Delivery" || rows[3].depth != 3 {
		t.Fatalf("search lost ancestors: %+v", rows)
	}
	if !collapsed["Remote/origin/feat"] {
		t.Fatal("search changed collapse state")
	}
	if rows := branchTreeRows(branches, "missing", collapsed); len(rows) != 0 {
		t.Fatal(rows)
	}
}

func TestBranchTreeVirtualizedAndSearchRestoresCollapse(t *testing.T) {
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	for i := 0; i < 10000; i++ {
		a.Branches = append(a.Branches, gitexec.BranchRecord{Name: fmt.Sprintf("refs/remotes/origin/feat/branch-%05d", i)})
	}
	tt := ui.NewTester(a.branchNavigation, 280, 650)
	if a.branchRowsBuilt > 50 {
		t.Fatal("tree is not virtualized", a.branchRowsBuilt)
	}
	if err := tt.Click("Toggle branch folder Remote/origin/feat"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("branch-00000") {
		t.Fatal("folder did not collapse")
	}
	a.BranchQuery = "branch-09999"
	tt.Frame()
	if !tt.HasText("branch-09999") {
		t.Fatal(tt.Texts())
	}
	a.BranchQuery = ""
	tt.Frame()
	if tt.HasText("branch-00000") {
		t.Fatal("clearing search lost collapse state")
	}
}

func TestHistorySearchWithinLoadedScope(t *testing.T) {
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.HistoryRef = "refs/remotes/origin/feature"
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{{OID: strings.Repeat("a", 40), Subject: "tip"}, {OID: strings.Repeat("b", 40), Subject: "common ancestor"}})
	a.HistoryQuery = "common"
	tt := ui.NewTester(a.historyView, 900, 650)
	if !tt.HasText("common ancestor") || tt.HasText("tip") || len(a.historyVisible) != 1 || a.historyVisible[0] != 1 {
		t.Fatal(tt.Texts(), a.historyVisible)
	}
	if tt.HasText("Show Remote Branches") {
		t.Fatal("obsolete filtering control remains")
	}
}

func TestBranchTrackingLabels(t *testing.T) {
	for _, tc := range []struct{ track, want string }{{"[ahead 2, behind 3]", "↑2 ↓3 origin/main"}, {"", "origin/main"}, {"[gone]", "gone origin/main"}} {
		if got := branchTracking(gitexec.BranchRecord{Upstream: "refs/remotes/origin/main", Tracking: tc.track}); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
	if branchTracking(gitexec.BranchRecord{Tracking: "[ahead 1]"}) != "" {
		t.Fatal("tracking without upstream")
	}
}
