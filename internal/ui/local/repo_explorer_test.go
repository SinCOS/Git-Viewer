package local_test

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/ui/local"
)

func newExplorerApp(t *testing.T) *local.App {
	t.Helper()
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a
}

func TestToolbarOmitsRepositoryAndWorkspaceMenus(t *testing.T) {
	a := newExplorerApp(t)
	current := domain.Repository{RepositoryID: "probe", Location: "local", Path: "/fixtures/current", DisplayName: "Current"}
	a.Repository = domain.RepositoryContext{Repository: current}
	a.Workspace = domain.WorkspaceView{FilePath: "/fixtures/multi.code-workspace"}
	tt := ui.NewTester(a.View, 1400, 850)
	for _, label := range []string{"Workspace: multi ▾", "Repository: Current ▾"} {
		if tt.HasText(label) {
			t.Fatalf("toolbar still contains %q: %v", label, tt.Texts())
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

func TestAddFolderToWorkspaceUsesDirectoryPicker(t *testing.T) {
	a, queue := setup(t)
	dir := t.TempDir()
	a.ChooseFolder = func() (string, error) { return dir, nil }
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
