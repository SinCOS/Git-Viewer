package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionFloor(t *testing.T) {
	for _, v := range []string{"git version 2.38.0", "git version 2.39.5 (Apple Git-154)", "git version 3.0.0"} {
		if err := gitexec.CheckVersion(v); err != nil {
			t.Fatal(v, err)
		}
	}
	for _, v := range []string{"git version 1.9", "git version 2.35.0", "unexpected", "git version 2.x"} {
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
