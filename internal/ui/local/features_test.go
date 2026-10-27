package local_test

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutoRefreshPreservesSelectionAndUpdatesDiff(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "before\n")
	a, q := setup(t)
	a.EnableAuto = true
	a.Open(dir)
	settle(t, a, q)
	a.Select(0)
	settle(t, a, q)
	testgit.Write(t, filepath.Join(dir, "file"), "after\n")
	deadline := time.After(5 * time.Second)
	for !strings.Contains(a.Preview.Text, "+after") {
		select {
		case apply := <-q:
			apply()
		case <-deadline:
			t.Fatal("native refresh missed update", a.Message, a.Preview)
		}
	}
	if a.Selected != 0 || a.Preview.Path != "file" || a.Stale {
		t.Fatal(a.Selected, a.Preview, a.Message)
	}
}
func TestOpenKeepsCurrentView(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	a, q := setup(t)
	a.Open(first)
	settle(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	a.Open(second)
	settle(t, a, q)
	if a.Pane != "History" || a.Mode != "log" {
		t.Fatalf("opening a repository reset the view: pane=%s mode=%s", a.Pane, a.Mode)
	}
	resolved, _ := filepath.EvalSymlinks(second)
	if a.Repository.Repository.Path != resolved {
		t.Fatal(a.Repository.Repository.Path)
	}
	tt.Frame()
	if !tt.HasText("Git Log") || tt.HasText("Local Changes") {
		t.Fatal(tt.Texts())
	}
}
func TestOpenWorkspaceOpensFirstRepository(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	a, q := setup(t)
	path := filepath.Join(t.TempDir(), "auto.code-workspace")
	a.ChooseWorkspaceSave = func() (string, error) { return path, nil }
	a.WorkspaceFolders = []domain.WorkspaceFolder{{Path: first}, {Path: second}}
	a.SaveWorkspace(true)
	settle(t, a, q)
	if len(a.WorkspaceRepositories) != 2 {
		t.Fatal(a.WorkspaceRepositories)
	}
	// The first repository opens after the background status load finishes.
	deadline := time.After(10 * time.Second)
	for a.Repository.Repository.RepositoryID == "" {
		select {
		case apply := <-q:
			apply()
		case <-deadline:
			t.Fatal("first workspace repository did not open")
		}
	}
	resolvedFirst, _ := filepath.EvalSymlinks(first)
	if a.Repository.Repository.Path != resolvedFirst {
		t.Fatalf("want first workspace repository open, got %s", a.Repository.Repository.Path)
	}
}
func TestHistoryGraphVirtualization(t *testing.T) {
	a, _ := setup(t)
	a.Pane = "History"
	commits := make([]domain.CommitRecord, 10000)
	for i := range commits {
		commits[i] = domain.CommitRecord{OID: fmt.Sprintf("%040x", 10000-i), Subject: fmt.Sprintf("row %05d", i)}
		if i+1 < len(commits) {
			commits[i].Parents = []string{fmt.Sprintf("%040x", 9999-i)}
		}
	}
	a.GraphRows = a.Graph.Append(commits)
	tt := ui.NewTester(a.View, 1400, 850)
	if a.HistoryBuiltRows > 80 {
		t.Fatal("history not virtualized", a.HistoryBuiltRows)
	}
	a.HistoryList.ScrollTo(9999, ui.Center)
	tt.Frame()
	if !tt.HasText("row 09999") || a.HistoryBuiltRows > 80 {
		t.Fatal(a.HistoryBuiltRows, tt.Texts())
	}
}
func TestNativeWriteHistoryWorkspaceFlow(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Run(t, dir, "config", "user.name", "UI Test")
	testgit.Run(t, dir, "config", "user.email", "ui@example.invalid")
	testgit.Write(t, filepath.Join(dir, "file"), "new\n")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	tt := ui.NewTester(a.View, 1600, 1000)
	a.Select(0)
	settle(t, a, q)
	a.RunWrite(domain.Stage)
	if a.Loading {
		t.Fatal("write without confirmation")
	}
	confirmed := 0
	a.Confirm = func(title, detail string) bool { confirmed++; return true }
	if err := tt.Click(`More actions for "file"`); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stage file"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if len(a.Entries) != 1 || a.Entries[0].Scope != domain.StagedDiff {
		t.Fatal(a.Entries, a.Message)
	}
	a.MessageInput = "native commit"
	// The commit action uses the same controlled write flow.
	a.RunWrite(domain.Commit)
	settle(t, a, q)
	tt.Frame()
	if len(a.Entries) != 0 || !strings.HasPrefix(a.Message, "Completed") {
		t.Fatal(a.Entries, a.Message)
	}
	a.ConnectionMenuOpen = true
	tt.Frame()
	if err := tt.Click("History / Graph"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	a.ConnectionMenuOpen = false
	tt.Frame()
	if len(a.GraphRows) != 1 || !tt.HasText("native commit") {
		t.Fatal(a.GraphRows, tt.Texts())
	}
	if err := tt.Click("native commit"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	tt.Frame()
	if !strings.Contains(a.Preview.Text, "+new") {
		t.Fatal(a.Preview, a.Message)
	}
	a.LoadBranches()
	settle(t, a, q)
	tt.Frame()
	if len(a.Branches) != 1 || len(a.Worktrees) != 1 {
		t.Fatal(a.Branches, a.Worktrees)
	}
	path := filepath.Join(t.TempDir(), "native.code-workspace")
	a.ChooseWorkspaceSave = func() (string, error) { return path, nil }
	a.WorkspaceFolders = []domain.WorkspaceFolder{{Path: dir}}
	a.SaveWorkspace(true)
	settle(t, a, q)
	if len(a.WorkspaceRepositories) != 1 || a.Workspace.FilePath != path {
		t.Fatal(a.Workspace, a.Message)
	}
	a.ChooseWorkspace = func() (string, error) { return path, nil }
	tt.Frame()
	a.WorkspaceMenuOpen = true // workspace controls have their own popover
	tt.Frame()
	if err := tt.Click("Open workspace"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, q)
	testgit.Write(t, path, `{"folders":[],"external":true}`)
	a.WorkspaceFolders = append(a.WorkspaceFolders, domain.WorkspaceFolder{Path: "other"})
	a.SaveWorkspace(false)
	settle(t, a, q)
	if !strings.HasPrefix(a.Message, "WorkspaceConflict") {
		t.Fatal(a.Message)
	}
	if confirmed != 2 {
		t.Fatal("confirmation count", confirmed)
	}
}
