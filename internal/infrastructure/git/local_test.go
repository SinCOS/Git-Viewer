package git_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalGit(t *testing.T) {
	dir := testgit.Repo(t)
	name := "中文 space\n-leading'file"
	testgit.Write(t, filepath.Join(dir, name), "hello")
	executor := gitexec.LocalExecutor{}
	result, err := executor.Execute(context.Background(), testgit.Command(dir, domain.Status))
	if err != nil || result.ExitCode != 0 || !strings.Contains(string(result.Stdout), name+"\x00") {
		t.Fatalf("%+v %v", result, err)
	}
	// Inherited GIT_DIR must not redirect the command to another repository.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := executor.Execute(context.Background(), testgit.Command(dir, domain.Status)); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Execute(context.Background(), testgit.Command(t.TempDir(), domain.Status))
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.CommandFailed || result.ExitCode == 0 || len(result.Stderr) == 0 {
		t.Fatalf("%+v %v", result, err)
	}
	_, err = (gitexec.LocalExecutor{GitPath: filepath.Join(t.TempDir(), "missing")}).Execute(context.Background(), testgit.Command(dir, domain.Status))
	if !errors.As(err, &ge) || ge.Code != domain.GitUnavailable {
		t.Fatalf("%v", err)
	}
	result, err = (gitexec.LocalExecutor{OutputLimit: 8}).Execute(context.Background(), testgit.Command(dir, domain.Status))
	if !errors.As(err, &ge) || ge.Code != domain.OutputLimitExceeded || len(result.Stdout)+len(result.Stderr) > 8 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestRejectUnsafeCommands(t *testing.T) {
	for _, op := range []domain.Operation{domain.Stage, domain.Commit, domain.Push, "shell"} {
		_, err := (gitexec.LocalExecutor{}).Execute(context.Background(), testgit.Command(t.TempDir(), op))
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
			t.Fatal(op, err)
		}
	}
	for _, mutate := range []func(*domain.GitCommand){
		func(c *domain.GitCommand) { c.Operation.RepositoryID = "other" },
		func(c *domain.GitCommand) { c.Operation.Ref = "--exec=bad" },
		func(c *domain.GitCommand) { c.Operation.Paths = []string{"x"} },
		func(c *domain.GitCommand) { c.Operation.Limit = 1001 },
		func(c *domain.GitCommand) { c.Repository.Repository.Path = "/tmp/\x00bad" },
	} {
		c := testgit.Command(t.TempDir(), domain.Status)
		mutate(&c)
		if _, err := gitexec.ReadArgs(c); err == nil {
			t.Fatal("unsafe input accepted")
		}
	}
}
