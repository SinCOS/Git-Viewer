package ssh

import (
	"context"
	"errors"
	"os/exec"

	"gitviewer/internal/domain"
)

// SessionMode makes connection ownership explicit for every SSH consumer.
// Streams share authentication, not their stdin/stdout or subsystem lifecycle.
type SessionMode uint8

const (
	SharedSession SessionMode = iota
	IsolatedSession
	TerminalSession
)

func (e Executor) SessionArgs(ctx context.Context, host string, mode SessionMode) ([]string, error) {
	if !ValidHost(host) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	args := e.ConnectionArgs()
	switch mode {
	case IsolatedSession:
		return append(args, "-S", "none", "-o", "ControlMaster=no"), nil
	case SharedSession, TerminalSession:
		c, err := e.connections().get(ctx, e, host)
		if err != nil {
			return nil, err
		}
		if mode == TerminalSession {
			args[0] = "-tt"
		}
		return append(args, "-o", "ControlMaster=no", "-S", c.path), nil
	default:
		return nil, &domain.GitError{Code: domain.UnsafeOperation}
	}
}

// Command prepares an exec/subsystem channel through the same connection
// policy as Git and SFTP. Callers own streaming, output bounds and Wait.
func (e Executor) Command(ctx context.Context, host string, mode SessionMode, argv []string, subsystem string) (*exec.Cmd, error) {
	if subsystem != "" && (subsystem != "sftp" || len(argv) != 0) {
		return nil, &domain.GitError{Code: domain.UnsafeOperation}
	}
	remote, err := Encode(argv)
	if err != nil {
		return nil, err
	}
	args, err := e.SessionArgs(ctx, host, mode)
	if err != nil {
		return nil, err
	}
	if subsystem != "" {
		args = append(args, "-s", host, subsystem)
	} else {
		args = append(args, host)
		if len(argv) > 0 {
			args = append(args, remote)
		}
	}
	cmd := exec.CommandContext(ctx, e.executable(), args...)
	cmd.Env = e.Environment(host)
	return cmd, nil
}

// ForwardSessionArgs keeps forwarding ownership isolated while applying the
// common strict authentication and connection policy.
func (e Executor) ForwardSessionArgs(ctx context.Context, host, localSocket, remoteSocket string) ([]string, error) {
	if !ValidHost(host) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	return e.ForwardArgs(localSocket, remoteSocket), nil
}

// CommandError adapts streaming consumers to the same context/SSH errors as
// Git commands. Diagnostics are used for classification and are never logged.
func CommandError(ctx context.Context, host string, stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return contextError(ctx.Err())
	}
	code := 255
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	}
	return ClassifyError(host, domain.GitResult{ExitCode: code, Stderr: stderr}, err)
}

// ClassifyError maps SSH diagnostics consistently without logging them.
func ClassifyError(host string, result domain.GitResult, err error) error {
	_, classified := classifyResult(result, err, host)
	return classified
}
