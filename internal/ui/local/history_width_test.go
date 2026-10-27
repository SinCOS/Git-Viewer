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
	commit := domain.CommitRecord{OID: strings.Repeat("a", 40), Subject: strings.Repeat("Long commit subject ", 20), Author: "developer", Refs: "HEAD -> master, origin/master", AuthoredAt: time.Now()}
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
		branches, author := box("Commit branches "+commit.OID), box("Commit author "+commit.OID)
		date, revision := box("Commit date "+commit.OID), box("Commit revision "+commit.OID)
		if list.W != float32(width) || description.X >= branches.X || branches.X >= author.X || author.X >= date.X || date.X >= revision.X || revision.X+revision.W > list.X+list.W {
			t.Fatalf("columns at width %d: list=%+v description=%+v branches=%+v author=%+v date=%+v revision=%+v", width, list, description, branches, author, date, revision)
		}
		for _, label := range []string{"Graph", "Description", "Branches", "Author", "Date", "Commit"} {
			box(label)
		}
		widths[width] = description.W
	}
	if widths[1200] <= widths[1000] || widths[1000] <= widths[800] {
		t.Fatal("description did not follow viewport", widths)
	}
}

func TestHistoryColumnsStayAlignedAtNarrowWidths(t *testing.T) {
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	commit := domain.CommitRecord{
		OID:        strings.Repeat("b", 40),
		Subject:    strings.Repeat("narrow commit subject ", 8),
		Author:     "Richard",
		Refs:       "HEAD -> feature/long-branch-name, origin/feature/long-branch-name",
		AuthoredAt: time.Now(),
	}
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{commit})
	tt := ui.NewTester(a.historyView, 480, 500)
	tt.Frame()
	list, ok := tt.Find("Commit history list")
	if !ok {
		t.Fatal("missing history table")
	}
	positions := make(map[string]ui.Rect)
	for _, label := range []string{
		"Commit description " + commit.OID,
		"Commit branches " + commit.OID,
		"Commit author " + commit.OID,
		"Commit date " + commit.OID,
		"Commit revision " + commit.OID,
	} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatal("missing cell", label)
		}
		positions[label] = r
	}
	ordered := []string{
		"Commit description " + commit.OID,
		"Commit branches " + commit.OID,
		"Commit author " + commit.OID,
		"Commit date " + commit.OID,
		"Commit revision " + commit.OID,
	}
	for i := 1; i < len(ordered); i++ {
		if positions[ordered[i-1]].X >= positions[ordered[i]].X {
			t.Fatalf("columns crossed at narrow width: %+v", positions)
		}
	}
	for _, label := range ordered {
		if positions[label].Y < list.Y || positions[label].Y+positions[label].H > list.Y+list.H {
			t.Fatalf("cell escaped its row or table: list=%+v cells=%+v", list, positions)
		}
	}
}
