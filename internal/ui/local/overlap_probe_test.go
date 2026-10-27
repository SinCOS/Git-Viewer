package local_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestProbeStatusOverlap(t *testing.T) {
	first := testgit.Repo(t)
	for i := 0; i < 30; i++ {
		testgit.Write(t, filepath.Join(first, fmt.Sprintf("f%02d.txt", i)), fmt.Sprintf("c%d\n", i))
		testgit.Run(t, first, "add", ".")
		testgit.Run(t, first, "commit", "-m", fmt.Sprintf("commit %02d", i))
	}
	a, q := setup(t)
	a.Open(first)
	settle(t, a, q)
	a.Pane = "History"
	a.HistoryDetailsOpen = true
	tt := ui.NewTester(a.View, 1000, 550)
	deadline := time.After(10 * time.Second)
	tt.Frame()
	for len(a.GraphRows) == 0 {
		select {
		case apply := <-q:
			apply()
		case <-deadline:
			t.Fatal("history did not load")
		}
		tt.Frame()
	}
	type row struct {
		y int
		s string
		r ui.Rect
	}
	var rows []row
	seen := map[string]bool{}
	for _, text := range tt.Texts() {
		if seen[text] {
			continue
		}
		seen[text] = true
		r, ok := tt.Find(text)
		if !ok {
			continue
		}
		rows = append(rows, row{y: int(r.Y), s: text, r: r})
	}
	for _, label := range []string{"Workspace body", "Main toolbar", "History panels", "Git history workspace", "Commit history list", "Operation status"} {
		if r, ok := tt.Find(label); ok {
			fmt.Printf("label %-24q rect=%v\n", label, r)
		} else {
			fmt.Printf("label %-24q NOT FOUND\n", label)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].y < rows[j].y })
	for _, it := range rows {
		fmt.Printf("y=%4d h=%3d %q\n", int(it.r.Y), int(it.r.H), it.s)
	}
}
