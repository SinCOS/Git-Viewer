package git_test

import (
	"context"
	"strings"
	"testing"

	"gitviewer/internal/testgit"
)

func TestBranchTrackingSymbolicAndHeadHistory(t *testing.T) {
	r, repo := configured(t)
	ctx := context.Background()
	dir := repo.Repository.Path
	page, err := r.HeadHistory(ctx, repo, 10)
	if err != nil || len(page.Commits) != 0 {
		t.Fatal("unborn HEAD", page, err)
	}
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "base")
	base := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Run(t, dir, "remote", "add", "origin", "https://example.invalid/repo.git")
	testgit.Run(t, dir, "update-ref", "refs/remotes/origin/main", base)
	testgit.Run(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	testgit.Run(t, dir, "branch", "--set-upstream-to=origin/main", "main")
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "ahead")
	refs, err := r.BranchList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	current, symbolic := false, false
	for _, ref := range refs {
		if ref.Current {
			current = true
			if ref.Tracking != "[ahead 1]" || ref.Upstream != "refs/remotes/origin/main" {
				t.Fatal(ref)
			}
		}
		if ref.Name == "refs/remotes/origin/HEAD" {
			symbolic = true
			if ref.Symbolic != "refs/remotes/origin/main" {
				t.Fatal(ref)
			}
		}
	}
	if !current || !symbolic {
		t.Fatal(refs)
	}
	testgit.Run(t, dir, "checkout", "--detach", base)
	page, err = r.HeadHistory(ctx, repo, 10)
	if err != nil || len(page.Commits) != 1 || page.Commits[0].OID != base {
		t.Fatal("detached HEAD", page, err)
	}
	testgit.Run(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
	refs, err = r.BranchList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if ref.Name == "refs/heads/main" && ref.Tracking != "[gone]" {
			t.Fatal(ref)
		}
	}
}
