package gui

import "testing"

// newTestRadioGroup builds a RadioGroup with one RadioButton per label.
func newTestRadioGroup(labels ...string) (*RadioGroup, []*RadioButton) {
	g := NewRadioGroup()
	rbs := make([]*RadioButton, len(labels))
	for i, s := range labels {
		rbs[i] = NewRadioButton(s, g)
	}
	return g, rbs
}

// checkedIndex returns the index of the single checked radio, or -1.
func checkedIndex(rbs []*RadioButton) int {
	for i, rb := range rbs {
		if rb.IsChecked() {
			return i
		}
	}
	return -1
}

// TestRadioButtonNextEnabledHelper covers the pure next/prev helper: forward,
// backward, wrap at both ends, skipping disabled, and all-disabled / empty.
func TestRadioButtonNextEnabledHelper(t *testing.T) {
	_, rbs := newTestRadioGroup("A", "B", "C")

	if got := nextEnabledRadio(rbs, 0, +1); got != 1 {
		t.Errorf("forward from 0 = %d, want 1", got)
	}
	if got := nextEnabledRadio(rbs, 1, -1); got != 0 {
		t.Errorf("backward from 1 = %d, want 0", got)
	}
	// Wrap: forward past the last lands on the first.
	if got := nextEnabledRadio(rbs, 2, +1); got != 0 {
		t.Errorf("forward wrap from 2 = %d, want 0", got)
	}
	// Wrap: backward before the first lands on the last.
	if got := nextEnabledRadio(rbs, 0, -1); got != 2 {
		t.Errorf("backward wrap from 0 = %d, want 2", got)
	}

	// Disabled radios are skipped.
	rbs[1].SetEnabled(false)
	if got := nextEnabledRadio(rbs, 0, +1); got != 2 {
		t.Errorf("forward skipping disabled from 0 = %d, want 2", got)
	}
	if got := nextEnabledRadio(rbs, 2, -1); got != 0 {
		t.Errorf("backward skipping disabled from 2 = %d, want 0", got)
	}

	// No enabled sibling -> -1 (every button disabled).
	rbs[0].SetEnabled(false)
	rbs[2].SetEnabled(false)
	if got := nextEnabledRadio(rbs, 0, +1); got != -1 {
		t.Errorf("all disabled = %d, want -1", got)
	}

	// Empty slice -> -1.
	if got := nextEnabledRadio(nil, 0, +1); got != -1 {
		t.Errorf("empty = %d, want -1", got)
	}
}

// TestRadioSpaceSelects verifies Space selects the focused radio and that the
// previously selected sibling deselects (mutual exclusion) with callbacks.
func TestRadioButtonSpaceSelects(t *testing.T) {
	_, rbs := newTestRadioGroup("A", "B", "C")

	fired := 0
	rbs[1].SetChangedCallback(func(_ interface{}, _ bool) { fired++ })

	rbs[0].SetChecked(true)
	rbs[1].OnKeyDown(KeySpace, false)

	if checkedIndex(rbs) != 1 {
		t.Fatalf("after Space on B: checked index = %d, want 1", checkedIndex(rbs))
	}
	if rbs[0].IsChecked() {
		t.Error("A should have deselected when B was selected")
	}
	if fired == 0 {
		t.Error("changed callback did not fire on Space-select")
	}

	// Space on an already-checked radio is a no-op (no extra callback).
	before := fired
	rbs[1].OnKeyDown(KeySpace, false)
	if fired != before {
		t.Errorf("Space on already-checked radio fired callback: %d -> %d", before, fired)
	}

	// Disabled radio ignores Space.
	rbs[2].SetEnabled(false)
	rbs[2].OnKeyDown(KeySpace, false)
	if rbs[2].IsChecked() {
		t.Error("disabled radio must not select on Space")
	}
}

// TestRadioArrowNavigation verifies Down/Right move to the next radio and
// Up/Left to the previous, wrapping at the ends, with selection following.
func TestRadioButtonArrowNavigation(t *testing.T) {
	_, rbs := newTestRadioGroup("A", "B", "C")

	rbs[0].SetChecked(true)

	// Down: A -> B.
	rbs[0].OnKeyDown(KeyDown, false)
	if checkedIndex(rbs) != 1 {
		t.Fatalf("Down from A: checked = %d, want 1 (B)", checkedIndex(rbs))
	}
	if !rbs[1].HasFocus() {
		t.Error("Down should move focus to B")
	}

	// Right: B -> C (continues from the now-focused B).
	rbs[1].OnKeyDown(KeyRight, false)
	if checkedIndex(rbs) != 2 {
		t.Fatalf("Right from B: checked = %d, want 2 (C)", checkedIndex(rbs))
	}

	// Down wraps: C -> A.
	rbs[2].OnKeyDown(KeyDown, false)
	if checkedIndex(rbs) != 0 {
		t.Fatalf("Down wrap from C: checked = %d, want 0 (A)", checkedIndex(rbs))
	}

	// Up wraps: A -> C.
	rbs[0].OnKeyDown(KeyUp, false)
	if checkedIndex(rbs) != 2 {
		t.Fatalf("Up wrap from A: checked = %d, want 2 (C)", checkedIndex(rbs))
	}

	// Left: C -> B.
	rbs[2].OnKeyDown(KeyLeft, false)
	if checkedIndex(rbs) != 1 {
		t.Fatalf("Left from C: checked = %d, want 1 (B)", checkedIndex(rbs))
	}
}

// TestRadioArrowSkipsDisabled verifies arrow navigation jumps over disabled
// radios in the group.
func TestRadioButtonArrowSkipsDisabled(t *testing.T) {
	_, rbs := newTestRadioGroup("A", "B", "C")

	rbs[1].SetEnabled(false) // disable the middle radio
	rbs[0].SetChecked(true)

	// Down from A skips disabled B -> C.
	rbs[0].OnKeyDown(KeyDown, false)
	if checkedIndex(rbs) != 2 {
		t.Fatalf("Down skipping disabled B: checked = %d, want 2 (C)", checkedIndex(rbs))
	}

	// Up from C skips disabled B -> A.
	rbs[2].OnKeyDown(KeyUp, false)
	if checkedIndex(rbs) != 0 {
		t.Fatalf("Up skipping disabled B: checked = %d, want 0 (A)", checkedIndex(rbs))
	}
}
