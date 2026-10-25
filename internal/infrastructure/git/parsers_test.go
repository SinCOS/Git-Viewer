package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
)

func read(t *testing.T, dir string, op domain.Operation) []byte {
	t.Helper()
	r, err := (gitexec.LocalExecutor{}).Execute(context.Background(), testgit.Command(dir, op))
	if err != nil {
		t.Fatalf("%v: %s", err, r.Stderr)
	}
	return r.Stdout
}
func TestStatusRealRepository(t *testing.T) {
	dir := testgit.Repo(t)
	empty, err := gitexec.ParseStatus(read(t, dir, domain.Status))
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	original := "old 中文\n' file"
	renamed := "new 中文\n' file"
	for _, name := range []string{original, "both", "deleted", "conflict"} {
		testgit.Write(t, filepath.Join(dir, name), "base\n")
	}
	testgit.Run(t, dir, "add", "--all")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Run(t, dir, "mv", "--", original, renamed)
	testgit.Write(t, filepath.Join(dir, "both"), "staged\n")
	testgit.Run(t, dir, "add", "--", "both")
	testgit.Write(t, filepath.Join(dir, "both"), "unstaged\n")
	if err := os.Remove(filepath.Join(dir, "deleted")); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "-untracked\n中文"), "data")
	rows, err := gitexec.ParseStatus(read(t, dir, domain.Status))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]domain.Change{}
	for _, r := range rows {
		found[r.Path] = r
	}
	r := found[renamed]
	if r.OldPath == nil || *r.OldPath != original || r.IndexStatus != "R" {
		t.Fatalf("rename %+v", r)
	}
	if r := found["both"]; r.IndexStatus != "M" || r.WorktreeStatus != "M" {
		t.Fatalf("both %+v", r)
	}
	if r := found["deleted"]; r.WorktreeStatus != "D" {
		t.Fatalf("deleted %+v", r)
	}
	if !found["-untracked\n中文"].Untracked {
		t.Fatal("untracked lost")
	}
	// Build a real conflicted index in a separate fixture.
	conflict := testgit.Repo(t)
	testgit.Write(t, filepath.Join(conflict, "file"), "base\n")
	testgit.Run(t, conflict, "add", ".")
	testgit.Run(t, conflict, "commit", "-m", "base")
	testgit.Run(t, conflict, "checkout", "-b", "side")
	testgit.Write(t, filepath.Join(conflict, "file"), "side\n")
	testgit.Run(t, conflict, "commit", "-am", "side")
	testgit.Run(t, conflict, "checkout", "main")
	testgit.Write(t, filepath.Join(conflict, "file"), "main\n")
	testgit.Run(t, conflict, "commit", "-am", "main")
	// merge is intentionally expected to fail; it only modifies this fixture.
	result := exec.Command("git", "-C", conflict, "merge", "side").Run()
	if result == nil {
		t.Fatal("expected fixture merge conflict")
	}
	rows, err = gitexec.ParseStatus(read(t, conflict, domain.Status))
	if err != nil || len(rows) != 1 || !rows[0].Conflicted {
		t.Fatal(rows, err)
	}
}
func TestHistoryAndWorktreesRealRepository(t *testing.T) {
	dir := testgit.Repo(t)
	if rows, err := gitexec.ParseHistory(nil); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	// Unborn repositories fail git log distinctly; never treat command errors as parsed history.
	result, err := (gitexec.LocalExecutor{}).Execute(context.Background(), testgit.Command(dir, domain.History))
	if err == nil || result.ExitCode == 0 {
		t.Fatal("unborn log must report command failure")
	}
	testgit.Write(t, filepath.Join(dir, "base"), "base")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "root 中文")
	testgit.Run(t, dir, "checkout", "-b", "side")
	testgit.Write(t, filepath.Join(dir, "side"), "side")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "side")
	testgit.Run(t, dir, "checkout", "main")
	testgit.Write(t, filepath.Join(dir, "main"), "main")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "main")
	testgit.Run(t, dir, "merge", "--no-ff", "side", "-m", "merge 中文")
	rows, err := gitexec.ParseHistory(read(t, dir, domain.History))
	if err != nil || len(rows) != 4 || len(rows[0].Parents) != 2 || rows[0].Subject != "merge 中文" {
		t.Fatal(rows, err)
	}
	linked := filepath.Join(t.TempDir(), "worktree 中文\n' path")
	testgit.Run(t, dir, "worktree", "add", "--detach", linked, "HEAD")
	testgit.Run(t, dir, "worktree", "lock", "--reason", "保留\nreason", linked)
	worktrees, err := gitexec.ParseWorktrees(read(t, dir, domain.Worktrees))
	if err != nil || len(worktrees) != 2 {
		t.Fatal(worktrees, err)
	}
	canonical, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !worktrees[1].Detached || !worktrees[1].Locked || worktrees[1].LockReason != "保留\nreason" || worktrees[1].Path != canonical {
		t.Fatalf("%+v", worktrees)
	}
	bare := t.TempDir()
	testgit.Run(t, bare, "init", "--bare")
	worktrees, err = gitexec.ParseWorktrees(read(t, bare, domain.Worktrees))
	if err != nil || len(worktrees) != 1 || !worktrees[0].Bare {
		t.Fatal(worktrees, err)
	}
}
func TestMalformedProtocols(t *testing.T) {
	hash := strings.Repeat("a", 40)
	for _, data := range []string{"?? missing terminator", "M\x00", "?? \x00", "RX bad\x00", "R  new\x00", "?M bad\x00"} {
		if _, err := gitexec.ParseStatus([]byte(data)); err == nil {
			t.Fatalf("status accepted %q", data)
		}
	}
	for _, data := range []string{"bad\x00", "bad\x00\x00author\x002026-01-01T00:00:00Z\x00subject\x00\x00", hash + "\x00bad\x00author\x00bad-date\x00subject\x00refs\x00"} {
		if _, err := gitexec.ParseHistory([]byte(data)); err == nil {
			t.Fatalf("history accepted %q", data)
		}
	}
	for _, data := range []string{"worktree /tmp\x00", "HEAD " + hash + "\x00\x00", "worktree /tmp\x00HEAD bad\x00\x00", "worktree /tmp\x00bare\x00bare\x00\x00", "worktree /tmp\x00mystery\x00\x00"} {
		if _, err := gitexec.ParseWorktrees([]byte(data)); err == nil {
			t.Fatalf("worktree accepted %q", data)
		}
	}
}
func FuzzParsers(f *testing.F) {
	f.Add([]byte("?? 中文\nfile\x00"))
	f.Add([]byte("worktree /tmp\x00bare\x00\x00"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = gitexec.ParseStatus(b)
		_, _ = gitexec.ParseHistory(b)
		_, _ = gitexec.ParseWorktrees(b)
	})
}
