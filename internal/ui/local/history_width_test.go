package local

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
)

func TestHistoryRowsFollowViewportWidth(t *testing.T) {
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	commit := domain.CommitRecord{OID: strings.Repeat("a", 40), Subject: strings.Repeat("Long commit subject ", 20), Author: "developer", Refs: strings.Repeat("feature/long-branch ", 10), AuthoredAt: time.Now()}
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{commit})
	tt := ui.NewTester(a.historyView, 500, 650)
	for _, width := range []int{500, 1000, 700} {
		tt.SetSize(width, 650)
		tt.Frame()
		box := func(label string) ui.Rect {
			t.Helper()
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("missing %s", label)
			}
			return r
		}
		list := box("Commit history list")
		row := box("Commit row " + commit.OID)
		revision := box("Commit revision " + commit.OID)
		if list.W != float32(width) || row.W < list.W-20 || row.X+row.W > list.X+list.W+1 {
			t.Fatalf("width %d: list=%+v row=%+v", width, list, row)
		}
		if revision.X+revision.W > row.X+row.W+1 || revision.X+revision.W < row.X+row.W-2 {
			t.Fatalf("revision is not anchored to the row edge at %d: row=%+v revision=%+v", width, row, revision)
		}
	}
}
