//go:build unix

package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"gitviewer/internal/domain"
)

// RunInputGroup is RunInput with the child in its own process group. On
// cancellation the whole group receives SIGTERM, then SIGKILL after a short
// grace period, so Git hooks and their descendants cannot outlive a canceled
// commit or fetch.
func RunInputGroup(ctx context.Context, timeout time.Duration, limit int, executable string, args []string, env []string, input []byte) (domain.GitResult, error) {
	result := domain.GitResult{ExitCode: -1}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if err := ctx.Err(); err != nil {
		return result, contextFailure(err)
	}
	timed, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()
	run, cancel := context.WithCancel(timed)
	defer cancel()
	b := &budget{remaining: limit, cancel: cancel}
	stdout, stderr := &output{budget: b}, &output{budget: b}
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound), os.IsNotExist(err):
			return result, &domain.GitError{Code: domain.GitUnavailable, Cause: err}
		default:
			return result, &domain.GitError{Code: domain.CommandFailed, Cause: err}
		}
	}
	pgid := cmd.Process.Pid
	exited := make(chan struct{})
	killerDone := make(chan struct{})
	go func() {
		defer close(killerDone)
		select {
		case <-run.Done():
		case <-exited:
			// Cancellation and parent exit can race. A dead parent does not
			// imply that its descendants exited or closed inherited pipes.
			if run.Err() == nil {
				return
			}
		}
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		// Never stop escalation merely because Wait reaped the parent.
		// Poll group existence to avoid delaying ordinary TERM cancellation.
		deadline := time.NewTimer(250 * time.Millisecond)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			if syscall.Kill(-pgid, 0) == syscall.ESRCH {
				return
			}
			select {
			case <-deadline.C:
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
				return
			case <-tick.C:
			}
		}
	}()
	err := cmd.Wait()
	close(exited)
	<-killerDone
	result.Stdout = stdout.data
	result.Stderr = stderr.data
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	switch {
	case b.exceeded:
		return result, &domain.GitError{Code: domain.OutputLimitExceeded, Cause: err}
	case errors.Is(timed.Err(), context.DeadlineExceeded):
		return result, &domain.GitError{Code: domain.OperationTimedOut, Cause: err}
	case errors.Is(timed.Err(), context.Canceled):
		return result, &domain.GitError{Code: domain.OperationCancelled, Cause: err}
	case err == nil:
		return result, nil
	}
	return result, &domain.GitError{Code: domain.CommandFailed, Cause: err}
}

func contextFailure(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &domain.GitError{Code: domain.OperationTimedOut, Cause: err}
	}
	return &domain.GitError{Code: domain.OperationCancelled, Cause: err}
}
