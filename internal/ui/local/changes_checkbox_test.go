package local_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestCheckboxSelectsOnlyChosenFilesForCommit(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "chosen.txt"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, filepath.Join(dir, "chosen.txt"), "changed\n")
	testgit.Write(t, filepath.Join(dir, "excluded.txt"), "excluded\n")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	tt := ui.NewTester(a.View, 1400, 850)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(t, a, q)
		tt.Frame()
	}
	// Routine checkboxes stage and unstage immediately, like VS Code.
	click(`Check Stage file "chosen.txt"`)
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "diff", "--cached", "--name-only"))); got != "chosen.txt" {
		t.Fatal(got)
	}
	// Staged rows start checked: clicking one must remove it from the index.
	click(`Check Unstage file "chosen.txt"`)
	if got := string(testgit.Run(t, dir, "diff", "--cached", "--name-only")); got != "" {
		t.Fatal(got)
	}
	click(`Check Stage file "chosen.txt"`)
	a.MessageInput = "selected file"
	tt.Frame()
	click("Commit")
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "show", "--pretty=format:", "--name-only", "HEAD"))); got != "chosen.txt" {
		t.Fatalf("commit included unselected files: %q", got)
	}
	if got := string(testgit.Run(t, dir, "status", "--porcelain")); !strings.Contains(got, "?? excluded.txt") {
		t.Fatalf("unselected file changed: %q", got)
	}
}
