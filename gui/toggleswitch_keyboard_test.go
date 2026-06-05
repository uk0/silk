package gui

import (
	"testing"
)

// TestToggleSwitchKeyboardSpaceToggles: Space flips the switch while it holds
// focus, and the change callback fires with the new value exactly as a click
// would.
func TestToggleSwitchKeyboardSpaceToggles(t *testing.T) {
	sw := NewToggleSwitch()
	sw.SetChecked(false)

	var got []bool
	sw.SigToggle(func(b bool) { got = append(got, b) })

	sw.OnKeyDown(KeySpace, false)
	if !sw.IsChecked() {
		t.Fatalf("after Space: IsChecked = false, want true")
	}
	sw.OnKeyDown(KeySpace, false)
	if sw.IsChecked() {
		t.Fatalf("after second Space: IsChecked = true, want false")
	}

	want := []bool{true, false}
	if len(got) != len(want) {
		t.Fatalf("callback fired %d times, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("callback[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestToggleSwitchKeyboardEnterToggles: Enter toggles too, for keyboard
// convenience.
func TestToggleSwitchKeyboardEnterToggles(t *testing.T) {
	sw := NewToggleSwitch()
	sw.SetChecked(false)

	sw.OnKeyDown(KeyEnter, false)
	if !sw.IsChecked() {
		t.Fatalf("after Enter: IsChecked = false, want true")
	}
	sw.OnKeyDown(KeyEnter, false)
	if sw.IsChecked() {
		t.Fatalf("after second Enter: IsChecked = true, want false")
	}
}

// TestToggleSwitchKeyboardLeftRightSetState: Left forces OFF and Right forces
// ON (Qt switch direction). Each fires the callback only when the state
// actually changes; re-asserting the current state is a no-op with no callback.
func TestToggleSwitchKeyboardLeftRightSetState(t *testing.T) {
	sw := NewToggleSwitch()
	sw.SetChecked(false)

	var got []bool
	sw.SigToggle(func(b bool) { got = append(got, b) })

	// Right turns it on.
	sw.OnKeyDown(KeyRight, false)
	if !sw.IsChecked() {
		t.Fatalf("after Right: IsChecked = false, want true")
	}
	// Right again is a no-op (already on): no second callback.
	sw.OnKeyDown(KeyRight, false)
	if !sw.IsChecked() {
		t.Fatalf("after second Right: IsChecked = false, want true")
	}

	// Left turns it off.
	sw.OnKeyDown(KeyLeft, false)
	if sw.IsChecked() {
		t.Fatalf("after Left: IsChecked = true, want false")
	}
	// Left again is a no-op (already off): no extra callback.
	sw.OnKeyDown(KeyLeft, false)
	if sw.IsChecked() {
		t.Fatalf("after second Left: IsChecked = true, want false")
	}

	want := []bool{true, false}
	if len(got) != len(want) {
		t.Fatalf("callback fired %d times, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("callback[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestToggleSwitchKeyboardLeftNoOpWhenOff: Left on an already-off switch does
// nothing and fires no callback.
func TestToggleSwitchKeyboardLeftNoOpWhenOff(t *testing.T) {
	sw := NewToggleSwitch()
	sw.SetChecked(false)

	fired := false
	sw.SigToggle(func(bool) { fired = true })

	sw.OnKeyDown(KeyLeft, false)
	if sw.IsChecked() {
		t.Fatalf("Left on an off switch turned it on, want unchanged")
	}
	if fired {
		t.Fatalf("Left on an off switch fired the callback, want no fire")
	}
}

// TestToggleSwitchKeyboardDisabledIgnoresKeys: a disabled switch ignores keys,
// so its state never changes and the callback never fires.
func TestToggleSwitchKeyboardDisabledIgnoresKeys(t *testing.T) {
	sw := NewToggleSwitch()
	sw.SetChecked(false)
	sw.SetEnabled(false)

	fired := false
	sw.SigToggle(func(bool) { fired = true })

	sw.OnKeyDown(KeySpace, false)
	sw.OnKeyDown(KeyRight, false)
	if sw.IsChecked() {
		t.Fatalf("disabled switch toggled on key, want unchanged")
	}
	if fired {
		t.Fatalf("disabled switch fired change callback on key, want no fire")
	}
}
