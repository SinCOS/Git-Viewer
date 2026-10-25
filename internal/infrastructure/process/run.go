package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"gitviewer/internal/domain"
)

const DefaultLimit = 2 * 1024 * 1024

type budget struct {
	sync.Mutex
	remaining int
	exceeded  bool
	cancel    context.CancelFunc
}
type output struct {
	data   []byte
	budget *budget
}

func (w *output) Write(p []byte) (int, error) {
	w.budget.Lock()
	defer w.budget.Unlock()
	n := len(p)
	keep := min(n, w.budget.remaining)
	w.data = append(w.data, p[:keep]...)
	w.budget.remaining -= keep
	if keep < n {
		w.budget.exceeded = true
		w.budget.cancel()
	}
	return n, nil
}

// Run bounds combined output and process lifetime. It never logs argv or output.
func Run(ctx context.Context, timeout time.Duration, limit int, executable string, args []string, env []string) (domain.GitResult, error) {
	return RunInput(ctx, timeout, limit, executable, args, env, nil)
}

func RunInput(ctx context.Context, timeout time.Duration, limit int, executable string, args []string, env []string, input []byte) (domain.GitResult, error) {
	result := domain.GitResult{ExitCode: -1}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	timed, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()
	run, cancel := context.WithCancel(timed)
	defer cancel()
	b := &budget{remaining: limit, cancel: cancel}
	stdout, stderr := &output{budget: b}, &output{budget: b}
	cmd := exec.CommandContext(run, executable, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.WaitDelay = 250 * time.Millisecond
	err := cmd.Run()
	result.Stdout = stdout.data
	result.Stderr = stderr.data
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	code := domain.CommandFailed
	switch {
	case b.exceeded:
		code = domain.OutputLimitExceeded
	case errors.Is(timed.Err(), context.DeadlineExceeded):
		code = domain.OperationTimedOut
	case errors.Is(timed.Err(), context.Canceled):
		code = domain.OperationCancelled
	case errors.Is(err, exec.ErrNotFound), os.IsNotExist(err):
		code = domain.GitUnavailable
	case err == nil:
		return result, nil
	}
	return result, &domain.GitError{Code: code, Cause: err}
}
