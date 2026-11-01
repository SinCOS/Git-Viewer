package local

import (
	"testing"

	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

func setCommitPreview(a *App) {
	a.CommitDetails = gitexec.CommitDetails{Commit: domain.CommitRecord{OID: "0123456789012345678901234567890123456789"}}
	a.Preview = domain.DiffPreview{Scope: domain.CommitDiff, Path: "README.md", Text: "diff --git a/README.md b/README.md\n"}
	a.lines = []string{"diff --git a/README.md b/README.md"}
	a.historyFileSelected = 0
}

func assertCommitPreviewCleared(t *testing.T, a *App) {
	t.Helper()
	if a.CommitDetails.Commit.OID != "" || a.Preview.Scope != "" || len(a.lines) != 0 || a.historyFileSelected != -1 {
		t.Fatalf("commit preview leaked into Source Control: details=%+v preview=%+v lines=%d", a.CommitDetails, a.Preview, len(a.lines))
	}
}

func TestCommitPreviewDoesNotLeakIntoSourceControl(t *testing.T) {
	a := &App{}
	setCommitPreview(a)
	a.Pane = "Status"
	a.syncMode()
	if a.Mode != "commit" {
		t.Fatalf("mode = %q", a.Mode)
	}
	assertCommitPreviewCleared(t, a)

	a = &App{}
	setCommitPreview(a)
	a.Mode = "commit"
	a.setMode("commit")
	a.Mode = "log"
	a.setMode("commit")
	if a.Pane != "Status" {
		t.Fatalf("pane = %q", a.Pane)
	}
	assertCommitPreviewCleared(t, a)
}
