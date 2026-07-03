package gui

import "testing"

// TestSteppedValue covers the pure stepping helper: increment, decrement,
// and clamping at both ends of the range.
func TestSteppedValue(t *testing.T) {
	cases := []struct {
		name       string
		cur, delta float64
		min, max   float64
		want       float64
	}{
		{"increment", 50, 10, 0, 100, 60},
		{"decrement", 50, -10, 0, 100, 40},
		{"clamp at max", 95, 10, 0, 100, 100},
		{"clamp at min", 5, -10, 0, 100, 0},
		{"already at max", 100, 10, 0, 100, 100},
		{"already at min", 0, -10, 0, 100, 0},
		{"negative range", 0, -1, -50, 50, -1},
	}
	for _, c := range cases {
		if got := steppedValue(c.cur, c.delta, c.min, c.max); got != c.want {
			t.Errorf("%s: steppedValue(%v,%v,%v,%v)=%v want %v",
				c.name, c.cur, c.delta, c.min, c.max, got, c.want)
		}
	}
}

// TestSliderStepDefaults verifies the auto step/page-step derivation.
func TestSliderStepDefaults(t *testing.T) {
	s := NewSlider(0, 100)
	if got := s.Step(); got != 1 {
		t.Errorf("default Step()=%v want 1", got)
	}
	if got := s.PageStep(); got != 10 {
		t.Errorf("default PageStep()=%v want 10", got)
	}
	s.SetStep(5)
	s.SetPageStep(25)
	if got := s.Step(); got != 5 {
		t.Errorf("Step()=%v want 5", got)
	}
	if got := s.PageStep(); got != 25 {
		t.Errorf("PageStep()=%v want 25", got)
	}
}

// TestSliderArrowKeys drives the public API: arrow keys move by a single step
// in both directions for horizontal and vertical orientations.
func TestSliderArrowKeys(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetStep(5)
	s.SetValue(50)

	s.OnKeyDown(KeyRight, false)
	if s.Value() != 55 {
		t.Errorf("KeyRight: value=%v want 55", s.Value())
	}
	s.OnKeyDown(KeyUp, false)
	if s.Value() != 60 {
		t.Errorf("KeyUp: value=%v want 60", s.Value())
	}
	s.OnKeyDown(KeyLeft, false)
	if s.Value() != 55 {
		t.Errorf("KeyLeft: value=%v want 55", s.Value())
	}
	s.OnKeyDown(KeyDown, false)
	if s.Value() != 50 {
		t.Errorf("KeyDown: value=%v want 50", s.Value())
	}

	// Vertical sliders use the same mapping (Up increases, matching Qt).
	s.SetVertical(true)
	s.SetValue(50)
	s.OnKeyDown(KeyUp, false)
	if s.Value() != 55 {
		t.Errorf("vertical KeyUp: value=%v want 55", s.Value())
	}
	s.OnKeyDown(KeyDown, false)
	if s.Value() != 50 {
		t.Errorf("vertical KeyDown: value=%v want 50", s.Value())
	}
}

// TestSliderPageKeys checks PageUp/PageDown move by the larger page step.
func TestSliderPageKeys(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetPageStep(20)
	s.SetValue(50)

	s.OnKeyDown(KeyPageUp, false)
	if s.Value() != 70 {
		t.Errorf("KeyPageUp: value=%v want 70", s.Value())
	}
	s.OnKeyDown(KeyPageDown, false)
	if s.Value() != 50 {
		t.Errorf("KeyPageDown: value=%v want 50", s.Value())
	}
}

// TestSliderHomeEnd checks Home/End jump to min/max.
func TestSliderHomeEnd(t *testing.T) {
	s := NewSlider(10, 90)
	s.SetValue(50)

	s.OnKeyDown(KeyHome, false)
	if s.Value() != 10 {
		t.Errorf("KeyHome: value=%v want 10", s.Value())
	}
	s.OnKeyDown(KeyEnd, false)
	if s.Value() != 90 {
		t.Errorf("KeyEnd: value=%v want 90", s.Value())
	}
}

// TestSliderClampViaKeys verifies arrow/page steps clamp at the bounds.
func TestSliderClampViaKeys(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetStep(5)
	s.SetPageStep(20)

	s.SetValue(98)
	s.OnKeyDown(KeyRight, false)
	if s.Value() != 100 {
		t.Errorf("clamp up: value=%v want 100", s.Value())
	}
	s.SetValue(2)
	s.OnKeyDown(KeyLeft, false)
	if s.Value() != 0 {
		t.Errorf("clamp down: value=%v want 0", s.Value())
	}
	s.SetValue(90)
	s.OnKeyDown(KeyPageUp, false)
	if s.Value() != 100 {
		t.Errorf("clamp page up: value=%v want 100", s.Value())
	}
}

// TestSliderCallbackFiresOnChange asserts the change callback fires on a real
// change but not on a clamped no-op (End while already at max).
func TestSliderCallbackFiresOnChange(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetStep(5)

	count := 0
	var last float64
	s.SetValueChangedCallback(func(_ interface{}, v float64) {
		count++
		last = v
	})

	s.SetValue(50)
	if count != 1 || last != 50 {
		t.Fatalf("after SetValue(50): count=%d last=%v", count, last)
	}

	// Real change via key fires the callback.
	s.OnKeyDown(KeyRight, false)
	if count != 2 || last != 55 {
		t.Fatalf("after KeyRight: count=%d last=%v want count=2 last=55", count, last)
	}

	// Jump to max, then End is a no-op and must not fire.
	s.OnKeyDown(KeyEnd, false)
	if count != 3 || last != 100 {
		t.Fatalf("after KeyEnd: count=%d last=%v want count=3 last=100", count, last)
	}
	before := count
	s.OnKeyDown(KeyEnd, false)
	if count != before {
		t.Errorf("KeyEnd at max fired callback: count went %d -> %d", before, count)
	}

	// Right arrow at max is also a clamped no-op.
	s.OnKeyDown(KeyRight, false)
	if count != before {
		t.Errorf("KeyRight at max fired callback: count went %d -> %d", before, count)
	}
}
