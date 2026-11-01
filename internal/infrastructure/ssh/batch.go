package ssh

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

// runReadBatch groups fixed reads locally. Each command retains ordinary SSH
// stdout/stderr, limits and exit status; no remote shell batch protocol exists.
func (e Executor) runReadBatch(ctx context.Context, host, dir string, limit int, commands [][]string) ([]domain.GitResult, error) {
	if !ValidHost(host) || !path.IsAbs(dir) || strings.ContainsRune(dir, 0) || len(commands) == 0 {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	for _, command := range commands {
		if len(command) == 0 || gitexec.IsWriteArgs(command) {
			return nil, &domain.GitError{Code: domain.UnsafeOperation}
		}
		for _, arg := range command {
			if strings.ContainsRune(arg, 0) {
				return nil, &domain.GitError{Code: domain.UnsafeOperation}
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]domain.GitResult, len(commands))
	var firstErr error
	var once sync.Once
	var workers sync.WaitGroup
	jobs := make(chan int)
	for range min(4, len(commands)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				argv := []string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "git", "--no-pager", "-C", dir, "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "log.showSignature=false", "-c", "color.ui=false"}
				result, err := e.runReadRemote(ctx, host, 2*time.Minute, limit, append(argv, commands[i]...))
				results[i] = result
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
				}
			}
		})
	}
	for i := range commands {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	return results, nil
}
