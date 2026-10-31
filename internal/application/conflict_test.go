package application

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
)

func TestConflictFileAndResolveConflict(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file.txt"), "base\n")
	testgit.Run(t, dir, "add", "file.txt")
	testgit.Run(t, dir, "commit", "-m", "base")
	base := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))

	testgit.Run(t, dir, "switch", "-c", "feature")
	testgit.Write(t, filepath.Join(dir, "file.txt"), "theirs\n")
	testgit.Run(t, dir, "commit", "-am", "feature")
	testgit.Run(t, dir, "switch", "main")
	testgit.Write(t, filepath.Join(dir, "file.txt"), "ours\n")
	testgit.Run(t, dir, "commit", "-am", "main")
	merge := exec.Command("git", "-C", dir, "merge", "feature")
	merge.Env = append(git.CleanEnvironment(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	if err := merge.Run(); err == nil {
		t.Fatal("conflict fixture merged cleanly")
	}

	reader := git.LocalReader{}
	repo, err := reader.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	service := LocalService{Reader: reader}
	conflict, err := service.ConflictFile(context.Background(), repo, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if conflict.Base != "base\n" || conflict.Ours != "ours\n" || conflict.Theirs != "theirs\n" {
		t.Fatalf("stages = base:%q ours:%q theirs:%q", conflict.Base, conflict.Ours, conflict.Theirs)
	}
	if !conflict.ResultAvailable || !strings.Contains(conflict.Result, "<<<<<<<") {
		t.Fatalf("worktree result = %q available=%t", conflict.Result, conflict.ResultAvailable)
	}

	result, err := service.ResolveConflict(context.Background(), repo, "file.txt", "resolved\n")
	if err != nil || !result.Verified {
		t.Fatalf("resolve = %+v %v", result, err)
	}
	if got := string(testgit.Run(t, dir, "show", ":file.txt")); got != "resolved\n" {
		t.Fatalf("index content = %q", got)
	}
	for _, change := range result.Status.Changes {
		if change.Path == "file.txt" && change.Conflicted {
			t.Fatal("file remained conflicted")
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, "file.txt")); err != nil || string(got) != "resolved\n" {
		t.Fatalf("worktree content = %q %v", got, err)
	}
	if conflict.Base != "base\n" || base == "" {
		t.Fatal("fixture base commit missing")
	}
}

func TestResolveConflictRejectsCleanPath(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file.txt"), "clean\n")
	testgit.Run(t, dir, "add", "file.txt")
	testgit.Run(t, dir, "commit", "-m", "base")
	reader := git.LocalReader{}
	repo, err := reader.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	service := LocalService{Reader: reader}
	if _, err := service.ResolveConflict(context.Background(), repo, "file.txt", "changed\n"); err == nil {
		t.Fatal("clean path was accepted as a conflict")
	}
}
