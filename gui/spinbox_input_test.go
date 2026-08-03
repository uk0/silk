package gui

import "testing"

// TestSpinBoxDisabledIgnoresMouse: OnKeyDown already refuses to step a disabled
// box; the up/down buttons and the wheel must refuse too. Widget-level
// enablement is not enforced by the event dispatcher — each widget checks it
// itself.
func TestSpinBoxDisabledIgnoresMouse(t *testing.T) {
	s := NewSpinBox()
	s.SetSize(80, 24)
	s.SetRange(0, 100)
	s.SetValue(10)
	s.SetEnabled(false)

	// Each case starts from 10 so an up-then-down pair cannot cancel out and
	// hide a live handler. SetValue is the programmatic path and stays live
	// while disabled — only user input is refused.
	cases := []struct {
		name string
		act  func()
	}{
		// Up / down buttons: right-hand buttonWidth() strip, upper / lower half.
		{"up-button click", func() { s.OnLeftDown(s.w-2, 2) }},
		{"down-button click", func() { s.OnLeftDown(s.w-2, s.h-2) }},
		{"wheel up", func() { s.OnMouseWheel(40, 12, 1) }},
		{"wheel down", func() { s.OnMouseWheel(40, 12, -1) }},
	}
	for _, c := range cases {
		s.SetValue(10)
		c.act()
		if s.Value() != 10 {
			t.Errorf("disabled %s: Value() = %d, want 10", c.name, s.Value())
		}
	}
}

// TestSpinBoxSizeHintsFitsWidestValue: the hint has to fit the widest value the
// box can ever display. Draw clips the text to the strip left of the buttons,
// so measuring only the maximum truncates a range whose minimum is the longer
// string ("-1000000" against "5").
func TestSpinBoxSizeHintsFitsWidestValue(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(-1000000, 5)

	h := s.SizeHints()
	s.SetSize(h.Width, h.Height)

	// Draw starts the text at x=4 and clips it at w-buttonWidth()-1.
	avail := h.Width - s.buttonWidth() - 1 - 4
	ext := Theme().Font.TextExtents("-1000000")
	if avail < ext.Width {
		t.Errorf("SizeHints().Width = %.3f leaves %.3f px for text, but the minimum "+
			"renders %.3f px wide", h.Width, avail, ext.Width)
	}
}
