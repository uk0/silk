//go:build !windows

package gui

import "testing"

func TestPostAfterNativeCleanupRunsAfterRetirementOutsideCleanup(t *testing.T) {
	previous := pendingNativeDestruction
	pendingNativeDestruction = nil
	resetUIQueue()
	t.Cleanup(func() { pendingNativeDestruction = previous; resetUIQueue() })
	destroyed, restored, inside, ran := false, false, false, false
	pendingNativeDestruction = append(pendingNativeDestruction, func() { destroyed = true }, func() { restored = true })
	PostAfterNativeCleanup(nil)
	PostAfterNativeCleanup(func() {
		if !destroyed || !restored || inside {
			t.Fatal("continuation ran before focus/native cleanup completed")
		}
		ran = true
	})
	drainUITasks() // a nested modal may drain normal Posts before outer cleanup
	if ran {
		t.Fatal("normal UI queue bypassed native cleanup")
	}
	inside = true
	flushNativeDestruction()
	inside = false
	if ran {
		t.Fatal("callback ran inline during native destruction")
	}
	drainUITasks()
	if !ran {
		t.Fatal("outer continuation was lost")
	}
	flushNativeDestruction()
	drainUITasks()
}

func TestPostAfterNativeCleanupReentrantWaitsForNextBoundary(t *testing.T) {
	previous := pendingNativeDestruction
	pendingNativeDestruction = nil
	resetUIQueue()
	t.Cleanup(func() { pendingNativeDestruction = previous; resetUIQueue() })
	ran := false
	pendingNativeDestruction = append(pendingNativeDestruction, func() { PostAfterNativeCleanup(func() { ran = true }) })
	flushNativeDestruction()
	drainUITasks()
	if ran {
		t.Fatal("reentrant registration ran in the old native batch")
	}
	flushNativeDestruction()
	drainUITasks()
	if !ran {
		t.Fatal("next-boundary registration was dropped")
	}
}
