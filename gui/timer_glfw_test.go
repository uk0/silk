//go:build !windows

package gui

import (
	"testing"
	"time"
)

// TestProcessTimersSkipsATimerStoppedInTheSameBatch pins the rule that Stop()
// means no further callback, even for a timer that was already due when the
// batch began. processTimers used to snapshot every due callback and then run
// them all, so a callback that tore its owner down (a toast dismissing itself,
// a panel closing) could not cancel a sibling timer that came due in the same
// tick: that sibling fired anyway, against state the first callback had just
// destroyed. The Win32 backend never had the hole — its WM_TIMER handler looks
// each id up in timerMap one message at a time.
//
// Two timers that cancel each other make the check order-independent, since
// map iteration order is random: exactly one must survive to fire, and under
// the old snapshot-then-fire loop both did.
func TestProcessTimersSkipsATimerStoppedInTheSameBatch(t *testing.T) {
	// init() arms the 47ms idle timer; swap the map out so only ours are due.
	saved := timerMap
	timerMap = make(map[uintptr]*timerEntry)
	defer func() { timerMap = saved }()

	var a, b Timer
	fired := 0
	a.Start(0, func() {
		fired++
		b.Stop()
	})
	b.Start(0, func() {
		fired++
		a.Stop()
	})

	processTimers()

	if fired == 0 {
		t.Fatal("no due timer fired at all")
	}
	if fired != 1 {
		t.Errorf("fired = %d, want 1: a timer stopped by an earlier callback in the same batch still ran", fired)
	}
}

// TestProcessTimersRefiresARepeatingTimer guards the other half of the
// re-read: skipping a stopped id must not also skip a live one. A timer that
// is still in timerMap has to come due again on the next pass, which is what
// every repeating caller (scrollbar auto-repeat, terminal output polling,
// the 47ms idle tick) relies on.
func TestProcessTimersRefiresARepeatingTimer(t *testing.T) {
	saved := timerMap
	timerMap = make(map[uintptr]*timerEntry)
	defer func() { timerMap = saved }()

	var tm Timer
	fired := 0
	tm.Start(0, func() { fired++ })

	processTimers()
	processTimers()

	if fired != 2 {
		t.Errorf("fired = %d after two passes, want 2 (a repeating timer stopped repeating)", fired)
	}
}

// TestProcessTimersDoesNotRefireATimerFiredByANestedLoop covers the other way a
// due timer can go stale between the batch and the fire: not Stop(), but a
// nested pass that already ran it. processTimers has two call sites — MainLoop
// and modalLoop (window_glfw.go) — and a callback that opens a modal runs the
// second one inside the first one's fire loop. Presence in timerMap is then not
// enough: a sibling still due when the batch began can be fired by the nested
// loop and fired again the moment the modal closes, two callbacks a few
// microseconds apart on a timer with a multi-second interval.
//
// Time is moved by hand rather than slept: the outer pass has to see both
// timers due, the nested pass has to see exactly the sibling due, and a real
// sleep would make the interval the test's tolerance budget. Both callbacks
// enter the "modal" so the assertion does not depend on map iteration order —
// whichever of the two the batch happens to fire first, each must run once.
func TestProcessTimersDoesNotRefireATimerFiredByANestedLoop(t *testing.T) {
	saved := timerMap
	timerMap = make(map[uintptr]*timerEntry)
	defer func() { timerMap = saved }()

	const interval = 2 * time.Second
	const elapse = 3 * time.Second // > interval, so a backdated timer reads as due

	backdate := func(tm Timer) {
		timerMu.Lock()
		defer timerMu.Unlock()
		if e := timerMap[uintptr(tm)]; e != nil {
			e.lastFire = e.lastFire.Add(-elapse)
		}
	}

	var a, b Timer
	aFired, bFired := 0, 0
	entered := false
	// Stands in for a callback that opens a modal dialog: elapse enough time
	// for the sibling to come due again, then run the modal loop's own
	// processTimers. Only the first callback nests; a real modal loop keeps
	// pumping, but one nested pass is all the double fire needs.
	enterModal := func(sibling Timer) {
		if entered {
			return
		}
		entered = true
		backdate(sibling)
		processTimers()
	}

	a.Start(uint32(interval/time.Millisecond), func() { aFired++; enterModal(b) })
	b.Start(uint32(interval/time.Millisecond), func() { bFired++; enterModal(a) })
	backdate(a)
	backdate(b)

	processTimers()

	if aFired != 1 || bFired != 1 {
		t.Errorf("aFired = %d, bFired = %d, want 1 and 1: a timer already fired by the nested loop fired again when the outer batch resumed", aFired, bFired)
	}
}
