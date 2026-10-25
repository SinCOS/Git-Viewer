package local_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

func TestHistoryHeaderAndCommitDetails(t *testing.T) {
	a, _ := setup(t)
	a.Pane = "History"
	a.HistoryDetailsOpen = true
	commits := make([]domain.CommitRecord, 10000)
	for i := range commits {
		commits[i] = domain.CommitRecord{
			OID:        fmt.Sprintf("%040x", 10000-i),
			Subject:    fmt.Sprintf("commit %05d", i),
			Author:     "Alice",
			AuthoredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}
		if i+1 < len(commits) {
			commits[i].Parents = []string{fmt.Sprintf("%040x", 9999-i)}
		}
	}
	a.GraphRows = a.Graph.Append(commits)
	// CommitDetails is populated by the existing CommitDiff flow; the mock
	// fills it directly to exercise the details section layout.
	a.CommitDetails = gitexec.CommitDetails{
		Commit:  commits[0],
		Message: "detail body\n",
		Files:   []gitexec.CommitFile{{Status: "M", Path: "file"}, {Status: "A", Path: "added file"}},
	}
	tt := ui.NewTester(a.View, 1400, 850)
	if !tt.HasText("Git Log") || !tt.HasText("History · 10000 commits") {
		t.Fatal(tt.Texts())
	}
	if !tt.HasText("COMMIT DETAILS") || !tt.HasText("Alice · 2026-01-02 03:04:05") || !tt.HasText("detail body") || !tt.HasText("Changed files: 2") || !tt.HasText("file") || !tt.HasText("added file") {
		t.Fatal(tt.Texts())
	}
	if a.HistoryBuiltRows > 80 {
		t.Fatal("history not virtualized", a.HistoryBuiltRows)
	}
	a.HistoryList.ScrollTo(9999, ui.Center)
	tt.Frame()
	if !tt.HasText("commit 09999") || a.HistoryBuiltRows > 80 {
		t.Fatal(a.HistoryBuiltRows, tt.Texts())
	}
}
