package gui

import "testing"

// TestComboBoxDisabledIgnoresKeys: OnLeftDown / OnLeftUp already refuse a
// disabled combo, but the keyboard path did not — a greyed-out box still
// changed its selection under the arrows, Home/End and type-ahead. Widget-level
// enablement is not enforced by the event dispatcher; each widget checks it.
func TestComboBoxDisabledIgnoresKeys(t *testing.T) {
	cb := newTestCombo("Apple", "Banana", "Cherry")
	cb.SetEnabled(false)

	// Each case restarts from index 1 so an up/down pair cannot cancel out.
	// setActiveIndex is the programmatic path and stays live while disabled —
	// only user input is refused.
	cases := []struct {
		name string
		act  func()
	}{
		{"Down", func() { cb.OnKeyDown(KeyDown, false) }},
		{"Up", func() { cb.OnKeyDown(KeyUp, false) }},
		{"Home", func() { cb.OnKeyDown(KeyHome, false) }},
		{"End", func() { cb.OnKeyDown(KeyEnd, false) }},
		{"type-ahead", func() { cb.OnKeyDown('C', false) }},
	}
	for _, c := range cases {
		cb.setActiveIndex(1)
		c.act()
		if cb.ActiveIndex() != 1 {
			t.Errorf("disabled %s: ActiveIndex() = %d, want 1", c.name, cb.ActiveIndex())
		}
	}
}
