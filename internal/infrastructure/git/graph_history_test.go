package git_test

import (
	"context"
	"fmt"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphRealOctopusRootsDetachedAndWorktreeLock(t *testing.T) {
	r, repo := configured(t)
	dir := repo.Repository.Path
	testgit.Write(t, filepath.Join(dir, "base"), "base")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "root")
	for _, branch := range []string{"left", "right", "main"} {
		if branch != "main" {
			testgit.Run(t, dir, "checkout", "-b", branch, "main")
		} else {
			testgit.Run(t, dir, "checkout", "main")
		}
		testgit.Write(t, filepath.Join(dir, branch), branch)
		testgit.Run(t, dir, "add", branch)
		testgit.Run(t, dir, "commit", "-m", branch)
	}
	testgit.Run(t, dir, "merge", "--no-ff", "left", "right", "-m", "octopus")
	testgit.Run(t, dir, "checkout", "--orphan", "isolated")
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "isolated root")
	testgit.Run(t, dir, "checkout", "--detach", "main")
	testgit.Run(t, dir, "tag", "-a", "root-tag", "isolated", "-m", "tag-only root")
	testgit.Run(t, dir, "branch", "-D", "isolated")
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "detached")
	var layout domain.GraphLayout
	var commits []domain.CommitRecord
	anchor := ""
	offset := 0
	for {
		page, err := r.History(context.Background(), repo, anchor, offset, 2)
		if err != nil {
			t.Fatal(err)
		}
		layout.Append(page.Commits)
		commits = append(commits, page.Commits...)
		anchor, offset = page.Next, page.Offset
		if !page.More {
			break
		}
	}
	octopus, roots, detached := false, 0, false
	for _, c := range commits {
		if len(c.Parents) == 3 {
			octopus = true
		}
		if len(c.Parents) == 0 {
			roots++
		}
		if c.Subject == "detached" {
			detached = true
		}
	}
	if !octopus || roots != 2 || !detached || len(layout.Lanes) != 0 {
		t.Fatal(commits, layout.Lanes)
	}
	wt := filepath.Join(t.TempDir(), "中文 worktree\npath")
	testgit.Run(t, dir, "worktree", "add", "--detach", wt, "main")
	testgit.Run(t, dir, "worktree", "lock", "--reason", "locked\nreason", wt)
	trees, err := r.WorktreeList(context.Background(), repo)
	if err != nil || len(trees) != 2 {
		t.Fatal(trees, err)
	}
	found := false
	for _, tree := range trees {
		if tree.Locked && tree.Detached && tree.LockReason == "locked\nreason" {
			found = true
		}
	}
	if !found {
		t.Fatal(trees)
	}
}
func TestHistory10000CommitsPagination(t *testing.T) {
	dir := testgit.Repo(t)
	var input strings.Builder
	for i := 0; i < 10000; i++ {
		subject := fmt.Sprintf("commit %05d", i)
		fmt.Fprintf(&input, "commit refs/heads/main\ncommitter Test <test@example.invalid> %d +0000\ndata %d\n%s\n\n", 1700000000+i, len(subject), subject)
	}
	cmd := exec.Command("git", "-C", dir, "fast-import", "--quiet")
	cmd.Env = gitexec.CleanEnvironment()
	cmd.Stdin = strings.NewReader(input.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import fixture: %v %s", err, out)
	}
	r := gitexec.LocalReader{}
	repo, err := r.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.History(context.Background(), repo, "", 0, 100)
	if err != nil || len(first.Commits) != 100 || !first.More {
		t.Fatal(len(first.Commits), err)
	}
	last, err := r.History(context.Background(), repo, first.Next, 9900, 100)
	if err != nil || len(last.Commits) != 100 || last.More || last.Commits[99].Subject != "commit 00000" {
		t.Fatal(len(last.Commits), err)
	}
}
