package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"

	"gitviewer/internal/domain"
)

// HelperRemoteOps is the small remote filesystem/command surface used to
// install and probe the helper. OpenWrite creates a new file exclusively;
// implementations must never follow or truncate an existing destination.
type HelperRemoteOps interface {
	Home(context.Context) (string, error)
	Lstat(context.Context, string) (os.FileInfo, error)
	ReadDir(context.Context, string) ([]os.FileInfo, error)
	Mkdir(context.Context, string, os.FileMode) error
	OpenRead(context.Context, string) (io.ReadCloser, error)
	OpenWrite(context.Context, string) (io.WriteCloser, error)
	ReadFile(context.Context, string, int64) ([]byte, error)
	Chmod(context.Context, string, os.FileMode) error
	Rename(context.Context, string, string) error
	Remove(context.Context, string) error
	RemoveAll(context.Context, string) error
	Run(context.Context, []string, time.Duration) (domain.GitResult, error)
	Close() error
}

// HelperOpsFactory opens operations on the shared ControlMaster's subsystem.
type HelperOpsFactory func(context.Context, Executor, string) (HelperRemoteOps, error)

const helperSFTPRequestLimit = 10 * time.Second

func openSFTPHelperOps(ctx context.Context, cfg Executor, host string) (HelperRemoteOps, error) {
	if !ValidHost(host) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	// Bound even callers that do not supply an install deadline.
	runCtx, cancel := context.WithTimeout(ctx, helperInstallLimit)
	cmd, err := cfg.Command(runCtx, host, SharedSession, nil, "sftp")
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		cancel()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	ops := &sftpHelperOps{cfg: cfg, host: host, cmd: cmd, stdin: stdin, stdout: stdout, cancel: cancel, stderr: &connectionOutput{}, waitDone: make(chan struct{})}
	cmd.Stderr = ops.stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		ops.abort()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	go func() {
		_ = cmd.Wait()
		close(ops.waitDone)
	}()
	ops.stop = context.AfterFunc(runCtx, ops.abort)
	finish := ops.guard(ctx)
	client, handshakeErr := sftp.NewClientPipe(stdout, stdin)
	ops.client = client
	err = finish(handshakeErr)
	if err != nil {
		ctxErr := runCtx.Err()
		_ = ops.Close()
		var ge *domain.GitError
		if errors.As(err, &ge) && (ge.Code == domain.OperationTimedOut || ge.Code == domain.OperationCancelled) {
			return nil, err
		}
		if ctxErr != nil {
			return nil, contextError(ctxErr)
		}
		if handshakeErr != nil {
			return nil, ops.wrap(handshakeErr)
		}
		return nil, err
	}
	ops.client = client
	return ops, nil
}

type sftpHelperOps struct {
	cfg       Executor
	host      string
	client    *sftp.Client
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	cancel    context.CancelFunc
	stop      func() bool
	stderr    *connectionOutput
	waitDone  chan struct{}
	abortOnce sync.Once
	closeOnce sync.Once
}

// abort closes the underlying pipes before touching sftp.Client.Close, which
// can otherwise wait forever for an outstanding protocol response or write.
func (o *sftpHelperOps) abort() {
	o.abortOnce.Do(func() {
		if o.cancel != nil {
			o.cancel()
		}
		if o.cmd != nil && o.cmd.Process != nil {
			_ = o.cmd.Process.Kill()
		}
		if o.stdin != nil {
			_ = o.stdin.Close()
		}
		if o.stdout != nil {
			_ = o.stdout.Close()
		}
	})
}

// A timeout aborts this dedicated SFTP channel (never the shared master).
// Every operation, including handle reads/writes/closes, gets this guard.
func (o *sftpHelperOps) guard(ctx context.Context) func(error) error {
	runCtx, cancel := context.WithTimeout(ctx, helperSFTPRequestLimit)
	stop := context.AfterFunc(runCtx, o.abort)
	if runCtx.Err() != nil {
		o.abort()
	}
	return func(err error) error {
		stop()
		ctxErr := runCtx.Err()
		cancel()
		if ctxErr != nil {
			return contextError(ctxErr)
		}
		return o.wrap(err)
	}
}

func (o *sftpHelperOps) Home(ctx context.Context) (string, error) {
	finish := o.guard(ctx)
	home, err := o.client.RealPath(".")
	if err := finish(err); err != nil {
		return "", err
	}
	if !strings.HasPrefix(home, "/") || strings.ContainsRune(home, 0) {
		return "", fmt.Errorf("%w: remote home directory is invalid", ErrHelperPermanent)
	}
	if home != "/" {
		home = strings.TrimSuffix(home, "/")
	}
	return home, nil
}

func (o *sftpHelperOps) Lstat(ctx context.Context, name string) (os.FileInfo, error) {
	finish := o.guard(ctx)
	info, err := o.client.Lstat(name)
	return info, finish(err)
}

func (o *sftpHelperOps) ReadDir(ctx context.Context, name string) ([]os.FileInfo, error) {
	finish := o.guard(ctx)
	entries, err := o.client.ReadDir(name)
	return entries, finish(err)
}

func (o *sftpHelperOps) Mkdir(ctx context.Context, name string, mode os.FileMode) error {
	finish := o.guard(ctx)
	if err := o.client.Mkdir(name); err != nil {
		return finish(err)
	}
	return finish(o.client.Chmod(name, mode))
}

func (o *sftpHelperOps) OpenRead(ctx context.Context, name string) (io.ReadCloser, error) {
	finish := o.guard(ctx)
	file, err := o.client.Open(name)
	if err := finish(err); err != nil {
		return nil, err
	}
	return &helperSFTPFile{file: file, ops: o, ctx: ctx}, nil
}

func (o *sftpHelperOps) OpenWrite(ctx context.Context, name string) (io.WriteCloser, error) {
	finish := o.guard(ctx)
	file, err := o.client.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err := finish(err); err != nil {
		return nil, err
	}
	return &helperSFTPFile{file: file, ops: o, ctx: ctx}, nil
}

type helperSFTPFile struct {
	file *sftp.File
	ops  *sftpHelperOps
	ctx  context.Context
}

func (f *helperSFTPFile) Read(data []byte) (int, error) {
	finish := f.ops.guard(f.ctx)
	n, err := f.file.Read(data)
	if err == io.EOF {
		if ctxErr := finish(nil); ctxErr != nil {
			return n, ctxErr
		}
		return n, io.EOF
	}
	return n, finish(err)
}

func (f *helperSFTPFile) Write(data []byte) (int, error) {
	finish := f.ops.guard(f.ctx)
	n, err := f.file.Write(data)
	return n, finish(err)
}

func (f *helperSFTPFile) Close() error {
	finish := f.ops.guard(f.ctx)
	return finish(f.file.Close())
}

func (o *sftpHelperOps) ReadFile(ctx context.Context, name string, limit int64) ([]byte, error) {
	file, err := o.OpenRead(ctx, name)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	return data, nil
}

func (o *sftpHelperOps) Chmod(ctx context.Context, name string, mode os.FileMode) error {
	finish := o.guard(ctx)
	return finish(o.client.Chmod(name, mode))
}

func (o *sftpHelperOps) Rename(ctx context.Context, oldPath, newPath string) error {
	finish := o.guard(ctx)
	return finish(o.client.PosixRename(oldPath, newPath))
}

func (o *sftpHelperOps) Remove(ctx context.Context, name string) error {
	finish := o.guard(ctx)
	return finish(o.client.Remove(name))
}

func (o *sftpHelperOps) RemoveAll(ctx context.Context, name string) error {
	info, err := o.Lstat(ctx, name)
	if isNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return o.Remove(ctx, name)
	}
	entries, err := o.ReadDir(ctx, name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// Never let a malformed server entry escape the selected directory.
		if entry.Name() == "." || entry.Name() == ".." || strings.ContainsAny(entry.Name(), "/\x00") {
			return fmt.Errorf("%w: invalid SFTP directory entry", ErrHelperPermanent)
		}
		child := strings.TrimSuffix(name, "/") + "/" + entry.Name()
		if err := o.RemoveAll(ctx, child); err != nil && !isNotExist(err) {
			return err
		}
	}
	finish := o.guard(ctx)
	err = o.client.RemoveDirectory(name)
	if isNotExist(err) {
		err = nil
	}
	return finish(err)
}

func (o *sftpHelperOps) Run(ctx context.Context, argv []string, timeout time.Duration) (domain.GitResult, error) {
	return o.cfg.runRemote(ctx, o.host, timeout, 64*1024, argv)
}

func (o *sftpHelperOps) Close() error {
	o.closeOnce.Do(func() {
		if o.stop != nil {
			o.stop()
		}
		o.abort()
		if o.client != nil {
			_ = o.client.Close()
		}
		if o.waitDone != nil {
			<-o.waitDone
		}
	})
	return nil
}

func (o *sftpHelperOps) wrap(err error) error {
	if err == nil || isNotExist(err) {
		return err
	}
	var ge *domain.GitError
	if errors.As(err, &ge) || errors.Is(err, ErrHelperPermanent) {
		return err
	}
	var diagnostics []byte
	if o.stderr != nil {
		diagnostics = o.stderr.bytes()
	}
	classified := ClassifyError(o.host, domain.GitResult{ExitCode: 255, Stderr: diagnostics}, err)
	if errors.As(classified, &ge) && (ge.Code == domain.SshAuthFailed || ge.Code == domain.HostKeyRejected) {
		return classified
	}
	message := strings.ToLower(string(diagnostics))
	if strings.Contains(message, "subsystem request failed") || strings.Contains(message, "subsystem not found") ||
		strings.Contains(message, "sftp-server: not found") || strings.Contains(message, "sftp-server: no such file") {
		return fmt.Errorf("%w: SFTP subsystem unavailable", ErrHelperPermanent)
	}
	var status *sftp.StatusError
	if errors.As(err, &status) {
		switch status.FxCode() {
		case sftp.ErrSSHFxNoConnection, sftp.ErrSSHFxConnectionLost:
			return &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
		case sftp.ErrSSHFxPermissionDenied:
			return &domain.GitError{Code: domain.PermissionDenied, Cause: err}
		default:
			return fmt.Errorf("%w: SFTP filesystem operation failed", ErrHelperPermanent)
		}
	}
	if errors.Is(err, os.ErrPermission) {
		return &domain.GitError{Code: domain.PermissionDenied, Cause: err}
	}
	return classified
}

func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	var status *sftp.StatusError
	return errors.Is(err, os.ErrNotExist) || errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxNoSuchFile
}
