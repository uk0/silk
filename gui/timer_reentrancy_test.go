package gui

import (
	"runtime"
	"testing"
)

func TestTimerNestedPumpDoesNotReenterSameArming(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var timer, sibling Timer
	defer timer.Stop()
	defer sibling.Stop()
	calls, siblingCalls, depth, maxDepth := 0, 0, 0, 0
	if !sibling.Start(0, func() { siblingCalls++ }) {
		t.Fatal("sibling start failed")
	}
	if !timer.Start(0, func() {
		calls++
		depth++
		maxDepth = max(maxDepth, depth)
		defer func() { depth-- }()
		if calls == 1 {
			for i := 0; i < 3; i++ {
				PumpTimersForTest()
			}
		}
	}) {
		t.Fatal("timer start failed")
	}
	PumpTimersForTest()
	if calls != 1 || maxDepth != 1 || siblingCalls < 1 {
		t.Fatalf("calls=%d depth=%d sibling=%d; nested UI loop reentered active callback", calls, maxDepth, siblingCalls)
	}
	PumpTimersForTest()
	if calls != 2 {
		t.Fatalf("timer did not resume after return: %d", calls)
	}
}

func TestTimerPanicDoesNotLeaveArmingMarkedRunning(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var timer Timer
	defer timer.Stop()
	calls := 0
	if !timer.Start(0, func() {
		calls++
		if calls == 1 {
			panic("timer callback probe")
		}
	}) {
		t.Fatal("start failed")
	}
	// Native Windows dispatch catches callback panics. Its explicit test pump
	// does not; either way, the arming must be available on the next pass.
	func() { defer func() { _ = recover() }(); PumpTimersForTest() }()
	PumpTimersForTest()
	if calls != 2 {
		t.Fatalf("running state leaked across panic: %d", calls)
	}
}

func TestTimerStopInsideCallbackAndRestart(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var timer Timer
	defer timer.Stop()
	oldCalls, newCalls := 0, 0
	if !timer.Start(0, func() { oldCalls++; timer.Stop(); PumpTimersForTest() }) {
		t.Fatal("start failed")
	}
	PumpTimersForTest()
	PumpTimersForTest()
	if oldCalls != 1 || timer != 0 {
		t.Fatal("stopped callback fired again", oldCalls, timer)
	}
	if !timer.Start(0, func() { newCalls++ }) {
		t.Fatal("restart failed")
	}
	PumpTimersForTest()
	if oldCalls != 1 || newCalls != 1 {
		t.Fatal("restart dispatched wrong callback", oldCalls, newCalls)
	}
}
