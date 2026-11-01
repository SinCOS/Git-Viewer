package local

import (
	"context"
	"gitviewer/internal/application"
	gitexec "gitviewer/internal/infrastructure/git"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/workspace"
	"gitviewer/internal/testgit"
)

func workspaceFixture(t *testing.T, roots ...string) string {
	t.Helper()
	folders := make([]domain.WorkspaceFolder, len(roots))
	for i, root := range roots {
		folders[i] = domain.WorkspaceFolder{Path: root}
	}
	file := filepath.Join(t.TempDir(), "team.code-workspace")
	if _, err := workspace.Create(file, folders); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestWorkspaceReplacesPreviousGitPanels(t *testing.T) {
	old, next := testgit.Repo(t), testgit.Repo(t)
	oldOID := commitFixture(t, old, "old.txt", "old history")
	nextOID := commitFixture(t, next, "next.txt", "workspace history")
	testgit.Write(t, filepath.Join(next, "dirty.txt"), "dirty\n")
	a, q := newRepositoryApp(t)
	a.Open(old)
	settleSync(t, a, q)
	a.LoadHistory(false)
	settleSync(t, a, q)
	a.CommitInput = oldOID
	a.CommitDiff()
	settleSync(t, a, q)
	a.MessageInput = "old draft"
	a.Amend = true
	a.OpenWorkspace(workspaceFixture(t, next))
	settleSync(t, a, q)
	if len(a.Entries) != 1 || a.Entries[0].Change.Path != "dirty.txt" || len(a.GraphRows) != 0 || a.CommitDetails.Commit.OID != "" || a.MessageInput != "" || a.Amend {
		t.Fatalf("old repository state survived workspace open: repo=%+v entries=%+v history=%d", a.Repository, a.Entries, len(a.GraphRows))
	}
	a.LoadHistory(false)
	settleSync(t, a, q)
	if len(a.GraphRows) != 1 || a.GraphRows[0].Commit.OID != nextOID {
		t.Fatal("workspace history did not load", a.GraphRows)
	}
}

func TestLateWorkspaceResultCannotReplaceNewerWorkspace(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	commitFixture(t, first, "one", "first")
	commitFixture(t, second, "two", "second")
	a, q := newRepositoryApp(t)
	a.OpenWorkspace(workspaceFixture(t, first))
	late := <-q
	a.OpenWorkspace(workspaceFixture(t, second))
	settleSync(t, a, q)
	active := a.Repository.Repository.RepositoryID
	late()
	if a.Repository.Repository.RepositoryID != active || len(a.WorkspaceRepositories) != 1 || a.WorkspaceRepositories[0].Repository.RepositoryID != active {
		t.Fatal("late workspace result overwrote new selection")
	}
}

func TestWorkspaceWriteTargetsSelectedRepository(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	commitFixture(t, first, "one", "first")
	commitFixture(t, second, "two", "second")
	testgit.Write(t, filepath.Join(first, "dirty-first"), "first\n")
	testgit.Write(t, filepath.Join(second, "dirty-second"), "second\n")
	a, q := newRepositoryApp(t)
	a.OpenWorkspace(workspaceFixture(t, first, second))
	settleSync(t, a, q)
	a.selectWorkspaceRepository(a.WorkspaceRepositories[1].Repository.RepositoryID)
	settleSync(t, a, q)
	a.stageEntry(0, true)
	settleSync(t, a, q)
	if got := string(testgit.Run(t, first, "diff", "--cached", "--name-only")); got != "" {
		t.Fatal("write leaked to first repo", got)
	}
	if len(a.Entries) != 1 || a.Entries[0].Scope != domain.StagedDiff || a.Entries[0].Change.Path != "dirty-second" {
		t.Fatal("selected repository did not refresh after write", a.Entries)
	}
}

func TestRemoteWorkspaceRepositorySelectionKeepsRemoteTransport(t *testing.T) {
	a, q := newRepositoryApp(t)
	first := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "ssh-first", Location: "ssh", Path: "/first"}, SSHHost: "fixture"}
	second := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "ssh-second", Location: "ssh", Path: "/second"}, SSHHost: "fixture"}
	a.MessageInput = "old draft"
	a.Amend = true
	a.applyRemoteWorkspace(application.RemoteWorkspaceOpenResult{Repositories: []domain.RepositoryContext{first, second}}, "fixture", "/team.code-workspace", &domain.GitStatus{RepositoryID: "ssh-first"})
	if a.MessageInput != "" || a.Amend {
		t.Fatal("remote workspace retained old commit draft")
	}
	a.Pane = "Status"
	// A broken remote must fail, not read the local reader's fake success.
	a.Service.Reader = gitexec.LocalReader{Transport: func(context.Context, string, int, []byte, ...string) (domain.GitResult, error) {
		t.Error("SSH repository fell back to local transport")
		return domain.GitResult{}, nil
	}}
	a.Service.Remote.SSHPath = "/usr/bin/false"
	a.selectWorkspaceRepository(second.Repository.RepositoryID)
	settleSync(t, a, q)
	if !a.Stale || a.Repository.Repository.RepositoryID != "ssh-first" {
		t.Fatal("failed remote switch replaced current repository", a.Repository, a.Stale)
	}
}

func TestWorkspaceWithoutGitClearsOldRepository(t *testing.T) {
	dir := testgit.Repo(t)
	commitFixture(t, dir, "old", "old")
	a, q := newRepositoryApp(t)
	a.Open(dir)
	settleSync(t, a, q)
	a.LoadHistory(false)
	settleSync(t, a, q)
	a.OpenWorkspace(workspaceFixture(t, t.TempDir()))
	settleSync(t, a, q)
	if a.Repository.Repository.RepositoryID != "" || len(a.Entries) != 0 || len(a.GraphRows) != 0 {
		t.Fatal("non-Git workspace retained previous Git panels")
	}
}

func TestWorkspaceRepositorySelectorRoutesGitPanels(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	commitFixture(t, first, "one", "first")
	secondOID := commitFixture(t, second, "two", "second")
	a, q := newRepositoryApp(t)
	a.OpenWorkspace(workspaceFixture(t, first, second))
	settleSync(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1200, 850)
	tt.Frame()
	settleSync(t, a, q)
	tt.Frame()
	if err := tt.Click("Select Git repository"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(a.WorkspaceRepositories[1].Repository.Path); err != nil {
		t.Fatal(err)
	}
	settleSync(t, a, q)
	tt.Frame()
	settleSync(t, a, q)
	if a.Repository.Repository.RepositoryID != a.WorkspaceRepositories[1].Repository.RepositoryID || len(a.GraphRows) != 1 || a.GraphRows[0].Commit.OID != secondOID {
		t.Fatal("repository selection did not switch history", a.Repository, a.GraphRows)
	}
	// Reloading/saving the same workspace keeps the active member, but refreshes it.
	testgit.Write(t, filepath.Join(second, "new"), "new\n")
	a.OpenWorkspace(a.Workspace.FilePath)
	settleSync(t, a, q)
	if a.Repository.Repository.RepositoryID != a.WorkspaceRepositories[1].Repository.RepositoryID || len(a.Entries) != 1 {
		t.Fatal("workspace reload lost active repository or failed to refresh")
	}
	a.Pane = "Status"
	tt.Frame()
	if _, ok := tt.Find("Select Git repository"); !ok {
		t.Fatal("Commit has no repository selector")
	}
	// Removing the active root on save selects the remaining member.
	a.WorkspaceFolders = a.WorkspaceFolders[:1]
	a.SaveWorkspace(false)
	settleSync(t, a, q)
	if len(a.WorkspaceRepositories) != 1 || a.Repository.Repository.RepositoryID != a.WorkspaceRepositories[0].Repository.RepositoryID {
		t.Fatal("saving removed active root left the old repository selected")
	}
}

func TestWorkspaceRepositorySelectorLooksLikeField(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	a, q := newRepositoryApp(t)
	a.OpenWorkspace(workspaceFixture(t, first, second))
	settleSync(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1200, 850)
	tt.Frame()
	settleSync(t, a, q)
	tt.Frame()
	selector, ok := tt.Find("Select Git repository")
	if !ok {
		t.Fatal("multi-repository workspace has no selector")
	}
	sidebar, ok := tt.Find("Branch navigation")
	if !ok {
		t.Fatal("branch navigation missing")
	}
	if selector.H != 30 || selector.X != sidebar.X+8 || selector.W != sidebar.W-16 {
		t.Fatalf("selector must sit inset above the branch panel: selector=%+v panel=%+v", selector, sidebar)
	}
	if !tt.HasText(a.Repository.Repository.DisplayName) {
		t.Fatal("selector must name the active repository", tt.Texts())
	}
}
