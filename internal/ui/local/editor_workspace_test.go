package local

import (
	"os"
	"path/filepath"
	"testing"

	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
)

func TestWorkspaceExplorerUsesEveryRoot(t *testing.T) {
	base := t.TempDir()
	alpha := filepath.Join(base, "alpha")
	beta := filepath.Join(base, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(alpha, "src", "alpha.go"), []byte("package alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beta, "src", "beta.go"), []byte("package beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Workspace = domain.WorkspaceView{
		FilePath: filepath.Join(base, "team.code-workspace"),
		Folders: []domain.WorkspaceFolderView{
			{Path: alpha, Name: "Alpha", ResolvedPath: alpha},
			{Path: beta, Name: "Beta", ResolvedPath: beta},
		},
	}

	roots := a.explorerRoots()
	if len(roots) != 2 || roots[0].label != "Alpha" || roots[1].label != "Beta" {
		t.Fatalf("workspace roots = %+v", roots)
	}
	a.OpenQuickOpen()
	if len(a.quickOpenItems) != 2 {
		t.Fatalf("quick open items = %+v", a.quickOpenItems)
	}
	if a.quickOpenItems[0].label != "Alpha/src/alpha.go" || a.quickOpenItems[1].label != "Beta/src/beta.go" {
		t.Fatalf("quick open labels = %+v", a.quickOpenItems)
	}
}
