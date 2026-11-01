package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVersionFloor(t *testing.T) {
	for _, v := range []string{"git version 2.13.0", "git version 2.25.1", "git version 2.38.0", "git version 2.39.5 (Apple Git-154)", "git version 3.0.0"} {
		if err := gitexec.CheckVersion(v); err != nil {
			t.Fatal(v, err)
		}
	}
	for _, v := range []string{"git version 1.9", "git version 2.12.5", "unexpected", "git version 2.x"} {
		if err := gitexec.CheckVersion(v); err == nil {
			t.Fatal(v)
		}
	}
}
func TestCommitUsesUserIdentityAndExactRenameDeletePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	testgit.Write(t, filepath.Join(home, ".gitconfig"), "[user]\n name = Global Identity\n email = global@example.invalid\n")
	dir := testgit.Repo(t)
	r := gitexec.LocalReader{}
	repo, err := r.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"old", "keep"} {
		testgit.Write(t, filepath.Join(dir, p), "initial\n")
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"old", "keep"}})
	perform(t, r, repo, domain.GitOperation{Operation: domain.Commit, Message: "user identity"})
	details, err := r.CommitDetails(context.Background(), repo, stringTrim(testgit.Run(t, dir, "rev-parse", "HEAD")))
	if err != nil || details.Commit.Author != "Global Identity" || len(details.Files) != 2 {
		t.Fatal(details, err)
	}
	testgit.Run(t, dir, "mv", "old", "new")
	testgit.Write(t, filepath.Join(dir, "new"), "initial\nupdated\n")
	perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"new"}})
	perform(t, r, repo, domain.GitOperation{Operation: domain.Unstage, Paths: []string{"new"}})
	// Stage a tracked deletion without staging unrelated untracked content.
	if err = os.Remove(filepath.Join(dir, "keep")); err != nil {
		t.Fatal(err)
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Stage, Paths: []string{"keep"}})
	staged := string(testgit.Run(t, dir, "diff", "--cached", "--name-only"))
	if staged != "keep\n" {
		t.Fatal("staged unrelated paths", staged)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.History(ctx, repo, "", 0, 100)
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
		t.Fatal(err)
	}
}
func stringTrim(data []byte) string { return string(data[:len(data)-1]) }

func TestDiscardRestoresIndexAndRejectsUnsafePaths(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	name := "-literal[1]\nfile"
	testgit.Write(t, filepath.Join(dir, name), "base\n")
	testgit.Write(t, filepath.Join(dir, "deleted"), "base\n")
	testgit.Run(t, dir, "add", "--", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, filepath.Join(dir, name), "staged\n")
	testgit.Run(t, dir, "--literal-pathspecs", "add", "--", name)
	testgit.Write(t, filepath.Join(dir, name), "unstaged\n")
	if err := os.Remove(filepath.Join(dir, "deleted")); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "untracked"), "keep\n")
	for _, paths := range [][]string{{"untracked"}, {name, "untracked"}, {"../escape"}} {
		_, err := r.Write(context.Background(), repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Discard, Paths: paths})
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
			t.Fatal(paths, err)
		}
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Discard, Paths: []string{name, "deleted"}})
	for path, want := range map[string]string{name: "staged\n", "deleted": "base\n", "untracked": "keep\n"} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Fatalf("%q: %q %v", path, got, err)
		}
	}
	if got := string(testgit.Run(t, dir, "show", ":"+name)); got != "staged\n" {
		t.Fatal(got)
	}
}

func TestAmendHeadStagedChangesAndFailureVerification(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	op := domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Amend, Message: "new $(touch injected)\n\nbody"}
	if _, err := r.Write(context.Background(), repo, op); err == nil {
		t.Fatal("amend without HEAD accepted")
	}
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", "--", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	old := string(testgit.Run(t, dir, "rev-parse", "HEAD"))
	perform(t, r, repo, op)
	if got := string(testgit.Run(t, dir, "log", "-1", "--format=%B")); got != op.Message+"\n\n" {
		t.Fatalf("message %q", got)
	}
	if got := string(testgit.Run(t, dir, "rev-parse", "HEAD")); got == old {
		t.Fatal("HEAD unchanged")
	}
	testgit.Write(t, filepath.Join(dir, "file"), "staged\n")
	testgit.Run(t, dir, "add", "--", "file")
	hook := filepath.Join(repo.GitDir, "hooks", "pre-commit")
	testgit.Write(t, hook, "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := r.Write(context.Background(), repo, op)
	if err == nil || !result.Verified {
		t.Fatal(result, err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	perform(t, r, repo, op)
	if got := string(testgit.Run(t, dir, "show", "HEAD:file")); got != "staged\n" {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatal("message executed", err)
	}
}

func TestDiscardAndAmendRejectConflicts(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", "--", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Run(t, dir, "switch", "-c", "other")
	testgit.Write(t, filepath.Join(dir, "file"), "other\n")
	testgit.Run(t, dir, "commit", "-am", "other")
	testgit.Run(t, dir, "switch", "main")
	testgit.Write(t, filepath.Join(dir, "file"), "main\n")
	testgit.Run(t, dir, "commit", "-am", "main")
	// merge is expected to fail with a conflict.
	cmd := exec.Command("git", "-C", dir, "merge", "other")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected merge conflict")
	}
	for _, op := range []domain.GitOperation{{Operation: domain.Discard, Paths: []string{"file"}}, {Operation: domain.Amend, Message: "amend"}} {
		op.RepositoryID = repo.Repository.RepositoryID
		_, err := r.Write(context.Background(), repo, op)
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
			t.Fatal(op, err)
		}
	}
}

func TestDiscardRejectsIgnoredContentAtDeletedPath(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	testgit.Write(t, filepath.Join(dir, "file"), "tracked")
	testgit.Run(t, dir, "add", "--", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	if err := os.Remove(filepath.Join(dir, "file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "file"), 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, ".gitignore"), "file/\n")
	testgit.Write(t, filepath.Join(dir, "file", "ignored"), "keep")
	_, err := r.Write(context.Background(), repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Discard, Paths: []string{"file"}})
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "file", "ignored")); err != nil || string(got) != "keep" {
		t.Fatal(string(got), err)
	}
}
