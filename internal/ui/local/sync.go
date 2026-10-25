package local

import "time"

// syncScheduler implements Rule 2 of the auto-refresh redesign: coalesce,
// don't cancel. Watcher events and timer ticks only mark the state dirty; at
// most one quiet refresh runs at a time and an in-flight refresh is never
// cancelled. Any number of events collapses into one trailing refresh, and a
// user operation (Loading/Writing/DialogOpen) pauses new starts until it
// finishes. minInterval throttles consecutive background refreshes.
//
// The scheduler only reads App flags; its own state is touched exclusively on
// the UI thread. AfterFunc timers re-enter through Dispatch, never directly.
type syncScheduler struct {
	app         *App
	repoID      string
	dispatch    func(func())
	minInterval time.Duration
	pollEvery   time.Duration
	begin       func() // starts one quiet refresh; replaced in tests

	timer     *time.Timer
	pending   bool
	lastStart time.Time
	stopped   bool
}

func newSyncScheduler(a *App, repoID string, dispatch func(func()), minInterval time.Duration) *syncScheduler {
	s := &syncScheduler{app: a, repoID: repoID, dispatch: dispatch, minInterval: minInterval, pollEvery: 150 * time.Millisecond}
	s.begin = func() { a.refresh(true) }
	return s
}

// canStart reports whether a new background refresh may begin now. Any
// in-flight operation pauses the scheduler; it never interrupts one. A
// background sync also counts: startBackground refuses a second sync, so
// starting now would silently drop the pending refresh instead of trailing.
func (s *syncScheduler) canStart() bool {
	a := s.app
	return !s.stopped && !a.closed && !a.Loading && !a.Writing && !a.DialogOpen && !a.syncInFlight
}

func (s *syncScheduler) markDirty() {
	if s.stopped {
		return
	}
	s.pending = true
	s.drain()
}

// drain starts the pending refresh once the app is idle and the minimum
// interval has passed; otherwise it re-arms the poll timer and retries later.
func (s *syncScheduler) drain() {
	if s.stopped {
		return
	}
	if s.app.closed {
		s.stop()
		return
	}
	if !s.pending {
		return
	}
	if !s.canStart() {
		s.schedule(s.pollEvery)
		return
	}
	if wait := s.minInterval - time.Since(s.lastStart); wait > 0 {
		s.schedule(wait)
		return
	}
	s.pending = false
	s.clearTimer()
	s.lastStart = time.Now()
	s.begin()
}

func (s *syncScheduler) schedule(d time.Duration) {
	s.clearTimer()
	s.timer = time.AfterFunc(d, func() { s.dispatch(s.drain) })
}

func (s *syncScheduler) clearTimer() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// stop releases the scheduler. startAuto calls it on repo switches and toggle
// changes; Close relies on drain noticing a.closed.
func (s *syncScheduler) stop() {
	s.stopped = true
	s.pending = false
	s.clearTimer()
}
