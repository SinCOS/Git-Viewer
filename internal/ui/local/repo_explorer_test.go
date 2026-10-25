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
	if n := countTexts(tt, "●"); n != 1 {
		t.Fatalf("want one current recent repository, got %d: %v", n, tt.Texts())
	}
	for _, want := range []string{"Recent local repositories", strconv.QuoteToGraphic("/fixtures/current"), strconv.QuoteToGraphic("/fixtures/other"), "Open path"} {
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
