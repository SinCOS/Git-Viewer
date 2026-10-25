package local

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
)

// TestGraphHitTest covers the pure row-content hit test: a hit is any x within
// ±6px of 12 + lane*16, the frozen dot-center formula shared with graphRow.
func TestGraphHitTest(t *testing.T) {
	a := &App{}
	a.GraphRows = []domain.GraphRow{
		{Commit: domain.CommitRecord{OID: "m1"}, Lane: 0},
		{Commit: domain.CommitRecord{OID: "p2"}, Lane: 3},
	}
	for _, tc := range []struct {
		name string
		row  int
		x    float32
		want bool
	}{
		{"lane0 center", 0, 12, true},
		{"lane0 +6 boundary", 0, 12 + 6, true},
		{"lane0 -6 boundary", 0, 12 - 6, true},
		{"lane0 just outside", 0, 12 + 6.5, false},
		{"neighbor lane center", 0, 12 + 16, false},
		{"lane3 center", 1, 12 + 3*16, true},
		{"lane3 just outside", 1, 12 + 3*16 - 6.5, false},
		{"row out of range", 2, 12, false},
		{"negative row", -1, 12, false},
	} {
		if got := a.graphHitTest(tc.row, tc.x); got != tc.want {
			t.Fatalf("%s: graphHitTest(%d, %v) = %v, want %v", tc.name, tc.row, tc.x, got, tc.want)
		}
	}
}

// TestGraphRowRenderMerge builds a merge-first history through the real domain
// layout and renders the History pane with ui.NewTester, mirroring the mock
// construction in TestHistoryGraphVirtualization. It asserts graphRow draws
// every visible row without panicking and the commit subjects still appear.
func TestGraphRowRenderMerge(t *testing.T) {
	commits := []domain.CommitRecord{
		{OID: "m1", Subject: "merge commit row", Parents: []string{"p1", "p2"}},
		{OID: "p1", Subject: "first parent row"},
		{OID: "p2", Subject: "second parent row"},
	}
	a := New(application.LocalService{})
	a.Pane = "History"
	a.GraphRows = a.Graph.Append(commits)
	for _, row := range a.GraphRows {
		a.HistoryLanes = max(a.HistoryLanes, row.Width)
	}
	if len(a.GraphRows[0].Edges) < 2 {
		t.Fatal("merge commit should carry two parent edges", a.GraphRows[0].Edges)
	}
	a.HistorySelected = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if a.HistoryBuiltRows == 0 {
		t.Fatal("graph rows were not rendered")
	}
	for _, want := range []string{"merge commit row", "first parent row", "second parent row"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
}
