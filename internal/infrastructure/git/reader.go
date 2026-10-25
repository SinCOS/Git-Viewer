package git

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/process"
)

// LocalReader exposes only fixed local read operations. No shell or arbitrary argv API.
type LocalReader struct {
	GitPath     string
	OutputLimit int
	// Transport is internal execution state for a verified remote repository.
	// Callers can only reach fixed operations exposed by this reader.
	Transport func(context.Context, string, int, []byte, ...string) (domain.GitResult, error)
	Input     []byte
	Timeout   time.Duration
	WriteMode bool
}

func (r LocalReader) run(ctx context.Context, dir string, tail ...string) (domain.GitResult, error) {
	if r.Transport != nil {
		timeout := r.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		bounded, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return r.Transport(bounded, dir, r.OutputLimit, r.Input, tail...)
	}
	if !filepath.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return domain.GitResult{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	executable := r.GitPath
	if executable == "" {
		executable = "git"
	}
	args := []string{"--no-pager", "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "color.ui=false", "-c", "core.quotePath=true", "-C", dir}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	env := CleanEnvironment()
	if r.WriteMode {
		env = WriteEnvironment()
	}
	return process.RunInput(ctx, timeout, r.OutputLimit, executable, append(args, tail...), env, r.Input)
}

func (r LocalReader) Open(ctx context.Context, path string) (domain.RepositoryContext, error) {
	invalid := func() (domain.RepositoryContext, error) {
		return domain.RepositoryContext{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	if path == "" || strings.ContainsRune(path, 0) {
		return invalid()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return invalid()
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return invalid()
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return invalid()
	}
	version, err := r.run(ctx, abs, "--version")
	if err != nil {
		return domain.RepositoryContext{}, err
	}
	if err = CheckVersion(string(version.Stdout)); err != nil {
		return domain.RepositoryContext{}, err
	}
	inside, err := r.run(ctx, abs, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return domain.RepositoryContext{}, repositoryError(err)
	}
	if string(inside.Stdout) != "true\n" {
		return invalid()
	}
	// Separate invocations preserve embedded newlines in directory names.
	values := make([]string, 3)
	for i, flag := range []string{"--show-toplevel", "--absolute-git-dir", "--git-common-dir"} {
		result, err := r.run(ctx, abs, "rev-parse", flag)
		if err != nil {
			return domain.RepositoryContext{}, err
		}
		value := strings.TrimSuffix(string(result.Stdout), "\n")
		if !filepath.IsAbs(value) {
			value = filepath.Join(abs, value)
		}
		values[i], err = filepath.EvalSymlinks(value)
		if err != nil {
			return invalid()
		}
	}
	id := fmt.Sprintf("local-%x", sha256.Sum256([]byte(values[0]+"\x00"+values[1])))
	return domain.RepositoryContext{Repository: domain.Repository{RepositoryID: id, Location: "local", Path: values[0], DisplayName: filepath.Base(values[0])}, GitDir: values[1], CommonDir: values[2]}, nil
}

func repositoryError(err error) error {
	if ge, ok := err.(*domain.GitError); ok && ge.Code == domain.CommandFailed {
		return &domain.GitError{Code: domain.InvalidRepository}
	}
	return err
}

func (r LocalReader) Status(ctx context.Context, repo domain.RepositoryContext) (domain.GitStatus, error) {
	if (repo.Repository.Location != "local" && r.Transport == nil) || repo.Repository.RepositoryID == "" {
		return domain.GitStatus{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	result, err := r.run(ctx, repo.Repository.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return domain.GitStatus{}, err
	}
	changes, err := ParseStatus(result.Stdout)
	return domain.GitStatus{RepositoryID: repo.Repository.RepositoryID, FetchedAt: time.Now().UTC().Format(time.RFC3339Nano), Changes: changes}, err
}
