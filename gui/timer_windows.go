package gui

import (
	"github.com/uk0/silk/win32"
)

// timerMap maps a live win32 timer id to its callback. The WM_TIMER handler in
// window_windows.go looks the id up as each message is dispatched, one at a
// time — that lookup is also what keeps a stopped timer quiet, since KillTimer
// does not remove WM_TIMER messages already sitting in the queue.
//
// No mutex, deliberately. Timer is UI-thread-only and a lock would hide that
// instead of fixing it: SetTimer(NULL, ...) binds the timer to the message
// queue of the calling OS thread, and Go migrates an unpinned goroutine between
// threads at will, so a timer armed off the UI thread posts WM_TIMER to a
// thread with no pump and never fires. Locking the map would quiet the race
// detector and leave the timer dead. Off-thread callers go through Post()
// (uiqueue.go), as the rest of the codebase already does.
//
// Win32 wart with no cheap fix: SetTimer can hand back an id KillTimer just
// freed, so a stale WM_TIMER queued for the previous arming can dispatch once
// into the new timer's callback. Closing that needs a generation tag in the map
// value, which the dispatch site in window_windows.go would have to read too.
var timerMap = make(map[uintptr]func())

// Low precision timer in "UI thread"
type Timer uintptr

func (t *Timer) Stop() {
	if *t != 0 {
		win32.KillTimer(uintptr(*t))
		delete(timerMap, uintptr(*t))
		*t = 0
	}
}

func (t *Timer) Start(millisecond uint32, f func()) bool {
	t.Stop()
	id := win32.SetTimer(millisecond)
	if id != 0 {
		timerMap[id] = f
		*t = Timer(id)
		return true
	}
	return false
}

// processTimers fires every armed timer once, on the calling goroutine.
//
// The Win32 backend has no polling loop — a real WM_TIMER drives each callback
// from the message pump — so there is nothing to poll and nothing a test
// process can drive. This is the seam PumpTimersForTest calls, and it exists so
// that a debounced behaviour is testable identically on both backends. It fires
// unconditionally rather than checking dueness: without a pump there is no
// clock to be due against, and the caller's contract is "run what is armed".
//
// The callback may Stop() its own timer, which deletes from timerMap, so the
// ids are collected before anything runs.
func processTimers() {
	ids := make([]uintptr, 0, len(timerMap))
	for id := range timerMap {
		ids = append(ids, id)
	}
	for _, id := range ids {
		if fn := timerMap[id]; fn != nil {
			fn()
		}
	}
}

// expireTimersForTest is a no-op on this backend: processTimers here fires what
// is armed without consulting a clock, because Win32 has no polling loop to
// consult one from. It exists so PumpTimersForTest reads the same on both.
func expireTimersForTest() {}
