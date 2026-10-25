package local_test

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
	"gitviewer/internal/ui/local"
)

func setup(t *testing.T) (*local.App, chan func()) {
	t.Helper()
	a := local.New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a, queue
}
func settle(t *testing.T, a *local.App, queue chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for a.Loading || a.DialogOpen {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatal("background operation did not finish")
		}
	}
}
func TestLocalReadOnlyEndToEnd(t *testing.T) {
	dir := testgit.Repo(t)
	name := "中文 space\n-file"
	testgit.Write(t, filepath.Join(dir, name), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	oid := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Write(t, filepath.Join(dir, name), "staged\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Write(t, filepath.Join(dir, name), "unstaged\n")
	testgit.Write(t, filepath.Join(dir, "new file"), "untracked\n")
	before := testgit.Run(t, dir, "status", "--porcelain=v1", "-z")
	indexBefore := testgit.Run(t, dir, "ls-files", "--stage", "-z")
	a, queue := setup(t)
	a.RepoMenuOpen = true
	tt := ui.NewTester(a.View, 1400, 850)
	a.PathInput = dir
	if err := tt.Click("Open path"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	a.RepoMenuOpen = false
	tt.Frame()
	if a.Stale || len(a.Entries) != 3 || len(a.Recent) != 1 || !tt.HasText("Staged Changes") || !tt.HasText("Changes") || !tt.HasText("Untracked Files") {
		t.Fatalf("%+v %s %v", a.Entries, a.Message, tt.Texts())
	}
	for i, entry := range a.Entries {
		if i == 0 {
			label := strconv.QuoteToGraphic(name)
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		} else {
			a.Select(i)
		}
		settle(t, a, queue)
		tt.Frame()
		want := "+unstaged"
		if entry.Scope == domain.StagedDiff {
			want = "+staged"
		}
		if entry.Group == "Untracked" {
			want = "+untracked"
		}
		if !strings.Contains(a.Preview.Text, want) || !tt.HasText(want) {
			t.Fatalf("%+v %v", a.Preview, tt.Texts())
		}
	}
	a.CommitInput = oid
	a.CommitDiff()
	settle(t, a, queue)
	tt.Frame()
	if a.Preview.Scope != domain.CommitDiff || !strings.Contains(a.Preview.Text, "+base") {
		t.Fatalf("%+v", a.Preview)
	}
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	if string(before) != string(testgit.Run(t, dir, "status", "--porcelain=v1", "-z")) || string(indexBefore) != string(testgit.Run(t, dir, "ls-files", "--stage", "-z")) {
		t.Fatal("read-only workflow changed repository")
	}
	// Invalid opens keep the current data and explicitly mark it stale.
	a.Open(t.TempDir())
	settle(t, a, queue)
	tt.Frame()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Stale || a.Message != "InvalidRepository" || a.Repository.Repository.Path != canonical || len(a.Entries) != 3 {
		t.Fatalf("%s stale=%t", a.Message, a.Stale)
	}
	a.Refresh()
	settle(t, a, queue)
	if a.Stale {
		t.Fatal("refresh did not clear stale")
	}
	a.Cancel()
	a.Refresh()
	a.Cancel()
	if a.Loading || !a.Stale || a.Message != "OperationCancelled" {
		t.Fatal("cancel state incorrect")
	}
	a.Wait()
	for len(queue) > 0 {
		(<-queue)()
	}
	if a.Loading || a.Message != "OperationCancelled" {
		t.Fatal("late result overwrote cancellation")
	}
	reopened := local.New(a.Service)
	if len(reopened.Recent) != 1 || reopened.Recent[0].Path != canonical {
		t.Fatal("recent repository not persisted")
	}
}

func TestEmptyAndConflict(t *testing.T) {
	a, queue := setup(t)
	empty := testgit.Repo(t)
	a.Open(empty)
	settle(t, a, queue)
	tt := ui.NewTester(a.View, 1400, 850)
	if !tt.HasText("Working tree clean") {
		t.Fatal(tt.Texts())
	}
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Run(t, dir, "checkout", "-b", "side")
	testgit.Write(t, filepath.Join(dir, "file"), "side\n")
	testgit.Run(t, dir, "commit", "-am", "side")
	testgit.Run(t, dir, "checkout", "main")
	testgit.Write(t, filepath.Join(dir, "file"), "main\n")
	testgit.Run(t, dir, "commit", "-am", "main")
	// Invoke a real merge conflict; only the fixture performs Git writes.
	cmd := exec.Command("git", "-C", dir, "merge", "side")
	cmd.Env = git.CleanEnvironment()
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("expected merge conflict: %s %v", out, err)
	}
	a.Open(dir)
	settle(t, a, queue)
	tt.Frame()
	if len(a.Entries) != 1 || a.Entries[0].Group != "Conflicted" || !tt.HasText("Conflicted: 1") {
		t.Fatalf("%+v", a.Entries)
	}
	a.Select(0)
	settle(t, a, queue)
	if a.Message != "Ready" || !strings.Contains(a.Preview.Text, "diff --cc") {
		t.Fatalf("%+v %s", a.Preview, a.Message)
	}
}
