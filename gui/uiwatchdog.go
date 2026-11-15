package gui

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uk0/silk/core"
)

// UI-thread stall watchdog.
//
// Windows marks a window "Not Responding" (未响应) once its thread has gone
// ~5 seconds without taking a message. That tells the user something is stuck
// but nothing about WHAT — and a hang that only reproduces on someone else's
// machine is otherwise almost undiagnosable.
//
// An actual main/modal/drag message pump owns the watchdog's lifetime, including
// nested pumps. Construction, post-loop cleanup and later stray heartbeats do
// not arm it. Its clock is monotonic, so wall-clock corrections cannot invent a
// stall. Cancellation releases the poller without blocking UI on a user log sink.
// The event loop calls uiHeartbeat() on every iteration. A background goroutine
// watches that timestamp and, when it stops advancing, logs one warning naming
// the stall duration; with SILK_DEBUG set it also dumps every goroutine stack,
// which points straight at the blocked call. Recovery is logged too, so a slow
// operation can be told apart from a permanent deadlock.
//
// Native file dialogs run an OS-owned message loop. Their explicit wait scope
// suspends this heartbeat-only detector: time spent choosing a file is not UI
// work, and dumping every goroutine during that wait is both noisy and costly.
// The scope ends with a fresh beat, so real blocking after the dialog still
// triggers the normal deadline. This does not certify the native dialog itself.

const (
	uiStallThreshold = 5 * time.Second
	uiStallPoll      = time.Second
)

var (
	uiLastBeat   atomic.Int64 // process-relative monotonic nanoseconds, not wall time
	uiWatchClock = time.Now()
	uiMonitor    uiPumpWatchdog
	uiNativeWait nativeUIWait
)

// A heartbeat during window construction or destruction is not evidence that
// a message pump is running. Only actual main/modal/drag pumps own a lease.
// The last lease cancels its worker; a later pump gets a fresh deadline/worker.
type uiPumpWatchdog struct {
	mu     sync.Mutex
	depth  int
	active atomic.Pointer[uiWatchRun]
	watch  func(*uiWatchRun) // nil selects the real watcher; immutable while in use
}

type uiWatchRun struct {
	last    atomic.Int64
	stopped atomic.Bool
	stop    chan struct{}
	done    chan struct{}
}

func uiWatchTick() int64 { return time.Since(uiWatchClock).Nanoseconds() + 1 }

func (w *uiPumpWatchdog) enter() func() {
	w.mu.Lock()
	if w.depth == 0 {
		run := &uiWatchRun{stop: make(chan struct{}), done: make(chan struct{})}
		run.last.Store(uiWatchTick())
		w.active.Store(run)
		watch := w.watch
		if watch == nil {
			watch = watchUIThread
		}
		go watch(run)
	}
	w.depth++
	w.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.depth--
			if w.depth == 0 {
				run := w.active.Swap(nil)
				run.stopped.Store(true)
				close(run.stop)
			}
		})
	}
}

func (w *uiPumpWatchdog) beat(tick int64) {
	if run := w.active.Load(); run != nil {
		run.last.Store(tick)
	}
}

// nativeUIWait supports nested native dialogs and balances even on panic.
// Only known OS-owned modal calls use it, never arbitrary UI callbacks.
type nativeUIWait struct{ depth atomic.Int64 }

func (w *nativeUIWait) begin(heartbeat func()) func() {
	w.depth.Add(1)
	heartbeat()
	return func() {
		// Publish progress before unmasking the detector.
		heartbeat()
		w.depth.Add(-1)
	}
}

func uiWatchdogDelay(now, last int64, nativeWait bool) time.Duration {
	if last == 0 || nativeWait {
		return 0
	}
	return time.Duration(max(0, now-last))
}

// uiHeartbeat records that the event loop is still turning. Cheap enough to
// call on every message: monotonic time and atomic state only; no mutex or spawn.
func uiHeartbeat() {
	tick := uiWatchTick()
	uiLastBeat.Store(tick)
	uiMonitor.beat(tick)
}

func watchUIThread(run *uiWatchRun) {
	t := time.NewTicker(uiStallPoll)
	defer t.Stop()
	watchUIThreadTicks(run, t.C, uiWatchTick, func(recovered bool, delay time.Duration) {
		if recovered {
			core.Warn("ui: event loop resumed after ", delay.Round(time.Millisecond))
			return
		}
		core.Warn("ui: event loop has not run for ", delay.Round(time.Millisecond),
			" — UI work may be blocking. Set SILK_DEBUG=1 for goroutine stacks.")
		if !run.stopped.Load() && core.IsDebugOn() {
			core.Warn("ui: goroutine dump follows\n", goroutineDump())
		}
	})
}

// Cancellation never waits on an arbitrary user log sink from the UI thread.
// done acknowledges worker termination; an already-running sink is cooperative.
// No further tick is processed after cancellation, even if a timer is ready too.
func watchUIThreadTicks(run *uiWatchRun, ticks <-chan time.Time, now func() int64, report func(bool, time.Duration)) {
	defer close(run.done)
	var open bool
	var start int64
	for {
		select {
		case <-run.stop:
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
		}
		if run.stopped.Load() {
			return
		}
		nativeWait := uiNativeWait.depth.Load() > 0
		last := run.last.Load()
		if last == 0 {
			continue
		}
		// Sample the scope before the heartbeat to avoid observing the old
		// heartbeat together with a just-ended scope; recheck for entry too.
		idle := uiWatchdogDelay(now(), last, nativeWait || uiNativeWait.depth.Load() > 0)

		if idle < uiStallThreshold {
			// Recovered — report how long it was stuck, once.
			if open {
				open = false
				if !run.stopped.Load() {
					report(true, time.Duration(max(0, last-start)))
				}
			}
			continue
		}

		// Report a stall once per episode, not every poll.
		if open {
			continue
		}
		open, start = true, last
		if !run.stopped.Load() {
			report(false, idle)
		}
	}
}

// goroutineDump renders every goroutine's stack, growing the buffer until it
// fits so the blocked frame is never cut off.
func goroutineDump() string {
	buf := make([]byte, 64*1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		if len(buf) >= 8*1024*1024 {
			return string(buf[:n]) + "\n... (truncated)"
		}
		buf = make([]byte, len(buf)*2)
	}
}
