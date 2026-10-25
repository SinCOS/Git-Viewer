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
