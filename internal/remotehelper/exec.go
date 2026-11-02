package remotehelper

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
	"gitviewer/internal/infrastructure/process"
)

const (
	maxReadConcurrency = 8
	defaultReadTimeout = 15 * time.Second
	defaultWriteTime   = 2 * time.Minute
)

// Engine executes one validated fixed-argv Git command. Writes are serialized
// per helper process; reads run concurrently up to maxReadConcurrency.
type Engine struct {
	GitPath    string
	readSlots  chan struct{}
	writeSlots chan struct{}
}

// NewEngine creates an execution engine. An empty GitPath uses "git" from the
// remote PATH, matching the system-CLI policy.
func NewEngine(gitPath string) *Engine {
	return &Engine{
		GitPath:    gitPath,
		readSlots:  make(chan struct{}, maxReadConcurrency),
		writeSlots: make(chan struct{}, 1),
	}
}

// Exec validates and runs one command. It always returns a result; validation
// and process failures are reported in Result.Error.
func (e *Engine) Exec(ctx context.Context, dir string, limit int, timeout time.Duration, args []string, input []byte) CommandResult {
	result := CommandResult{Exit: -1}
	if !validDir(dir) {
		result.Error = unsafeError("Invalid repository directory")
		return result
	}
	bounded, failure := normalizeLimit(limit)
	if failure != nil {
		result.Error = failure
		return result
	}
	if len(input) > MaxInput {
		result.Error = unsafeError("Input exceeds the protocol limit")
		return result
	}
	if err := Validate(args); err != nil {
		result.Error = unsafeError("Command is not in the fixed allowlist")
		return result
	}
	write := gitargs.IsWriteArgs(args)
	if timeout <= 0 {
		if write {
			timeout = defaultWriteTime
		} else {
			timeout = defaultReadTimeout
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	slots := e.readSlots
	if write {
		slots = e.writeSlots
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		result.Error = failureFromContext(ctx)
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Error = failureFromContext(ctx)
		return result
	}
	executable := e.GitPath
	if executable == "" {
		executable = "git"
	}
	argv := []string{executable}
	argv = append(argv, gitargs.BasePrefix()...)
	argv = append(argv, "-C", dir)
	argv = append(argv, args...)
	env := gitargs.CleanEnvironment()
	if write {
		env = gitargs.WriteEnvironment()
	}
	gitResult, err := process.RunInputGroup(ctx, timeout, bounded, executable, argv[1:], env, input)
	result.Exit = gitResult.ExitCode
	result.Stdout = gitResult.Stdout
	result.Stderr = gitResult.Stderr
	if err != nil {
		result.Error = remoteError(err)
	}
	return result
}

func validDir(dir string) bool {
	return dir != "" && len(dir) <= MaxDir && path.IsAbs(dir) && path.Clean(dir) == dir && !strings.ContainsRune(dir, 0)
}

func normalizeLimit(limit int) (int, *RemoteError) {
	if limit < 0 {
		return 0, unsafeError("Output limit must not be negative")
	}
	if limit == 0 {
		return DefaultLimit, nil
	}
	if limit > MaxLimit {
		return 0, unsafeError("Output limit exceeds the protocol maximum")
	}
	return limit, nil
}

func unsafeError(message string) *RemoteError {
	return &RemoteError{Code: string(domain.UnsafeOperation), Message: message}
}

func failureFromContext(ctx context.Context) *RemoteError {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &RemoteError{Code: string(domain.OperationTimedOut)}
	}
	return &RemoteError{Code: string(domain.OperationCancelled)}
}

// remoteError converts a domain error to the protocol's stable error codes.
// Raw process output never becomes a message.
func remoteError(err error) *RemoteError {
	var ge *domain.GitError
	if errors.As(err, &ge) {
		return &RemoteError{Code: string(ge.Code), Message: ge.Reason}
	}
	return &RemoteError{Code: string(domain.CommandFailed)}
}
