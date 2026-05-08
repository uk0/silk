package graph

import "testing"

// TestSigZoomChangedFires: SetZoomFactor with a new value triggers
// the registered callback exactly once. Locks in the contract that
// silkide's status-bar zoom % cell relies on for Ctrl+wheel zoom.
func TestSigZoomChangedFires(t *testing.T) {
	v := NewView()

	var got float64
	var calls int
	v.SigZoomChanged(func(_ interface{}, z float64) {
		got = z
		calls++
	})

	v.SetZoomFactor(2.0)
	if calls != 1 {
		t.Errorf("expected 1 callback, got %d", calls)
	}
	if got != 2.0 {
		t.Errorf("zoom passed to callback = %g, want 2.0", got)
	}
}

// TestSigZoomChangedSecondCallbackOverwrites: re-registering with a
// new callback discards the prior one. Mirrors SigSelectionChanged's
// "single observer" contract — host code that wants fan-out wraps
// its multiple observers in a tee function rather than racing for
// the single slot.
func TestSigZoomChangedSecondCallbackOverwrites(t *testing.T) {
	v := NewView()

	var firstCalls, secondCalls int
	v.SigZoomChanged(func(_ interface{}, _ float64) { firstCalls++ })
	v.SigZoomChanged(func(_ interface{}, _ float64) { secondCalls++ })

	v.SetZoomFactor(2.0)

	if firstCalls != 0 {
		t.Errorf("first callback should not fire after replacement: got %d", firstCalls)
	}
	if secondCalls != 1 {
		t.Errorf("second callback fired %d times, want 1", secondCalls)
	}
}

// TestSigZoomChangedNoCrashWithoutCallback: SetZoomFactor must remain
// safe to call when no callback has been registered. cbZoomChanged
// stays nil; the firing code branches on nil.
func TestSigZoomChangedNoCrashWithoutCallback(t *testing.T) {
	v := NewView()
	v.SetZoomFactor(0.5)
	v.SetZoomFactor(2.0)
}
