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
	expanded := map[string]bool{"section:Local": true, "section:Remote": true, "Remote/origin": true, "Remote/upstream": true}
	rows := treeRows(branches, nil, nil, "", expanded)
	for _, row := range rows {
		if row.label == "Delivery" || row.label == "HEAD" {
			t.Fatal("collapsed or symbolic ref displayed", row)
		}
	}
	rows = treeRows(branches, nil, nil, "ORIGIN/FEAT/del", expanded)
	if len(rows) != 4 || rows[0].label != "Remote" || rows[3].label != "Delivery" || rows[3].depth != 3 {
		t.Fatalf("search lost ancestors: %+v", rows)
	}
	if expanded["Remote/origin/feat"] {
		t.Fatal("search changed expansion state")
	}
	if rows := treeRows(branches, nil, nil, "missing", expanded); len(rows) != 0 {
		t.Fatal(rows)
	}
}

func TestBranchTreeTagsAndStashesSections(t *testing.T) {
	branches := []gitexec.BranchRecord{{Name: "refs/heads/main", Current: true}}
	tags := []gitexec.TagRecord{{Name: "refs/tags/v1.0", OID: strings.Repeat("t", 40)}}
	stashes := []gitexec.StashRecord{{Ref: "stash@{0}", Subject: "wip: config"}}
	// Every section starts collapsed, so only its header is listed.
	rows := treeRows(branches, tags, stashes, "", nil)
	if got := treeLabels(rows); got != "Local,Remote,Tags,Stashes" {
		t.Fatalf("default sections: %s", got)
	}
	expanded := map[string]bool{"section:Local": true, "section:Tags": true, "section:Stashes": true}
	rows = treeRows(branches, tags, stashes, "", expanded)
	want := "Local,main,Remote,Tags,v1.0,Stashes,wip: config"
	if got := treeLabels(rows); got != want {
		t.Fatalf("sections: %s", got)
	}
	// A collapsed section hides its tags and stashes again.
	delete(expanded, "section:Tags")
	delete(expanded, "section:Stashes")
	rows = treeRows(branches, tags, stashes, "", expanded)
	for _, row := range rows {
		if row.tag != nil || row.stash != nil {
			t.Fatal("collapsed section still lists entries", row)
		}
	}
	// Search matches tag names and stash subjects.
	if rows = treeRows(branches, tags, stashes, "wip", nil); len(rows) != 2 {
		t.Fatalf("stash search: %+v", rows)
	}
	if rows = treeRows(branches, tags, stashes, "v1.0", nil); len(rows) != 2 {
		t.Fatalf("tag search: %+v", rows)
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
	if tt.HasText("branch-00000") {
		t.Fatal("tree started expanded")
	}
	for _, label := range []string{"Toggle section Remote", "Toggle branch folder Remote/origin", "Toggle branch folder Remote/origin/feat"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if !tt.HasText("branch-00000") {
		t.Fatal("expanded folder did not list branches")
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

func treeLabels(rows []treeRow) string {
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.label)
	}
	return strings.Join(labels, ",")
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
