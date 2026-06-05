package ged

import (
	"testing"

	"silk/gui"
)

// TestNudgeDelta checks the pure (key,shift)→(dx,dy) mapping for all four
// arrows, both with and without Shift, plus a non-arrow key. step=1,
// gridStep=10 mirror the values OnKeyDown feeds in.
func TestNudgeDelta(t *testing.T) {
	const step, gridStep = 1.0, 10.0

	cases := []struct {
		name        string
		key         int
		shift       bool
		wantDX      float64
		wantDY      float64
		wantHandled bool
	}{
		{"left", gui.KeyLeft, false, -1, 0, true},
		{"right", gui.KeyRight, false, 1, 0, true},
		{"up", gui.KeyUp, false, 0, -1, true},
		{"down", gui.KeyDown, false, 0, 1, true},

		{"shift-left", gui.KeyLeft, true, -10, 0, true},
		{"shift-right", gui.KeyRight, true, 10, 0, true},
		{"shift-up", gui.KeyUp, true, 0, -10, true},
		{"shift-down", gui.KeyDown, true, 0, 10, true},

		// A non-arrow key must report handled=false with a zero delta so the
		// caller falls through instead of consuming the keystroke.
		{"non-arrow", gui.KeyTab, false, 0, 0, false},
		{"non-arrow-shift", 'A', true, 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dx, dy, handled := nudgeDelta(tc.key, tc.shift, step, gridStep)
			if dx != tc.wantDX || dy != tc.wantDY || handled != tc.wantHandled {
				t.Errorf("nudgeDelta(%q) = (%g, %g, %v), want (%g, %g, %v)",
					tc.name, dx, dy, handled, tc.wantDX, tc.wantDY, tc.wantHandled)
			}
		})
	}
}

// TestNudgeViaOnKeyDownRight drives a real Right-arrow press through
// OnKeyDown and confirms the selected widget moves exactly 1 mm on X while
// Y is untouched. Uses the same harness as escape_test.go (no live window;
// IsKeyDown reports false, so this is the un-shifted fine-move path).
func TestNudgeViaOnKeyDownRight(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.OnKeyDown(gui.KeyRight, false)

	if x, y := fake.Pos(); x != 41 || y != 30 {
		t.Errorf("after Right: pos = (%g, %g), want (41, 30)", x, y)
	}
}

// TestNudgeShiftDownMovesByGridStep covers the coarse Shift+arrow jump. The
// test harness can't set the global Shift key state OnKeyDown reads, so the
// Shift modifier is injected into nudgeDelta directly and the resulting delta
// is applied through nudgeSelection — exactly the path OnKeyDown takes when
// Shift is held. The widget must move by nudgeGridStep (10 mm) on Y.
func TestNudgeShiftDownMovesByGridStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	// First a plain Right press: +1 mm on X (sanity that fine-move still works).
	view.OnKeyDown(gui.KeyRight, false)
	if x, _ := fake.Pos(); x != 41 {
		t.Fatalf("after Right: x = %g, want 41", x)
	}

	// Then a Shift+Down: +gridStep on Y, X unchanged.
	dx, dy, handled := nudgeDelta(gui.KeyDown, true, 1, nudgeGridStep)
	if !handled || dx != 0 || dy != nudgeGridStep {
		t.Fatalf("nudgeDelta(Down, shift) = (%g, %g, %v), want (0, %g, true)",
			dx, dy, handled, nudgeGridStep)
	}
	view.nudgeSelection(dx, dy)

	if x, y := fake.Pos(); x != 41 || y != 30+nudgeGridStep {
		t.Errorf("after Shift+Down: pos = (%g, %g), want (41, %g)", x, y, 30+nudgeGridStep)
	}
}
