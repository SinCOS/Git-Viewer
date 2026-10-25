package local_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func newExplorerApp(t *testing.T) *local.App {
	t.Helper()
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a
}

func countTexts(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == s {
			n++
		}
	}
	return n
}

func TestRepositoryExplorerMarksCurrentRepository(t *testing.T) {
	a := newExplorerApp(t)
	current := domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}
	other := domain.Repository{RepositoryID: "other", Location: "local", Path: "/fixtures/other", DisplayName: "Other"}
	a.Recent = []domain.Repository{other, current}
	a.WorkspaceRepositories = []domain.RepositoryContext{{Repository: other}, {Repository: current}}
	a.RepoMenuOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	if n := countTexts(tt, "●"); n != 0 {
		t.Fatalf("selection marker without an open repository: %d", n)
	}
	a.Repository = domain.RepositoryContext{Repository: current}
	tt.Frame()
	// The current repository is marked in both the workspace group and Recent.
	if n := countTexts(tt, "●"); n != 2 {
		t.Fatalf("want current repository marked in workspace and recent groups, got %d: %v", n, tt.Texts())
	}
	for _, want := range []string{"Workspace folders", "Open " + strconv.QuoteToGraphic("/fixtures/current"), "Recent local repositories", strconv.QuoteToGraphic("/fixtures/current"), strconv.QuoteToGraphic("/fixtures/other"), "Open path"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q: %v", want, tt.Texts())
		}
	}
	a.RepoMenuOpen = false
	a.WorkspaceMenuOpen = true
	tt.Frame()
	if countTexts(tt, "●") != 1 {
		t.Fatal("workspace selection marker missing", tt.Texts())
	}
	for _, want := range []string{"Open " + strconv.QuoteToGraphic("/fixtures/current"), "Workspace", "Workspace path", "Workspace folder", "Open workspace", "Save", "New / Save workspace as…", "Add folder"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q: %v", want, tt.Texts())
		}
	}
}

func TestAddToWorkspaceQueuesFoldersWithoutDuplicates(t *testing.T) {
	a := newExplorerApp(t)
	a.AddToWorkspace("")
	if len(a.WorkspaceFolders) != 0 {
		t.Fatal("empty path added")
	}
	a.AddToWorkspace("/fixtures/current")
	a.AddToWorkspace("/fixtures/current")
	a.AddToWorkspace("/fixtures/other")
	if len(a.WorkspaceFolders) != 2 || a.WorkspaceFolders[0].Path != "/fixtures/current" || a.WorkspaceFolders[1].Path != "/fixtures/other" {
		t.Fatal(a.WorkspaceFolders)
	}
	if a.Message != "Added to workspace · save to persist" {
		t.Fatal(a.Message)
	}
}

func TestRepositoryExplorerOffersAddToWorkspace(t *testing.T) {
	a := newExplorerApp(t)
	current := domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}
	a.Repository = domain.RepositoryContext{Repository: current}
	a.Locations = []domain.SavedLocation{{Kind: "local", Path: "/fixtures/kept"}, {Kind: "ssh", Host: "host", Path: "/remote"}}
	a.WorkspaceMenuOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	if err := tt.Click("Add current repository"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.WorkspaceFolders) != 1 || a.WorkspaceFolders[0].Path != "/fixtures/current" {
		t.Fatal(a.WorkspaceFolders)
	}
	if err := tt.Click("Add " + strconv.QuoteToGraphic("/fixtures/kept")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.WorkspaceFolders) != 2 || a.WorkspaceFolders[1].Path != "/fixtures/kept" {
		t.Fatal(a.WorkspaceFolders)
	}
	a.WorkspaceMenuOpen = false
	a.RepoMenuOpen = true
	tt.Frame()
	if err := tt.Click("Add current repository to workspace"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.WorkspaceFolders) != 2 {
		t.Fatalf("duplicate add: %v", a.WorkspaceFolders)
	}
}

func TestAddFolderToWorkspaceUsesDirectoryPicker(t *testing.T) {
	a, queue := setup(t)
	dir := t.TempDir()
	a.ChooseDirectory = func() (string, error) { return dir, nil }
	a.AddFolderToWorkspace()
	if !a.DialogOpen {
		t.Fatal("dialog was not tracked")
	}
	a.Wait()
	for len(queue) > 0 {
		(<-queue)()
	}
	if a.DialogOpen {
		t.Fatal("dialog wait did not finish")
	}
	if len(a.WorkspaceFolders) != 1 || a.WorkspaceFolders[0].Path != dir {
		t.Fatal(a.WorkspaceFolders)
	}
}

func TestRepositoryExplorerBranchesViewIntact(t *testing.T) {
	a := newExplorerApp(t)
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}}
	a.Worktrees = []domain.WorktreeRecord{
		{Path: "/fixtures/wt", Branch: "refs/heads/side", Detached: true, Locked: true, LockReason: "locked for reason", Prunable: true, PruneReason: "stale"},
		{Path: "/fixtures/other", Branch: "refs/heads/other", Head: "abc123"},
	}
	a.Branches = []gitexec.BranchRecord{{Name: "refs/heads/main", Current: true, Upstream: "origin/main"}}
	a.Pane = "Branches"
	tt := ui.NewTester(a.View, 1400, 850)
	for _, want := range []string{"Worktree List", "Worktree Details", "Branch: side", "detached=true · locked=true locked for reason · prunable=true stale"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q: %v", want, tt.Texts())
		}
	}
	if tt.HasText("Diff Preview") || tt.HasText("Commit message") {
		t.Fatal("other mode panels leaked into worktrees")
	}
	if err := tt.Click("other"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Branch: other") || !tt.HasText("HEAD: abc123") || a.SelectedWorktree != 1 {
		t.Fatal(tt.Texts())
	}
}

func TestRepositoryExplorerOffersLoadBranches(t *testing.T) {
	a := newExplorerApp(t)
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}}
	a.Pane = "Branches"
	tt := ui.NewTester(a.View, 1400, 850)
	for _, want := range []string{"Not loaded", "Load branches", "Worktree List", "Select a worktree"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q: %v", want, tt.Texts())
		}
	}
	a.Worktrees = []domain.WorktreeRecord{{Path: "/fixtures/current", Branch: "refs/heads/main"}}
	tt.Frame()
	if tt.HasText("Not loaded") || !tt.HasText("Branch: main") {
		t.Fatal(tt.Texts())
	}
}
