//go:build !windows

package gui

import "testing"

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
