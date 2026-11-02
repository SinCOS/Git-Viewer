//go:build unix

package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitviewer/internal/domain"
)

type poolFixture struct {
	pool    *HelperPool
	ops     *fakeOps
	starts  atomic.Int32
	factory func(context.Context, Executor, string) (HelperRemoteOps, error)
	block   chan struct{}
	clock   *fakeClock
}

type fakeClock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.offset += d
	c.mu.Unlock()
}

func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()
	local := writeLocalHelper(t, "helper-binary")
	f := &poolFixture{ops: newFakeOps(), clock: &fakeClock{}}
	pool := NewHelperPool(Executor{})
	pool.Now = f.clock.Now
	pool.LocalBinary = func(goos, goarch string) (string, error) { return local, nil }
	pool.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		f.starts.Add(1)
		if f.block != nil {
			select {
			case <-f.block:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return f.ops, nil
	}
	pool.StartClient = func(ctx context.Context, host, remotePath string) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperClientServer$")
		cmd.Env = append(os.Environ(), "GITVIEWER_HELPER_SERVER=1", "GITVIEWER_HELPER_GIT=")
		return cmd, nil
	}
	f.pool = pool
	t.Cleanup(pool.Close)
	return f
}

func TestHelperPoolSingleFlight(t *testing.T) {
	f := newPoolFixture(t)
	f.block = make(chan struct{})
	var wg sync.WaitGroup
	clients := make([]*HelperClient, 5)
	errs := make([]error, 5)
	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clients[i], errs[i] = f.pool.Acquire(context.Background(), "fixture")
		}(i)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if f.starts.Load() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(f.block)
	wg.Wait()
	if f.starts.Load() != 1 {
		t.Fatalf("started %d helpers", f.starts.Load())
	}
	for i := range clients {
		if errs[i] != nil || clients[i] == nil {
			t.Fatalf("acquire %d: %v", i, errs[i])
		}
		if clients[i] != clients[0] {
			t.Fatal("concurrent callers received different clients")
		}
	}
}

func TestHelperPoolWaitTimeoutAndBackgroundUpload(t *testing.T) {
	f := newPoolFixture(t)
	f.block = make(chan struct{})
	f.pool.StartWait = 50 * time.Millisecond
	started := time.Now()
	_, err := f.pool.Acquire(context.Background(), "fixture")
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
		t.Fatalf("expected a fast fallback, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("Acquire ignored StartWait")
	}
	close(f.block)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		client, err := f.pool.Acquire(context.Background(), "fixture")
		if err == nil && client != nil {
			if f.starts.Load() != 1 {
				t.Fatalf("upload restarted: %d", f.starts.Load())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background upload never completed")
}

func TestHelperPoolBackoffAndRecovery(t *testing.T) {
	f := newPoolFixture(t)
	var calls atomic.Int32
	fail := atomic.Bool{}
	fail.Store(true)
	f.pool.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		calls.Add(1)
		if fail.Load() {
			return nil, &domain.GitError{Code: domain.RemoteUnavailable}
		}
		return f.ops, nil
	}
	f.pool.StartWait = time.Second
	_, err := f.pool.Acquire(context.Background(), "fixture")
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
		t.Fatalf("expected a temporary failure, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected calls: %d", calls.Load())
	}
	if _, err := f.pool.Acquire(context.Background(), "fixture"); err == nil {
		t.Fatal("backoff did not reject the immediate retry")
	}
	if calls.Load() != 1 {
		t.Fatalf("backoff retried too early: %d", calls.Load())
	}
	fail.Store(false)
	f.clock.Advance(time.Second)
	client, err := f.pool.Acquire(context.Background(), "fixture")
	if err != nil || client == nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected calls after recovery: %d", calls.Load())
	}
}

func TestHelperPoolPermanentAndAuthFailures(t *testing.T) {
	f := newPoolFixture(t)
	var calls atomic.Int32
	f.pool.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		calls.Add(1)
		return nil, fmt.Errorf("%w: no sftp subsystem", ErrHelperPermanent)
	}
	if _, err := f.pool.Acquire(context.Background(), "fixture"); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("expected permanent failure, got %v", err)
	}
	if _, err := f.pool.Acquire(context.Background(), "fixture"); err == nil {
		t.Fatal("permanent failure was retried")
	}
	if calls.Load() != 1 {
		t.Fatalf("permanent failure retried: %d", calls.Load())
	}

	f = newPoolFixture(t)
	calls.Store(0)
	f.pool.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		calls.Add(1)
		return nil, &domain.GitError{Code: domain.SshAuthFailed}
	}
	_, err := f.pool.Acquire(context.Background(), "fixture")
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.SshAuthFailed {
		t.Fatalf("auth failure was hidden: %v", err)
	}
	_, _ = f.pool.Acquire(context.Background(), "fixture")
	if calls.Load() != 1 {
		t.Fatalf("auth failure retried: %d", calls.Load())
	}
}

func TestHelperPoolReconnectsAfterDeath(t *testing.T) {
	f := newPoolFixture(t)
	first, err := f.pool.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if first.cmd.Process == nil {
		t.Fatal("helper process missing")
	}
	_ = first.cmd.Process.Kill()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if !first.Alive() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.clock.Advance(time.Second)
	second, err := f.pool.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("pool reused a dead client")
	}
	if f.starts.Load() != 2 {
		t.Fatalf("expected a second start, got %d", f.starts.Load())
	}
}

func TestHelperPoolClose(t *testing.T) {
	f := newPoolFixture(t)
	client, err := f.pool.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	f.pool.Close()
	f.pool.Close()
	if client.Alive() {
		t.Fatal("client survived pool close")
	}
	if _, err := f.pool.Acquire(context.Background(), "fixture"); err == nil {
		t.Fatal("closed pool handed out a client")
	}
}

func TestHelperPoolEnsureIsNonBlocking(t *testing.T) {
	f := newPoolFixture(t)
	f.block = make(chan struct{})
	started := time.Now()
	f.pool.Ensure("fixture")
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("Ensure blocked on startup")
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if f.starts.Load() == 1 {
			close(f.block)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(f.block)
	t.Fatal("Ensure never started the helper")
}

type helperBlockingCloseOps struct {
	HelperRemoteOps
	entered chan struct{}
	release chan struct{}
}

func (o *helperBlockingCloseOps) Close() error {
	close(o.entered)
	<-o.release
	return o.HelperRemoteOps.Close()
}

func TestHelperPoolCloseWaitsForCleanupClose(t *testing.T) {
	f := newPoolFixture(t)
	ops := &helperBlockingCloseOps{HelperRemoteOps: f.ops, entered: make(chan struct{}), release: make(chan struct{})}
	f.pool.Ops = func(context.Context, Executor, string) (HelperRemoteOps, error) { return ops, nil }
	var cmd *exec.Cmd
	start := f.pool.StartClient
	f.pool.StartClient = func(ctx context.Context, host, name string) (*exec.Cmd, error) {
		var err error
		cmd, err = start(ctx, host, name)
		return cmd, err
	}
	f.pool.Ensure("fixture")
	select {
	case <-ops.entered: // cleanup owns the SFTP session and is closing it
	case <-time.After(5 * time.Second):
		t.Fatal("launch never completed handshake")
	}
	closed := make(chan struct{})
	go func() { f.pool.Close(); close(closed) }()
	<-f.pool.ctx.Done()
	close(ops.release)
	select {
	case <-closed:
	case <-time.After(7 * time.Second):
		t.Fatal("pool Close blocked during launch")
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatal("helper was not reaped")
	}
}

type helperBlockingCleanupOps struct {
	HelperRemoteOps
	entered  chan struct{}
	canceled chan struct{}
	closed   chan struct{}
}

func (o *helperBlockingCleanupOps) ReadDir(ctx context.Context, _ string) ([]os.FileInfo, error) {
	close(o.entered)
	<-ctx.Done()
	close(o.canceled)
	return nil, ctx.Err()
}

func (o *helperBlockingCleanupOps) Close() error {
	close(o.closed)
	return o.HelperRemoteOps.Close()
}

func TestHelperPoolCleanupDoesNotDelayReadinessAndCloseJoins(t *testing.T) {
	f := newPoolFixture(t)
	ops := &helperBlockingCleanupOps{HelperRemoteOps: f.ops, entered: make(chan struct{}), canceled: make(chan struct{}), closed: make(chan struct{})}
	f.pool.Ops = func(context.Context, Executor, string) (HelperRemoteOps, error) { return ops, nil }
	client, err := f.pool.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatalf("cleanup delayed readiness: %v", err)
	}
	select {
	case <-ops.entered:
	case <-time.After(time.Second):
		t.Fatal("cleanup was not scheduled")
	}
	select {
	case <-ops.closed:
		t.Fatal("closed SFTP while cleanup still used it")
	default:
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("helper not ready during cleanup: %v", err)
	}
	f.pool.Close()
	select {
	case <-ops.canceled:
	default:
		t.Fatal("pool did not cancel cleanup")
	}
	select {
	case <-ops.closed:
	default:
		t.Fatal("pool did not join cleanup session close")
	}
}

func TestHelperPoolCloseDuringLaunchClosesUnpublishedClient(t *testing.T) {
	f := newPoolFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var cmd *exec.Cmd
	start := f.pool.StartClient
	f.pool.StartClient = func(ctx context.Context, host, name string) (*exec.Cmd, error) {
		close(entered)
		<-release
		var err error
		cmd, err = start(ctx, host, name)
		return cmd, err
	}
	f.pool.Ensure("fixture")
	<-entered
	done := make(chan struct{})
	go func() { f.pool.Close(); close(done) }()
	<-f.pool.ctx.Done()
	close(release)
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("Close blocked during launch")
	}
	// Pool closure cancels the install context before the delayed StartClient
	// returns. A command that was never started has no ProcessState; one that
	// did start must have been waited for.
	if cmd.Process != nil && cmd.ProcessState == nil {
		t.Fatal("late startup process not reaped")
	}
}

func TestHelperPoolZeroValueAndConcurrentClose(t *testing.T) {
	f := newPoolFixture(t)
	pool := &HelperPool{Ops: f.pool.Ops, StartClient: f.pool.StartClient, LocalBinary: f.pool.LocalBinary}
	client, err := pool.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() { defer wg.Done(); pool.Close() }()
	}
	wg.Wait()
	if client.Alive() {
		t.Fatal("client survived zero-value pool close")
	}
	var empty HelperPool
	empty.Close()
	empty.Close()
	empty.Ensure("fixture")
	if _, err := empty.Acquire(context.Background(), "fixture"); err == nil {
		t.Fatal("closed zero-value pool accepted acquire")
	}
}

func TestHelperPoolCloseCancelsInstall(t *testing.T) {
	f := newPoolFixture(t)
	started := make(chan struct{})
	f.pool.Ops = func(ctx context.Context, _ Executor, _ string) (HelperRemoteOps, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.pool.Ensure("fixture")
	<-started
	done := make(chan struct{})
	go func() { f.pool.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel install")
	}
}

func TestHelperPoolExecutionFailuresArePermanent(t *testing.T) {
	for _, code := range []domain.ErrorCode{domain.PermissionDenied, domain.GitUnavailable, domain.CommandFailed} {
		t.Run(string(code), func(t *testing.T) {
			f := newPoolFixture(t)
			var calls atomic.Int32
			f.pool.StartClient = func(context.Context, string, string) (*exec.Cmd, error) {
				calls.Add(1)
				return nil, &domain.GitError{Code: code}
			}
			_, _ = f.pool.Acquire(context.Background(), "fixture")
			f.clock.Advance(time.Minute)
			_, _ = f.pool.Acquire(context.Background(), "fixture")
			if calls.Load() != 1 {
				t.Fatal("permanent execution failure retried")
			}
		})
	}
}

func TestHelperPoolSharedPointerAcrossExecutorCopies(t *testing.T) {
	f := newPoolFixture(t)
	executor := Executor{Helpers: f.pool}
	copied := executor
	first, err := executor.Helpers.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	second, err := copied.Helpers.Acquire(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || f.starts.Load() != 1 {
		t.Fatal("executor copies did not share one helper")
	}
}
