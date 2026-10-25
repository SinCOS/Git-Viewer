package local

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func TestWorkspaceBarListsRepositoriesWithCounts(t *testing.T) {
	first := testgit.Repo(t)
	testgit.Write(t, filepath.Join(first, "one.txt"), "one\n")
	testgit.Run(t, first, "add", "one.txt")
	second := testgit.Repo(t)
	testgit.Write(t, filepath.Join(second, "two.txt"), "two\n")
	testgit.Run(t, second, "add", "two.txt")
	testgit.Run(t, second, "commit", "-m", "clean")

	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 64)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })

	ws := filepath.Join(t.TempDir(), "multi.code-workspace")
	a.ChooseWorkspaceSave = func() (string, error) { return ws, nil }
	a.WorkspaceFolders = []domain.WorkspaceFolder{{Path: first}, {Path: second}}
	a.SaveWorkspace(true)
	settleSync(t, a, queue)

	if len(a.WorkspaceRepositories) != 2 {
		t.Fatal(a.WorkspaceRepositories)
	}
	if a.Repository.Repository.Path != a.WorkspaceRepositories[0].Repository.Path {
		t.Fatalf("first workspace repository did not open: %s", a.Repository.Repository.Path)
	}
	if got := a.workspaceChanges[a.WorkspaceRepositories[0].Repository.RepositoryID]; got != 1 {
		t.Fatalf("want 1 change in the dirty repository, got %d", got)
	}
	if got := a.workspaceChanges[a.WorkspaceRepositories[1].Repository.RepositoryID]; got != 0 {
		t.Fatalf("want 0 changes in the clean repository, got %d", got)
	}
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if !tt.HasText("Workspace repositories") {
		t.Fatal(tt.Texts())
	}
	target := a.WorkspaceRepositories[1]
	if err := tt.Click(target.Repository.DisplayName); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for a.Repository.Repository.Path != target.Repository.Path {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatal("switching repositories did not finish")
		}
	}
}
