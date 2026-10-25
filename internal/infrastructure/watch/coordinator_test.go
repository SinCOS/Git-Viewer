package watch_test

import (
	"context"
	"errors"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/watch"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemotePollBackoffBackgroundAndCleanup(t *testing.T) {
	p := watch.Policy{Foreground: 20 * time.Millisecond, Background: 80 * time.Millisecond, Maximum: 160 * time.Millisecond, Fallback: time.Second, Debounce: time.Millisecond}
	if p.Delay(false, 3) != 160*time.Millisecond || p.Delay(true, 1) != 160*time.Millisecond {
		t.Fatal("backoff cap")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bg := make(chan bool, 1)
	results := make(chan error, 1)
	ticks := make(chan time.Time, 10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watch.Run(ctx, domain.RepositoryContext{Repository: domain.Repository{Location: "ssh"}}, p, bg, results, func() { ticks <- time.Now() })
	}()
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("no poll")
	}
	bg <- true
	results <- errors.New("offline")
	// Switch back and report success to reset the delay without racing a timer assertion.
	bg <- false
	results <- nil
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("poll did not recover")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poll leaked")
	}
}
func TestLocalNativeWatcher(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan struct{}, 10)
	done := make(chan struct{})
	p := watch.DefaultPolicy()
	p.Fallback = time.Hour
	p.Debounce = 20 * time.Millisecond
	go func() {
		defer close(done)
		watch.Run(ctx, domain.RepositoryContext{Repository: domain.Repository{Location: "local", Path: dir}}, p, nil, nil, func() { ticks <- struct{}{} })
	}()
	select {
	case <-ticks:
	case <-time.After(3 * time.Second):
		t.Fatal("watch not ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "changed"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ticks:
	case <-time.After(3 * time.Second):
		t.Fatal("native event missed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch leaked")
	}
}
func TestLocalFallbackAndWatcherCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	tick := make(chan struct{}, 10)
	p := watch.DefaultPolicy()
	p.Fallback = 30 * time.Millisecond
	p.Debounce = time.Millisecond
	repo := domain.RepositoryContext{Repository: domain.Repository{Location: "local", Path: filepath.Join(t.TempDir(), "missing")}}
	go func() { defer close(done); watch.Run(ctx, repo, p, nil, nil, func() { tick <- struct{}{} }) }()
	for i := 0; i < 2; i++ {
		select {
		case <-tick:
		case <-time.After(time.Second):
			t.Fatal("watch failure did not fall back")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch cleanup leaked")
	}
}

func precheckLayout(t *testing.T) (domain.RepositoryContext, string) {
	t.Helper()
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(git, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(git, "HEAD"):                  "ref: refs/heads/main\n",
		filepath.Join(git, "index"):                 "index-v1",
		filepath.Join(git, "refs", "heads", "main"): "0123456789abcdef0123456789abcdef01234567\n",
	}
	for path, data := range files {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo := domain.RepositoryContext{
		Repository: domain.Repository{Location: "local", Path: dir},
		GitDir:     git,
		CommonDir:  git,
	}
	return repo, dir
}

func awaitTick(t *testing.T, ticks chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ticks:
	case <-time.After(3 * time.Second):
		t.Fatal(msg)
	}
}

func drainTicks(ticks chan struct{}) {
	for {
		select {
		case <-ticks:
		default:
			return
		}
	}
}

// runPrecheck starts Run against a hand-built .git layout and settles it: the
// first tick notifies before any capture, watchers report ready, and the
// simulated successful refresh captures the initial signature.
func runPrecheck(t *testing.T, repo domain.RepositoryContext) (context.CancelFunc, chan error, chan struct{}, chan struct{}) {
	t.Helper()
	p := watch.Policy{Foreground: 20 * time.Millisecond, Background: 20 * time.Millisecond, Fallback: 20 * time.Millisecond, Maximum: 40 * time.Millisecond, Debounce: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 4)
	ticks := make(chan struct{}, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watch.Run(ctx, repo, p, nil, results, func() { ticks <- struct{}{} })
	}()
	awaitTick(t, ticks, "no initial poll")
	time.Sleep(200 * time.Millisecond)
	drainTicks(ticks)
	results <- nil
	time.Sleep(50 * time.Millisecond)
	drainTicks(ticks)
	return cancel, results, ticks, done
}

func TestPrecheckSkipsUnchangedSignature(t *testing.T) {
	repo, dir := precheckLayout(t)
	cancel, results, ticks, done := runPrecheck(t, repo)
	defer cancel()
	// Unchanged HEAD/index/refs: fallback ticks stay silent.
	select {
	case <-ticks:
		t.Fatal("refresh requested although signature unchanged")
	case <-time.After(300 * time.Millisecond):
	}
	// A changed index must notify again.
	if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("index-v2-longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	awaitTick(t, ticks, "index change did not notify")
	results <- nil
	time.Sleep(50 * time.Millisecond)
	drainTicks(ticks)
	select {
	case <-ticks:
		t.Fatal("refresh requested after capturing the new signature")
	case <-time.After(300 * time.Millisecond):
	}
	// Fail-open: an unreadable signature always notifies.
	if err := os.Remove(filepath.Join(dir, ".git", "HEAD")); err != nil {
		t.Fatal(err)
	}
	awaitTick(t, ticks, "signature read error did not fail open")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch leaked")
	}
}

func TestPrecheckDoesNotSkipWorktreeEvents(t *testing.T) {
	repo, dir := precheckLayout(t)
	cancel, _, ticks, done := runPrecheck(t, repo)
	defer cancel()
	// Editing a worktree file leaves HEAD/index/refs untouched, so a matching
	// signature must not hide the change.
	if err := os.WriteFile(filepath.Join(dir, "notes"), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	awaitTick(t, ticks, "worktree event was skipped by the precheck")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch leaked")
	}
}
