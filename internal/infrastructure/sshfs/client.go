package sshfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/pkg/sftp"

	"gitviewer/internal/askpass"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
)

const maxWalkFiles = 20000

// Config contains the local OpenSSH settings used for SFTP. Authentication is
// always delegated to the user's system ssh configuration and agent.
type Config struct {
	SSHPath        string
	ConfigPath     string
	KnownHostsFile string
	AllowPassword  bool
}

// FileMeta is the bounded metadata the editor needs. Path is always absolute
// and uses POSIX separators on remote hosts.
type FileMeta = domain.RemoteFileInfo

var _ domain.RemoteFileSystem = (*Client)(nil)

// Client is one SFTP session backed by the system ssh binary. All calls are
// serialized because pkg/sftp's request stream is not an editor-facing API.
type Client struct {
	host   string
	root   string
	input  string
	cfg    Config
	cmd    *exec.Cmd
	client *sftp.Client
	stdin  io.WriteCloser
	cancel context.CancelFunc
	stderr bytes.Buffer
	mu     sync.Mutex
	closed bool
}

// Open starts an SFTP subsystem and canonicalizes root before exposing it.
func Open(ctx context.Context, cfg Config, host, root string) (*Client, error) {
	if !ssh.ValidHost(host) || !path.IsAbs(root) || strings.ContainsRune(root, 0) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	executable := cfg.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	executor := ssh.Executor{
		SSHPath: executable, ConfigPath: cfg.ConfigPath, KnownHostsFile: cfg.KnownHostsFile,
		AllowPassword: cfg.AllowPassword,
	}
	args := executor.ConnectionArgs()
	args = append(args, "-s", host, "sftp")
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Env = executor.Environment(host)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	c := &Client{host: host, input: path.Clean(root), cfg: cfg, cmd: cmd, stdin: stdin, cancel: cancel}
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	client, err := sftp.NewClientPipe(stdout, stdin)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cancel()
		return nil, c.transportError(err)
	}
	c.client = client
	canonical, err := client.RealPath(root)
	if err != nil {
		_ = c.Close()
		return nil, c.transportError(err)
	}
	canonical = path.Clean(canonical)
	if !path.IsAbs(canonical) {
		_ = c.Close()
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	c.root = canonical
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, &domain.GitError{Code: domain.OperationCancelled}
	}
	return c, nil
}

func (c *Client) Host() string { return c.host }
func (c *Client) Root() string { return c.root }

// WorkingDirectory returns the initial directory supplied by the SFTP server.
func (c *Client) WorkingDirectory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, err := c.client.RealPath(".")
	if err != nil {
		return "", c.transportError(err)
	}
	return c.checkPath(value)
}

func (c *Client) transportError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(c.stderr.String())
	code := domain.RemoteUnavailable
	switch {
	case strings.Contains(msg, "host key verification failed"), strings.Contains(msg, "remote host identification has changed"):
		code = domain.HostKeyRejected
	case strings.Contains(msg, "permission denied"):
		code = domain.SshAuthFailed
		askpass.ClearHost(c.host)
	case strings.Contains(msg, "no such file or directory"):
		code = domain.InvalidRepository
	}
	return &domain.GitError{Code: code, Cause: err}
}

func (c *Client) checkPath(p string) (string, error) {
	if !path.IsAbs(p) || strings.ContainsRune(p, 0) {
		return "", &domain.GitError{Code: domain.InvalidRepository}
	}
	clean := path.Clean(p)
	within := func(root string) bool {
		return clean == root || strings.HasPrefix(clean, strings.TrimSuffix(root, "/")+"/")
	}
	if !within(c.root) && (c.input == "" || !within(c.input)) {
		return "", &domain.GitError{Code: domain.UnsafeOperation}
	}
	return clean, nil
}

// Canonical resolves a path and confines it to the opened root.
func (c *Client) Canonical(ctx context.Context, p string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", &domain.GitError{Code: domain.OperationCancelled}
	}
	if _, err := c.checkPath(p); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	resolved, err := c.client.RealPath(p)
	if err != nil {
		return "", c.transportError(err)
	}
	return c.checkPath(resolved)
}

func meta(p string, info os.FileInfo) FileMeta {
	return FileMeta{Path: p, Name: info.Name(), Dir: info.IsDir(), Symlink: info.Mode()&os.ModeSymlink != 0, Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime()}
}

func (c *Client) Stat(ctx context.Context, p string) (FileMeta, error) {
	if err := ctx.Err(); err != nil {
		return FileMeta{}, &domain.GitError{Code: domain.OperationCancelled}
	}
	if _, err := c.checkPath(p); err != nil {
		return FileMeta{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := c.client.Lstat(p)
	if err != nil {
		return FileMeta{}, c.transportError(err)
	}
	return meta(path.Clean(p), info), nil
}

func (c *Client) ReadDir(ctx context.Context, p string) ([]FileMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, &domain.GitError{Code: domain.OperationCancelled}
	}
	if _, err := c.checkPath(p); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := c.client.Lstat(p)
	if err != nil {
		return nil, c.transportError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, &domain.GitError{Code: domain.UnsafeOperation, Reason: "Only directories can be listed"}
	}
	canonical, err := c.client.RealPath(p)
	if err != nil {
		return nil, c.transportError(err)
	}
	if _, err := c.checkPath(canonical); err != nil {
		return nil, err
	}
	infos, err := c.client.ReadDir(canonical)
	if err != nil {
		return nil, c.transportError(err)
	}
	out := make([]FileMeta, 0, len(infos))
	for _, info := range infos {
		out = append(out, meta(path.Join(p, info.Name()), info))
	}
	return out, nil
}

func (c *Client) ReadFile(ctx context.Context, p string, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 8 << 20
	}
	if err := ctx.Err(); err != nil {
		return nil, &domain.GitError{Code: domain.OperationCancelled}
	}
	if _, err := c.checkPath(p); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := c.client.Lstat(p)
	if err != nil {
		return nil, c.transportError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, &domain.GitError{Code: domain.UnsafeOperation, Reason: "Only regular files can be read"}
	}
	canonical, err := c.client.RealPath(p)
	if err != nil {
		return nil, c.transportError(err)
	}
	if _, err := c.checkPath(canonical); err != nil {
		return nil, err
	}
	f, err := c.client.Open(canonical)
	if err != nil {
		return nil, c.transportError(err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, c.transportError(err)
	}
	if !info.Mode().IsRegular() {
		return nil, &domain.GitError{Code: domain.UnsafeOperation, Reason: "Only regular files can be read"}
	}
	if info.Size() > limit {
		return nil, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, c.transportError(err)
	}
	if int64(len(data)) > limit {
		return nil, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	return data, nil
}

func randomName() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".gitviewer-" + hex.EncodeToString(b[:]) + ".tmp", nil
}

// WriteFileAtomic replaces an existing regular file without following a
// symlink at the target. The caller is responsible for conflict detection.
func (c *Client) WriteFileAtomic(ctx context.Context, p string, data []byte, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return &domain.GitError{Code: domain.OperationCancelled}
	}
	if _, err := c.checkPath(p); err != nil {
		return err
	}
	parent := path.Dir(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	transport := func(stage string, err error) error {
		return c.transportError(fmt.Errorf("%s: %w", stage, err))
	}
	canonicalParent, err := c.client.RealPath(parent)
	if err != nil {
		return transport("canonical parent", err)
	}
	if _, err := c.checkPath(canonicalParent); err != nil {
		return err
	}
	target := path.Join(canonicalParent, path.Base(p))
	if _, err := c.checkPath(target); err != nil {
		return err
	}
	if info, err := c.client.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return &domain.GitError{Code: domain.UnsafeOperation, Reason: "Only regular files can be saved"}
		}
		if mode == 0 {
			mode = info.Mode().Perm()
		}
	}
	if mode == 0 {
		mode = 0o644
	}
	name, err := randomName()
	if err != nil {
		return err
	}
	temp := path.Join(canonicalParent, name)
	f, err := c.client.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return transport("create temporary", err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = c.client.Remove(temp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return transport("write temporary", err)
	}
	// SFTP servers do not universally implement fsync. Atomicity comes from
	// closing the temporary file before rename; use fsync when available.
	_ = f.Sync()
	if err := f.Close(); err != nil {
		_ = c.client.Remove(temp)
		return transport("close temporary", err)
	}
	if err := c.client.Chmod(temp, mode.Perm()); err != nil {
		_ = c.client.Remove(temp)
		return transport("chmod temporary", err)
	}
	if err := c.client.PosixRename(temp, target); err != nil {
		_ = c.client.Remove(temp)
		return transport("posix rename temporary", err)
	}
	return nil
}

// Walk returns relative slash paths of regular files under root. It never
// follows symlinked directories and stops at maxWalkFiles.
func (c *Client) Walk(ctx context.Context, root string, skip map[string]bool) ([]string, error) {
	if _, err := c.checkPath(root); err != nil {
		return nil, err
	}
	out := []string{}
	var visit func(string) error
	visit = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return &domain.GitError{Code: domain.OperationCancelled}
		}
		entries, err := c.ReadDir(ctx, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return &domain.GitError{Code: domain.OperationCancelled}
			}
			if entry.Symlink {
				continue
			}
			if entry.Dir {
				if skip[entry.Name] {
					continue
				}
				if err := visit(entry.Path); err != nil {
					return err
				}
			} else {
				rel, err := relativePath(root, entry.Path)
				if err != nil {
					return err
				}
				out = append(out, rel)
				if len(out) >= maxWalkFiles {
					return nil
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return out, nil
}

func relativePath(root, p string) (string, error) {
	root = strings.TrimSuffix(path.Clean(root), "/")
	p = path.Clean(p)
	if p == root {
		return "", errors.New("not a child path")
	}
	if !strings.HasPrefix(p, root+"/") {
		return "", errors.New("path outside root")
	}
	return strings.TrimPrefix(p, root+"/"), nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	client, stdin, cmd := c.client, c.stdin, c.cmd
	cancel := c.cancel
	c.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return nil
}
