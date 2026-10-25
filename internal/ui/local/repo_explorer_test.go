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

// repo_explorer_test.go covers the Repository Explorer sidebar with manually
// filled state and no real Git calls. The full window renders with Pane=="":
// the shared Status pane combined with entries panics on a known defect that
// is owned by the main agent, so these mocks never select it.
func newExplorerApp(t *testing.T) *local.App {
	t.Helper()
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	t.Cleanup(func() { a.Close(); a.Wait() })
	// Explorer content lives inside the repository popover; open it so
	// the mock assertions see the entries.
	a.RepoMenuOpen = true
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
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	// No repository open yet: nothing is marked as current.
	if n := countTexts(tt, "●"); n != 0 {
		t.Fatalf("selection marker without an open repository: %d", n)
	}
	a.Repository = domain.RepositoryContext{Repository: current}
	a.Worktrees = []domain.WorktreeRecord{{Path: current.Path, Branch: "refs/heads/main"}}
	a.Branches = []gitexec.BranchRecord{
		{Name: "refs/heads/main", Current: true},
		{Name: "refs/heads/side", Upstream: "origin/side"},
	}
	tt.Frame()
	// Exactly one recent entry and one workspace entry carry the marker.
	if n := countTexts(tt, "●"); n != 2 {
		t.Fatalf("want 2 selection markers, got %d: %v", n, tt.Texts())
	}
	// The previously established texts and buttons are all still present.
	for _, want := range []string{
		"Recent local repositories",
		strconv.QuoteToGraphic("/fixtures/current"),
		strconv.QuoteToGraphic("/fixtures/other"),
		"Open " + strconv.QuoteToGraphic("/fixtures/current"),
		"Workspace",
		"Workspace path",
		"Workspace folder",
		"Open workspace",
		"Save",
		"New / Save workspace as…",
		"Add folder",
		"Worktrees",
		strconv.QuoteToGraphic(current.Path) + " · refs/heads/main",
		"Branches",
		"HEAD · refs/heads/main",
		"refs/heads/side",
		"Cmd+R refresh",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing text %q in %v", want, tt.Texts())
		}
	}
}

func TestRepositoryExplorerBranchesViewIntact(t *testing.T) {
	a := newExplorerApp(t)
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}}
	a.Worktrees = []domain.WorktreeRecord{{Path: "/fixtures/wt", Branch: "refs/heads/side", Detached: true, Locked: true, LockReason: "locked for reason", Prunable: true, PruneReason: "stale"}}
	a.Branches = []gitexec.BranchRecord{{Name: "refs/heads/main", Current: true, Upstream: "origin/main"}}
	a.Pane = "Branches"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	for _, want := range []string{
		"Branches",
		"refs/heads/main → origin/main",
		"Worktrees",
		"detached=true · locked=true locked for reason · prunable=true stale",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing text %q in %v", want, tt.Texts())
		}
	}
}

func TestRepositoryExplorerOffersLoadBranches(t *testing.T) {
	a := newExplorerApp(t)
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}}
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if !tt.HasText("Not loaded") || !tt.HasText("Worktrees") || !tt.HasText("Branches") {
		t.Fatalf("summary state wrong: %v", tt.Texts())
	}
	// Presence and enabled wiring of the load action (OnClick(a.LoadBranches))
	// cannot be click-verified here: the button sits below the sidebar scroll
	// fold and clicking would run the real Git service.
	if !tt.HasText("Load branches") {
		t.Fatalf("Load branches button missing: %v", tt.Texts())
	}
	// Once loaded, the summary shows the records and keeps its labels.
	a.Worktrees = []domain.WorktreeRecord{{Path: "/fixtures/current", Branch: "refs/heads/main"}}
	a.Branches = []gitexec.BranchRecord{{Name: "refs/heads/main", Current: true}}
	tt.Frame()
	if tt.HasText("Not loaded") || tt.HasText("Load branches") {
		t.Fatalf("loaded summary still shows placeholders: %v", tt.Texts())
	}
	if !tt.HasText("HEAD · refs/heads/main") || !tt.HasText(strconv.QuoteToGraphic("/fixtures/current")+" · refs/heads/main") {
		t.Fatalf("loaded summary missing records: %v", tt.Texts())
	}
	for _, want := range []string{"Worktrees", "Branches", "Recent local repositories", "Cmd+R refresh"} {
		if !tt.HasText(want) {
			t.Fatalf("missing text %q after load: %v", want, tt.Texts())
		}
	}
}
