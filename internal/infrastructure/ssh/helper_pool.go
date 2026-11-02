package ssh

import (
	"context"
	"errors"
	"os/exec"
	"path"
	"sync"
	"time"

	"gitviewer/internal/domain"
)

const (
	helperStartWait    = 3 * time.Second
	helperInstallLimit = 45 * time.Second
	helperBackoffMin   = 500 * time.Millisecond
	helperBackoffMax   = 30 * time.Second
	helperHeartbeat    = 30 * time.Second
)

// HelperPool owns one resident helper per SSH host. Startup is single-flight,
// failures are classified as permanent or temporary, and callers can always
// fall back to the ordinary SSH CLI when a helper is not ready.
type HelperPool struct {
	Executor    Executor
	Ops         HelperOpsFactory
	LocalBinary func(goos, goarch string) (string, error)
	RemoteBase  string
	StartWait   time.Duration
	BackoffMin  time.Duration
	BackoffMax  time.Duration
	Now         func() time.Time
	// StartClient starts the uploaded helper. Tests replace it; production
	// uses Executor.Command over the shared ControlMaster.
	StartClient func(context.Context, string, string) (*exec.Cmd, error)

	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	closeDone chan struct{}
	entries   map[string]*helperEntry
	workers   sync.WaitGroup
}

type helperEntry struct {
	host        string
	client      *HelperClient
	starting    bool
	ready       chan struct{}
	permanent   error
	lastErr     error
	nextAttempt time.Time
	backoff     time.Duration
}

// NewHelperPool creates a pool bound to one Executor configuration.
func NewHelperPool(executor Executor) *HelperPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &HelperPool{
		Executor: executor,
		Ops:      openSFTPHelperOps,
		ctx:      ctx,
		cancel:   cancel,
		entries:  map[string]*helperEntry{},
	}
}

// initLocked makes the zero value usable. Callers hold mu before adding any
// worker, so Close can safely wait without racing a new WaitGroup.Add.
func (p *HelperPool) initLocked() {
	if p.ctx == nil {
		p.ctx, p.cancel = context.WithCancel(context.Background())
	}
	if p.entries == nil {
		p.entries = make(map[string]*helperEntry)
	}
}

func (p *HelperPool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *HelperPool) startWait() time.Duration {
	if p.StartWait > 0 {
		return p.StartWait
	}
	return helperStartWait
}

func (p *HelperPool) backoffMin() time.Duration {
	if p.BackoffMin > 0 {
		return p.BackoffMin
	}
	return helperBackoffMin
}

func (p *HelperPool) backoffMax() time.Duration {
	if p.BackoffMax > 0 {
		return p.BackoffMax
	}
	return helperBackoffMax
}

// Acquire returns a ready client or an error. It waits at most StartWait; the
// upload continues in the background after a timeout so the next call can use
// the helper without repeating the install.
func (p *HelperPool) Acquire(ctx context.Context, host string) (*HelperClient, error) {
	if !ValidHost(host) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	deadline := time.Now().Add(p.startWait())
	for {
		if err := ctx.Err(); err != nil {
			return nil, contextError(err)
		}
		p.mu.Lock()
		p.initLocked()
		if p.closed {
			p.mu.Unlock()
			return nil, &domain.GitError{Code: domain.RemoteUnavailable, Reason: "Remote helper pool is closed"}
		}
		entry := p.entries[host]
		if entry == nil {
			entry = &helperEntry{host: host}
			p.entries[host] = entry
		}
		if entry.client != nil && entry.client.Alive() {
			client := entry.client
			p.mu.Unlock()
			return client, nil
		}
		if entry.client != nil {
			entry.client = nil
		}
		if entry.permanent != nil {
			err := entry.permanent
			p.mu.Unlock()
			return nil, err
		}
		if p.now().Before(entry.nextAttempt) {
			err := entry.lastErr
			if err == nil {
				err = &domain.GitError{Code: domain.RemoteUnavailable}
			}
			p.mu.Unlock()
			return nil, err
		}
		if !entry.starting {
			entry.starting = true
			entry.ready = make(chan struct{})
			p.workers.Add(1)
			go p.start(entry)
		}
		ready := entry.ready
		p.mu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, &domain.GitError{Code: domain.RemoteUnavailable, Reason: "Remote helper is still starting"}
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ready:
			timer.Stop()
		case <-ctx.Done():
			timer.Stop()
			return nil, contextError(ctx.Err())
		case <-timer.C:
			return nil, &domain.GitError{Code: domain.RemoteUnavailable, Reason: "Remote helper is still starting"}
		case <-p.ctx.Done():
			timer.Stop()
			return nil, &domain.GitError{Code: domain.RemoteUnavailable, Reason: "Remote helper pool is closed"}
		}
	}
}

// Ensure starts the helper without waiting for it. It is used to warm a host
// after a remote workspace opens.
func (p *HelperPool) Ensure(host string) {
	if !ValidHost(host) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initLocked()
	if p.closed {
		return
	}
	entry := p.entries[host]
	if entry == nil {
		entry = &helperEntry{host: host}
		p.entries[host] = entry
	}
	if entry.starting || entry.permanent != nil || entry.client != nil && entry.client.Alive() {
		return
	}
	if p.now().Before(entry.nextAttempt) {
		return
	}
	entry.starting = true
	entry.ready = make(chan struct{})
	p.workers.Add(1)
	go p.start(entry)
}

func (p *HelperPool) start(entry *helperEntry) {
	defer p.workers.Done()
	defer func() {
		p.mu.Lock()
		entry.starting = false
		ready := entry.ready
		entry.ready = nil
		p.mu.Unlock()
		if ready != nil {
			close(ready)
		}
	}()
	client, err := p.launch(entry.host)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		if client != nil {
			_ = client.Close()
		}
		return
	}
	defer p.mu.Unlock()
	if err != nil {
		entry.lastErr = err
		if permanentFailure(err) {
			entry.permanent = err
			return
		}
		backoff := entry.backoff
		if backoff == 0 {
			backoff = p.backoffMin()
		} else if backoff >= p.backoffMax()/2 {
			backoff = p.backoffMax()
		} else {
			backoff *= 2
		}
		if backoff > p.backoffMax() {
			backoff = p.backoffMax()
		}
		entry.backoff = backoff
		entry.nextAttempt = p.now().Add(backoff)
		return
	}
	entry.client = client
	entry.backoff = 0
	entry.nextAttempt = time.Time{}
	entry.lastErr = nil
	p.workers.Add(2)
	go p.watch(entry, client)
	go p.heartbeat(client)
}

func (p *HelperPool) launch(host string) (*HelperClient, error) {
	ctx, cancel := context.WithTimeout(p.ctx, helperInstallLimit)
	cleanupOwnsOps := false
	defer func() {
		if !cleanupOwnsOps {
			cancel()
		}
	}()
	factory := p.Ops
	if factory == nil {
		factory = openSFTPHelperOps
	}
	ops, err := factory(ctx, p.Executor, host)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !cleanupOwnsOps {
			_ = ops.Close()
		}
	}()
	installer := &HelperInstaller{Ops: ops, LocalBinary: p.LocalBinary, RemoteBase: p.RemoteBase, Now: p.Now}
	installed, err := installer.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	start := p.StartClient
	if start == nil {
		start = func(ctx context.Context, host, remotePath string) (*exec.Cmd, error) {
			return p.Executor.Command(ctx, host, SharedSession, []string{remotePath, "--serve"}, "")
		}
	}
	cmd, err := start(p.ctx, host, installed.Path)
	if err != nil {
		return nil, err
	}
	client, err := startHelperClient(ctx, cmd)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if !p.closed {
		// Transfer the SFTP session and its deadline to one tracked worker.
		// No installer operations use it after this point. Close cancels the
		// parent context and joins this worker before returning.
		cleanupOwnsOps = true
		p.workers.Add(1)
		go func() {
			defer p.workers.Done()
			defer cancel()
			defer ops.Close()
			installer.cleanup(ctx, path.Dir(path.Dir(installed.Path)), installed.BuildID)
		}()
	}
	p.mu.Unlock()
	return client, nil
}

func (p *HelperPool) watch(entry *helperEntry, client *HelperClient) {
	defer p.workers.Done()
	// Reap the process and join the client's transport goroutines even if an
	// Acquire already noticed death and installed a replacement entry.
	defer client.Close()
	select {
	case <-client.Done():
	case <-p.ctx.Done():
		return
	}
	p.mu.Lock()
	if entry.client == client {
		entry.client = nil
		entry.lastErr = &domain.GitError{Code: domain.RemoteUnavailable, Reason: "Remote helper exited"}
		if entry.nextAttempt.Before(p.now()) {
			entry.nextAttempt = p.now()
		}
	}
	p.mu.Unlock()
}

func (p *HelperPool) heartbeat(client *HelperClient) {
	defer p.workers.Done()
	ticker := time.NewTicker(helperHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-client.Done():
			return
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
			err := client.Ping(ctx)
			cancel()
			if err != nil {
				_ = client.Close()
				return
			}
		}
	}
}

// Close stops every helper. Callers close the pool before Connections so the
// helper sessions end while their master is still alive.
func (p *HelperPool) Close() {
	p.mu.Lock()
	p.initLocked()
	if p.closed {
		done := p.closeDone
		p.mu.Unlock()
		<-done
		return
	}
	p.closed = true
	p.closeDone = make(chan struct{})
	clients := make([]*HelperClient, 0, len(p.entries))
	for _, entry := range p.entries {
		if entry.client != nil {
			clients = append(clients, entry.client)
			entry.client = nil
		}
	}
	p.mu.Unlock()
	p.cancel()
	for _, client := range clients {
		_ = client.Close()
	}
	p.workers.Wait()
	close(p.closeDone)
}

func permanentFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrHelperPermanent) {
		return true
	}
	var ge *domain.GitError
	if errors.As(err, &ge) {
		switch ge.Code {
		case domain.SshAuthFailed, domain.HostKeyRejected, domain.InvalidRepository, domain.ProtocolError,
			domain.PermissionDenied, domain.GitUnavailable, domain.CommandFailed:
			return true
		}
	}
	return false
}
