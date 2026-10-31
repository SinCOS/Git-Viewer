package git_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestCreateBranchCherryPickAndRevert(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, filepath.Join(dir, "file"), "first\n")
	testgit.Run(t, dir, "commit", "-am", "first")
	first := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))

	perform(t, r, repo, domain.GitOperation{Operation: domain.CreateBranch, Ref: "feature/copy", Commit: first})
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "branch", "--show-current"))); got != "feature/copy" {
		t.Fatalf("current branch = %q", got)
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.SwitchBranch, Ref: "main"})
	testgit.Write(t, filepath.Join(dir, "other"), "second\n")
	testgit.Run(t, dir, "add", "other")
	testgit.Run(t, dir, "commit", "-m", "second")
	second := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	perform(t, r, repo, domain.GitOperation{Operation: domain.SwitchBranch, Ref: "feature/copy"})

	perform(t, r, repo, domain.GitOperation{Operation: domain.CherryPick, Commit: second})
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "log", "-1", "--format=%s"))); got != "second" {
		t.Fatalf("cherry-pick subject = %q", got)
	}
	perform(t, r, repo, domain.GitOperation{Operation: domain.Revert, Commit: strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))})
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "log", "-1", "--format=%s"))); !strings.HasPrefix(got, "Revert ") {
		t.Fatalf("revert subject = %q", got)
	}

	testgit.Write(t, filepath.Join(dir, "dirty"), "dirty\n")
	if _, err := r.Write(context.Background(), repo, domain.GitOperation{
		RepositoryID: repo.Repository.RepositoryID,
		Operation:    domain.CreateBranch,
		Ref:          "feature/dirty",
		Commit:       first,
	}); err == nil {
		t.Fatal("dirty branch creation was accepted")
	}
}
