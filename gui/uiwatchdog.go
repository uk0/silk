package gui

import (
	"runtime"
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
	uiLastBeat   atomic.Int64 // UnixNano of the last event-loop iteration
	uiWatchOnce  atomic.Bool
	uiStallOpen  atomic.Bool // a stall is currently being reported
	uiStallStart atomic.Int64
	uiNativeWait nativeUIWait
)

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

func uiWatchdogDelay(now time.Time, last int64, nativeWait bool) time.Duration {
	if last == 0 || nativeWait {
		return 0
	}
	return now.Sub(time.Unix(0, last))
}

// uiHeartbeat records that the event loop is still turning. Cheap enough to
// call on every message: one atomic store.
func uiHeartbeat() {
	uiLastBeat.Store(time.Now().UnixNano())
	if uiWatchOnce.CompareAndSwap(false, true) {
		go watchUIThread()
	}
}

func watchUIThread() {
	t := time.NewTicker(uiStallPoll)
	defer t.Stop()
	for range t.C {
		nativeWait := uiNativeWait.depth.Load() > 0
		last := uiLastBeat.Load()
		if last == 0 {
			continue
		}
		// Sample the scope before the heartbeat to avoid observing the old
		// heartbeat together with a just-ended scope; recheck for entry too.
		idle := uiWatchdogDelay(time.Now(), last, nativeWait || uiNativeWait.depth.Load() > 0)

		if idle < uiStallThreshold {
			// Recovered — report how long it was stuck, once.
			if uiStallOpen.CompareAndSwap(true, false) {
				stalled := time.Duration(last - uiStallStart.Load())
				core.Warn("ui: event loop resumed after ", stalled.Round(time.Millisecond))
			}
			continue
		}

		// Report a stall once per episode, not every poll.
		if !uiStallOpen.CompareAndSwap(false, true) {
			continue
		}
		uiStallStart.Store(last)
		core.Warn("ui: event loop has not run for ", idle.Round(time.Millisecond),
			" — UI work may be blocking. Set SILK_DEBUG=1 for goroutine stacks.")
		if core.IsDebugOn() {
			core.Warn("ui: goroutine dump follows\n", goroutineDump())
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
