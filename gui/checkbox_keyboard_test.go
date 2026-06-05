package gui

import (
	"testing"
)

// TestCheckBoxKeyboardSpaceToggles: Space toggles the checked state while the
// box holds focus, and the change callback fires with the new value exactly as
// a mouse click would.
func TestCheckBoxKeyboardSpaceToggles(t *testing.T) {
	cb := NewCheckBox()
	cb.SetChecked(false)

	var got []bool
	cb.SigCheck(func(b bool) { got = append(got, b) })

	cb.OnKeyDown(KeySpace, false)
	if !cb.IsChecked() {
		t.Fatalf("after Space: IsChecked = false, want true")
	}
	cb.OnKeyDown(KeySpace, false)
	if cb.IsChecked() {
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

// TestCheckBoxKeyboardEnterToggles: Enter toggles too, for keyboard convenience.
func TestCheckBoxKeyboardEnterToggles(t *testing.T) {
	cb := NewCheckBox()
	cb.SetChecked(false)

	cb.OnKeyDown(KeyEnter, false)
	if !cb.IsChecked() {
		t.Fatalf("after Enter: IsChecked = false, want true")
	}
	cb.OnKeyDown(KeyEnter, false)
	if cb.IsChecked() {
		t.Fatalf("after second Enter: IsChecked = true, want false")
	}
}

// TestCheckBoxKeyboardDisabledIgnoresKeys: a disabled box ignores Space, so its
// state never changes and the callback never fires.
func TestCheckBoxKeyboardDisabledIgnoresKeys(t *testing.T) {
	cb := NewCheckBox()
	cb.SetChecked(false)
	cb.SetEnabled(false)

	fired := false
	cb.SigCheck(func(bool) { fired = true })

	cb.OnKeyDown(KeySpace, false)
	if cb.IsChecked() {
		t.Fatalf("disabled box toggled on Space, want unchanged")
	}
	if fired {
		t.Fatalf("disabled box fired change callback on Space, want no fire")
	}
}

// TestCheckBoxKeyboardOtherKeysIgnored: keys other than Space/Enter are a no-op.
func TestCheckBoxKeyboardOtherKeysIgnored(t *testing.T) {
	cb := NewCheckBox()
	cb.SetChecked(false)

	cb.OnKeyDown(KeyTab, false)
	cb.OnKeyDown(KeyLeft, false)
	if cb.IsChecked() {
		t.Fatalf("unrelated key toggled the box, want unchanged")
	}
}
