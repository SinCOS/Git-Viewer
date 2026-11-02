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

// runReadBatch prefers one framed helper batch. Each command retains its own
// output and exit status; ordinary SSH channels provide the fallback.
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
	if e.Helpers != nil {
		client, err := e.Helpers.Acquire(ctx, host)
		if err == nil {
			results, batchErr := client.Batch(ctx, dir, limit, 15*time.Second, commands)
			if batchErr == nil {
				return results, nil
			}
			if !helperReadFallback(batchErr) {
				return nil, batchErr
			}
		} else if !helperAcquireFallback(err) {
			return nil, err
		}
	}
	return e.runReadBatchCLI(ctx, host, dir, limit, commands)
}

// runReadBatchCLI preserves the original per-channel fallback scheduling.
func (e Executor) runReadBatchCLI(ctx context.Context, host, dir string, limit int, commands [][]string) ([]domain.GitResult, error) {
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
				result, err := e.runGitCLI(ctx, host, dir, limit, 2*time.Minute, nil, commands[i]...)
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
