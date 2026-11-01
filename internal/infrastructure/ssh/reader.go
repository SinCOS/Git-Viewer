package ssh

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Reader binds fixed Git read operations to one SSH host; paths remain POSIX paths.
func (e Executor) Reader(host string) gitexec.LocalReader {
	return gitexec.LocalReader{OutputLimit: e.OutputLimit, WorktreeOpen: func(ctx context.Context, dir string) (domain.RepositoryContext, error) {
		return e.Open(ctx, host, dir)
	}, WorktreeCheckNewPath: func(ctx context.Context, target string) error {
		parent := path.Dir(target)
		result, err := e.runRemote(ctx, host, 15*time.Second, 64*1024, []string{"realpath", "--", parent})
		if err != nil {
			return err
		}
		if strings.TrimSuffix(string(result.Stdout), "\n") != parent {
			return &domain.GitError{Code: domain.UnsafeOperation, Reason: "Worktree parent must exist and be canonical"}
		}
		for _, flag := range []string{"-e", "-L"} {
			result, err = e.runRemote(ctx, host, 15*time.Second, 64*1024, []string{"test", flag, target})
			if err == nil {
				return &domain.GitError{Code: domain.UnsafeOperation, Reason: "Worktree path must not exist"}
			}
			var gitErr *domain.GitError
			if !errors.As(err, &gitErr) || gitErr.Code != domain.CommandFailed || result.ExitCode != 1 {
				return err
			}
		}
		return nil
	}, Transport: func(ctx context.Context, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
		return e.runGit(ctx, host, dir, limit, input, args...)
	}, Batch: func(ctx context.Context, dir string, limit int, commands [][]string) ([]domain.GitResult, error) {
		return e.runReadBatch(ctx, host, dir, limit, commands)
	}}
}

// runGit is the single product Git transport for Changes, Staged, History
// and grouped reads. Only connection isolation and Git config policy differ
// for controlled writes; framing, limits and connection scheduling are shared.
func (e Executor) runGit(ctx context.Context, host, dir string, limit int, input []byte, args ...string) (domain.GitResult, error) {
	if !path.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return domain.GitResult{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	write := gitexec.IsWriteArgs(args)
	argv := []string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat"}
	if !write {
		argv = append(argv, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	}
	argv = append(argv, "git", "--no-pager", "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "log.showSignature=false", "-c", "color.ui=false", "-C", dir)
	return e.runRemoteIsolated(ctx, host, 2*time.Minute, limit, append(argv, args...), input, write && !indexOnlyArgs(args))
}

// indexOnlyArgs is deliberately narrow. Commit/hooks, worktree changes and
// network writes retain isolated connections. Cancellation of a multiplexed
// index operation still returns an unknown outcome through the ordinary writer.
func indexOnlyArgs(args []string) bool {
	return len(args) >= 2 && args[0] == "add" && args[1] == "--" ||
		len(args) >= 3 && args[0] == "restore" && args[1] == "--staged" && args[2] == "--" ||
		len(args) >= 3 && args[0] == "rm" && args[1] == "--cached" && args[2] == "--"
}

func (e Executor) Open(ctx context.Context, host, dir string) (domain.RepositoryContext, error) {
	if !hostPattern.MatchString(host) || !path.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return domain.RepositoryContext{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	argv := []string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1"}
	argv = append(argv, "GIT_CONFIG_GLOBAL=/dev/null", "git", "--no-pager", "-C", dir)
	probeOne := func(args ...string) (string, error) {
		result, err := e.runRemote(ctx, host, 15*time.Second, 64*1024, append(append([]string(nil), argv...), args...))
		return strings.TrimSuffix(string(result.Stdout), "\n"), err
	}
	version, err := probeOne("--version")
	if err != nil {
		return domain.RepositoryContext{}, err
	}
	if err = gitexec.CheckVersion(version); err != nil {
		return domain.RepositoryContext{}, err
	}
	inside, err := probeOne("rev-parse", "--is-inside-work-tree")
	if err != nil {
		if ge, ok := err.(*domain.GitError); ok && ge.Code == domain.CommandFailed {
			return domain.RepositoryContext{}, &domain.GitError{Code: domain.InvalidRepository}
		}
		return domain.RepositoryContext{}, err
	}
	if inside != "true" {
		return domain.RepositoryContext{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	// Resolve on the remote host before deriving metadata paths, so aliases
	// such as /var and /private/var cannot change repository identity.
	canonical, err := probeOne("rev-parse", "--show-toplevel")
	if err != nil {
		return domain.RepositoryContext{}, err
	}
	dir = canonical
	if !path.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return domain.RepositoryContext{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	results, err := e.runReadBatch(ctx, host, dir, 64*1024, [][]string{
		{"rev-parse", "--absolute-git-dir"},
		{"rev-parse", "--git-common-dir"},
	})
	if err != nil {
		return domain.RepositoryContext{}, err
	}
	values := []string{dir, "", ""}
	for i, result := range results {
		v := strings.TrimSuffix(string(result.Stdout), "\n")
		if !path.IsAbs(v) {
			v = path.Join(dir, v)
		}
		// Paths belong to the remote filesystem. Never inspect local files
		// or resolve local symlinks to establish a remote repository identity.
		values[i+1] = path.Clean(v)
	}
	id := fmt.Sprintf("ssh-%x", sha256.Sum256([]byte(host+"\x00"+values[0]+"\x00"+values[1])))
	return domain.RepositoryContext{Repository: domain.Repository{RepositoryID: id, Location: "ssh", Path: values[0], DisplayName: host + ":" + path.Base(values[0])}, SSHHost: host, GitDir: values[1], CommonDir: values[2]}, nil
}

// Hosts lists literal aliases for UI suggestions. OpenSSH resolves Includes,
// Match blocks, identities and all connection options itself at connection time.
func Hosts(config string) ([]string, error) {
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		config = filepath.Join(home, ".ssh", "config")
	}
	data, err := os.ReadFile(config)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "Host") {
			continue
		}
		for _, host := range fields[1:] {
			if strings.HasPrefix(host, "#") {
				break
			}
			if hostPattern.MatchString(host) && !seen[host] {
				seen[host] = true
				result = append(result, host)
			}
		}
	}
	return result, nil
}
