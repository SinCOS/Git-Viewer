package local

import (
	"path/filepath"
	"testing"
	"time"

	"gitviewer/internal/application"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func pump(q chan func()) {
	for {
		select {
		case f := <-q:
			f()
		default:
			return
		}
	}
}

func waitUntil(t *testing.T, q chan func(), cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pump(q)
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func newTestScheduler(a *App, minInterval, pollEvery time.Duration, starts *int) *syncScheduler {
	s := &syncScheduler{app: a, dispatch: a.Dispatch, minInterval: minInterval, pollEvery: pollEvery}
	s.begin = func() { *starts++ }
	return s
}

func newSyncTestApp() (*App, chan func()) {
	q := make(chan func(), 128)
	a := &App{Dispatch: func(f func()) { q <- f }}
	return a, q
}

// Events arriving while a quiet background refresh is in flight must neither
// cancel it nor spawn extra refreshes: exactly one trailing refresh runs.
func TestSyncSchedulerCoalescesIntoSingleTrailingRefresh(t *testing.T) {
	a, q := newSyncTestApp()
	starts := 0
	s := newTestScheduler(a, 0, 2*time.Millisecond, &starts)
	a.Loading, a.quietLoading = true, true
	for i := 0; i < 5; i++ {
		s.markDirty()
		pump(q)
	}
	if starts != 0 || !s.pending {
		t.Fatalf("in-flight refresh was disturbed: starts=%d pending=%v", starts, s.pending)
	}
	a.Loading, a.quietLoading = false, false
	waitUntil(t, q, func() bool { return starts == 1 }, "trailing refresh did not run")
	if s.pending || s.timer != nil {
		t.Fatalf("scheduler not idle after trailing refresh: pending=%v timer=%v", s.pending, s.timer != nil)
	}
}

func TestSyncSchedulerMinInterval(t *testing.T) {
	a, q := newSyncTestApp()
	starts := 0
	s := newTestScheduler(a, 40*time.Millisecond, 2*time.Millisecond, &starts)
	s.markDirty()
	pump(q)
	if starts != 1 {
		t.Fatal("first refresh did not start")
	}
	s.markDirty()
	pump(q)
	if starts != 1 || !s.pending {
		t.Fatalf("minimum interval not enforced: starts=%d pending=%v", starts, s.pending)
	}
	waitUntil(t, q, func() bool { return starts == 2 }, "gated refresh did not start after the interval")
}

func TestSyncSchedulerPausesForUserOperations(t *testing.T) {
	a, q := newSyncTestApp()
	starts := 0
	s := newTestScheduler(a, 0, 2*time.Millisecond, &starts)
	a.Loading = true
	s.markDirty()
	pump(q)
	if starts != 0 {
		t.Fatal("background refresh started during a user read")
	}
	a.Writing = true
	a.Loading = false
	time.Sleep(20 * time.Millisecond)
	pump(q)
	if starts != 0 {
		t.Fatal("background refresh started during a write")
	}
	a.Writing = false
	waitUntil(t, q, func() bool { return starts == 1 }, "refresh did not resume after the user operation")
	a.DialogOpen = true
	s.markDirty()
	pump(q)
	if starts != 1 || !s.pending {
		t.Fatalf("refresh started while a dialog was open: starts=%d pending=%v", starts, s.pending)
	}
	a.DialogOpen = false
	waitUntil(t, q, func() bool { return starts == 2 }, "refresh did not resume after the dialog closed")
}

func TestSyncSchedulerStopCancelsPending(t *testing.T) {
	a, q := newSyncTestApp()
	starts := 0
	s := newTestScheduler(a, 0, 2*time.Millisecond, &starts)
	s.markDirty()
	pump(q)
	if starts != 1 {
		t.Fatal("refresh did not start")
	}
	s.stop()
	s.markDirty()
	pump(q)
	s.dispatch(s.drain)
	pump(q)
	if starts != 1 || s.pending || s.timer != nil {
		t.Fatalf("stopped scheduler still worked: starts=%d pending=%v timer=%v", starts, s.pending, s.timer != nil)
	}
}

func newAutoApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	q := make(chan func(), 128)
	a.Dispatch = func(f func()) { q <- f }
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a, q
}

func TestStartAutoWiresScheduler(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "content\n")
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	a, q := newAutoApp(t)
	a.EnableAuto = true
	a.Open(dir)
	waitUntil(t, q, func() bool { return a.sync != nil && !a.Loading && !a.sync.pending }, "auto refresh did not settle")
	s := a.sync
	if a.autoResults == nil || a.autoBackground == nil {
		t.Fatal("auto channels not wired")
	}
	s.markDirty()
	waitUntil(t, q, func() bool { return !a.Loading && !s.pending }, "dirty mark did not run one refresh")

	off, offQueue := newAutoApp(t)
	off.Open(dir)
	waitUntil(t, offQueue, func() bool { return !off.Loading }, "open did not finish")
	if off.sync != nil {
		t.Fatal("scheduler wired although auto refresh is disabled")
	}
}
