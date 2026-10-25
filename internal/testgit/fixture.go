package testgit

import (
	"os"
	"os/exec"
	"testing"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
)

func Run(t testing.TB, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(git.CleanEnvironment(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %v: %v: %s", args, err, out)
	}
	return out
}
func Repo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	Run(t, dir, "init", "-b", "main")
	return dir
}
func Write(t testing.TB, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func Command(dir string, op domain.Operation) domain.GitCommand {
	return domain.GitCommand{Repository: domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "fixture", Location: "local", Path: dir, DisplayName: "Fixture"}}, Operation: domain.GitOperation{RepositoryID: "fixture", Operation: op}}
}
