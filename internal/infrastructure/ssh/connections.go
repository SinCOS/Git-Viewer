package ssh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/process"
)

// Connections owns foreground OpenSSH masters. A request's cancellation never
// owns the master lifetime. The application closes the shared pool on exit.
type Connections struct {
	mu      sync.Mutex
	entries map[string]*connection
	closed  bool
}

type connection struct {
	ready  chan struct{}
	done   chan struct{}
	cancel context.CancelFunc
	path   string
	err    error // published by closing ready
	slots  chan struct{}
}

var defaultConnections Connections

// CloseConnections releases application-owned masters; it never touches user sockets.
func CloseConnections() { defaultConnections.Close() }

func (e Executor) connections() *Connections {
	if e.Connections != nil {
		return e.Connections
	}
	return &defaultConnections
}

func (e Executor) executable() string {
	if e.SSHPath != "" {
		return e.SSHPath
	}
	return "ssh"
}

func contextError(err error) error {
	code := domain.OperationCancelled
	if err == context.DeadlineExceeded {
		code = domain.OperationTimedOut
	}
	return &domain.GitError{Code: code, Cause: err}
}

func (p *Connections) get(ctx context.Context, e Executor, host string) (*connection, error) {
	if !ValidHost(host) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%t\x00%s", e.executable(), e.ConfigPath, e.KnownHostsFile, e.AllowPassword, host)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable}
	}
	if p.entries == nil {
		p.entries = make(map[string]*connection)
	}
	c := p.entries[key]
	if c != nil {
		select {
		case <-c.done:
			c = nil
		default:
		}
	}
	if c == nil {
		lifetime, cancel := context.WithCancel(context.Background())
		c = &connection{ready: make(chan struct{}), done: make(chan struct{}), cancel: cancel, slots: make(chan struct{}, 4)}
		p.entries[key] = c
		go c.serve(lifetime, e, host)
	}
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, contextError(ctx.Err())
	case <-c.ready:
		if c.err != nil {
			return nil, c.err
		}
		return c, nil
	}
}

func (c *connection) serve(ctx context.Context, e Executor, host string) {
	defer close(c.done)
	defer c.cancel()
	// /tmp is deliberately used on macOS: TMPDIR can exceed sun_path's limit.
	// MkdirTemp creates a private 0700 directory, separate for every master.
	dir, err := os.MkdirTemp("/tmp", "gv-ssh-")
	if err != nil {
		c.err = &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
		close(c.ready)
		return
	}
	defer os.RemoveAll(dir)
	c.path = filepath.Join(dir, "s")
	if len(c.path) >= 100 {
		c.err = &domain.GitError{Code: domain.RemoteUnavailable, Reason: "SSH control socket path is too long"}
		close(c.ready)
		return
	}
	args := append(e.ConnectionArgs(), "-N", "-o", "ControlMaster=yes", "-o", "ControlPersist=no", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", "-S", c.path, host)
	stderr := &connectionOutput{}
	cmd := exec.CommandContext(ctx, e.executable(), args...)
	cmd.Env = e.Environment(host)
	cmd.Stderr = stderr
	cmd.WaitDelay = 250 * time.Millisecond
	if err := cmd.Start(); err != nil {
		_, c.err = classifyResult(domain.GitResult{ExitCode: -1}, &domain.GitError{Code: domain.GitUnavailable, Cause: err}, host)
		close(c.ready)
		return
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		select {
		case err := <-exited:
			code := 255
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
				code = cmd.ProcessState.ExitCode()
			}
			if err == nil {
				err = fmt.Errorf("SSH master exited before becoming ready")
			}
			_, c.err = classifyResult(domain.GitResult{ExitCode: code, Stderr: stderr.bytes()}, err, host)
			close(c.ready)
			return
		case <-startup.Done():
			c.err = contextError(startup.Err())
			c.cancel()
			<-exited
			close(c.ready)
			return
		case <-ticker.C:
			if _, err := os.Stat(c.path); err != nil {
				continue
			}
			checkArgs := append(e.ConnectionArgs(), "-S", c.path, "-O", "check", host)
			if _, err := process.Run(startup, time.Second, 4096, e.executable(), checkArgs, e.Environment(host)); err != nil {
				continue
			}
			close(c.ready)
			<-exited
			return
		}
	}
}

// ReadConnectionArgs shares a verified master with Git reads and SFTP sessions.
// SFTP is a separate channel and may write files; controlled Git writes must use
// isolated connection arguments instead.
func (e Executor) ReadConnectionArgs(ctx context.Context, host string) ([]string, error) {
	return e.SessionArgs(ctx, host, SharedSession)
}

func (p *Connections) Close() {
	p.mu.Lock()
	p.closed = true
	entries := make([]*connection, 0, len(p.entries))
	for _, c := range p.entries {
		entries = append(entries, c)
		c.cancel()
	}
	p.mu.Unlock()
	for _, c := range entries {
		<-c.done
	}
}

// connectionOutput bounds authentication diagnostics and permits reading while
// exec's stderr copier is active. Contents are never logged.
type connectionOutput struct {
	sync.Mutex
	data []byte
}

func (w *connectionOutput) Write(data []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	w.data = append(w.data, data[:min(len(data), 64*1024-len(w.data))]...)
	return len(data), nil
}
func (w *connectionOutput) bytes() []byte {
	w.Lock()
	defer w.Unlock()
	return append([]byte(nil), w.data...)
}
