package gui

import "testing"

// TestEditDisabledIgnoresInput: SetEnabled(false) has to stop the field taking
// keystrokes, not merely change how it paints. Widget-level enablement is not
// enforced by the event dispatcher — every widget checks it itself (see
// Button.OnKeyDown, Slider.OnLeftDown), and Edit checked only its read-only
// flag, so a greyed-out field still edited.
func TestEditDisabledIgnoresInput(t *testing.T) {
	e := NewEdit()
	e.SetSize(200, 24)
	e.SetEnabled(false)

	// Each case restarts from the same buffer so an insert followed by a
	// delete cannot cancel out and hide a live handler.
	cases := []struct {
		name string
		act  func()
	}{
		{"typed character", func() { e.SetSelection(4, 4); e.OnTextInput("x") }},
		{"Backspace", func() { e.SetSelection(4, 4); e.OnKeyDown(KeyBackSpace, false) }},
		{"Delete over a selection", func() { e.SetSelection(0, 2); e.OnKeyDown(KeyDelete, false) }},
		{"Enter in a multi-line edit", func() {
			e.SetMultiLine(true)
			e.SetSelection(4, 4)
			e.OnKeyDown(KeyEnter, false)
			e.SetMultiLine(false)
		}},
	}
	for _, c := range cases {
		e.SetText("keep")
		c.act()
		if got := e.Text(); got != "keep" {
			t.Errorf("disabled %s: Text() = %q, want %q", c.name, got, "keep")
		}
	}

	// A click must not arm the drag-selection state machine either.
	e.OnLeftDown(10, 10)
	if e.mouseDown {
		t.Error("disabled click armed the drag-selection state")
	}
}

// TestEditEnabledStillEdits guards the gate itself: the default (enabled) path
// must be untouched.
func TestEditEnabledStillEdits(t *testing.T) {
	e := NewEdit()
	e.SetSize(200, 24)
	e.SetText("keep")
	e.SetSelection(4, 4)
	e.OnTextInput("x")
	if got := e.Text(); got != "keepx" {
		t.Errorf("enabled OnTextInput: Text() = %q, want %q", got, "keepx")
	}
}
