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
	tt := ui.NewTester(a.historyView, 800, 650)
	widths := map[int]float32{}
	for _, width := range []int{800, 1200, 1000} {
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
		description := box("Commit description " + commit.OID)
		date, author, revision := box("Commit date "+commit.OID), box("Commit author "+commit.OID), box("Commit revision "+commit.OID)
		if list.W != float32(width) || description.X >= date.X || date.X >= author.X || author.X >= revision.X || revision.X+revision.W > list.X+list.W {
			t.Fatalf("columns at width %d: list=%+v description=%+v date=%+v author=%+v revision=%+v", width, list, description, date, author, revision)
		}
		for _, label := range []string{"Graph", "Description", "Date", "Author", "Commit"} {
			box(label)
		}
		widths[width] = description.W
	}
	if widths[1200] <= widths[1000] || widths[1000] <= widths[800] {
		t.Fatal("description did not follow viewport", widths)
	}
}
