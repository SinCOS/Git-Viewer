package local_test

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestHistoryBranchSwitchRetainsDisplayedSnapshot(t *testing.T) {
	dir := testgit.Repo(t)
	for _, subject := range []string{"snapshot base", "snapshot tip"} {
		testgit.Write(t, filepath.Join(dir, "file"), subject+"\n")
		testgit.Run(t, dir, "add", "file")
		testgit.Run(t, dir, "commit", "-m", subject)
		if subject == "snapshot base" {
			testgit.Run(t, dir, "branch", "old")
		}
	}
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	a.HistoryDetailsOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	tt.Frame()
	if err := tt.Click("snapshot tip"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	originalPage := a.HistoryPage
	originalCommit := a.CommitDetails.Commit.OID
	originalPreview := a.Preview.Text
	assertOriginal := func() {
		t.Helper()
		tt.Frame()
		if a.HistoryRef != "" || len(a.GraphRows) != 2 || a.HistoryPage.Next != originalPage.Next || a.HistoryPage.Offset != originalPage.Offset {
			t.Fatal("displayed scope/page changed before replacement was ready", a.HistoryRef, a.HistoryPage)
		}
		if a.CommitDetails.Commit.OID != originalCommit || a.Preview.Text != originalPreview || !tt.HasText("snapshot tip") || tt.HasText("Loading history…") {
			t.Fatal("loading blanked the previous snapshot", tt.Texts())
		}
	}
	if err := tt.Click("Browse refs/heads/old"); err != nil {
		t.Fatal(err)
	}
	if !a.Loading {
		t.Fatal("switch did not start loading")
	}
	assertOriginal()
	// Cancellation and a late result must not change the displayed scope.
	a.Cancel()
	a.Wait()
	for len(q) > 0 {
		(<-q)()
	}
	assertOriginal()
	// Failed reads retain both the old rows and their identity/pagination.
	a.Service.Reader.GitPath = filepath.Join(t.TempDir(), "missing-git")
	if err := tt.Click("Browse refs/heads/old"); err != nil {
		t.Fatal(err)
	}
	assertOriginal()
	settle(t, a, q)
	if !a.Stale {
		t.Fatal("read failure was not reported")
	}
	assertOriginal()
	a.Service.Reader.GitPath = ""
	if err := tt.Click("Browse refs/heads/old"); err != nil {
		t.Fatal(err)
	}
	assertOriginal()
	settle(t, a, q)
	tt.Frame()
	if a.HistoryRef != "refs/heads/old" || len(a.GraphRows) != 1 || a.GraphRows[0].Commit.Subject != "snapshot base" || a.Stale || tt.HasText("snapshot tip") {
		t.Fatal("new scope and rows were not applied together", a.HistoryRef, tt.Texts())
	}
	if a.CommitDetails.Commit.OID != "" || a.CommitInput != "" {
		t.Fatal("previous scope's detail selection survived replacement")
	}
}

func TestCommitDiffRetainsPreviewDuringReplacementAndFailure(t *testing.T) {
	dir := testgit.Repo(t)
	for _, subject := range []string{"preview base", "preview tip"} {
		testgit.Write(t, filepath.Join(dir, "file"), subject+"\n")
		testgit.Run(t, dir, "add", "file")
		testgit.Run(t, dir, "commit", "-m", subject)
	}
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	a.HistoryDetailsOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	tt.Frame()
	if err := tt.Click("preview base"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	originalOID, originalPreview := a.CommitDetails.Commit.OID, a.Preview.Text
	if err := tt.Click("preview tip"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.Loading || a.CommitDetails.Commit.OID != originalOID || a.Preview.Text != originalPreview || !tt.HasText("preview base") {
		t.Fatal("commit selection blanked the displayed preview", tt.Texts())
	}
	settle(t, a, q)
	tt.Frame()
	if a.CommitDetails.Commit.Subject != "preview tip" || a.Preview.Text == originalPreview {
		t.Fatal("replacement was not applied")
	}
	latestOID, latestPreview := a.CommitDetails.Commit.OID, a.Preview.Text
	a.Service.Reader.GitPath = filepath.Join(t.TempDir(), "missing-git")
	if err := tt.Click("preview base"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if !a.Stale || a.CommitDetails.Commit.OID != latestOID || a.Preview.Text != latestPreview {
		t.Fatal("failed detail read discarded the last preview")
	}
}
