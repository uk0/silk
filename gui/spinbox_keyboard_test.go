package gui

import "testing"

// TestSpinStepped covers the pure stepping helper: increment, decrement, and
// clamping at both ends of the range.
func TestSpinStepped(t *testing.T) {
	cases := []struct {
		name       string
		cur, delta int
		min, max   int
		want       int
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
		if got := spinStepped(c.cur, c.delta, c.min, c.max); got != c.want {
			t.Errorf("%s: spinStepped(%d,%d,%d,%d)=%d want %d",
				c.name, c.cur, c.delta, c.min, c.max, got, c.want)
		}
	}
}

// TestSpinBoxPageStepDefault verifies PageStep defaults to 10*step and honours
// an explicit override.
func TestSpinBoxPageStepDefault(t *testing.T) {
	s := NewSpinBox()
	s.SetStep(2)
	if got := s.PageStep(); got != 20 {
		t.Errorf("default PageStep()=%d want 20", got)
	}
	s.SetPageStep(5)
	if got := s.PageStep(); got != 5 {
		t.Errorf("PageStep()=%d want 5", got)
	}
}

// TestSpinBoxArrowKeys drives the public API: Up/Down step by a single step.
func TestSpinBoxArrowKeys(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(0, 100)
	s.SetStep(5)
	s.SetValue(50)

	s.OnKeyDown(KeyUp, false)
	if s.Value() != 55 {
		t.Errorf("KeyUp: value=%d want 55", s.Value())
	}
	s.OnKeyDown(KeyDown, false)
	if s.Value() != 50 {
		t.Errorf("KeyDown: value=%d want 50", s.Value())
	}
}

// TestSpinBoxPageKeys checks PageUp/PageDown move by the larger page step.
func TestSpinBoxPageKeys(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(0, 100)
	s.SetStep(1)
	s.SetPageStep(20)
	s.SetValue(50)

	s.OnKeyDown(KeyPageUp, false)
	if s.Value() != 70 {
		t.Errorf("KeyPageUp: value=%d want 70", s.Value())
	}
	s.OnKeyDown(KeyPageDown, false)
	if s.Value() != 50 {
		t.Errorf("KeyPageDown: value=%d want 50", s.Value())
	}
}

// TestSpinBoxHomeEnd checks Home/End jump to min/max. The spin box always has a
// bounded range, so these keys are always applicable.
func TestSpinBoxHomeEnd(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(10, 90)
	s.SetValue(50)

	s.OnKeyDown(KeyHome, false)
	if s.Value() != 10 {
		t.Errorf("KeyHome: value=%d want 10", s.Value())
	}
	s.OnKeyDown(KeyEnd, false)
	if s.Value() != 90 {
		t.Errorf("KeyEnd: value=%d want 90", s.Value())
	}
}

// TestSpinBoxClampViaKeys verifies arrow/page steps clamp at the bounds.
func TestSpinBoxClampViaKeys(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(0, 100)
	s.SetStep(5)
	s.SetPageStep(20)

	s.SetValue(98)
	s.OnKeyDown(KeyUp, false)
	if s.Value() != 100 {
		t.Errorf("clamp up: value=%d want 100", s.Value())
	}
	s.SetValue(2)
	s.OnKeyDown(KeyDown, false)
	if s.Value() != 0 {
		t.Errorf("clamp down: value=%d want 0", s.Value())
	}
	s.SetValue(90)
	s.OnKeyDown(KeyPageUp, false)
	if s.Value() != 100 {
		t.Errorf("clamp page up: value=%d want 100", s.Value())
	}
}

// TestSpinBoxCallbackFiresOnChange asserts the change callback fires on a real
// change but not on a clamped no-op (Up while already at max).
func TestSpinBoxCallbackFiresOnChange(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(0, 100)
	s.SetStep(5)

	count := 0
	last := 0
	s.SetValueChangedCallback(func(_ interface{}, v int) {
		count++
		last = v
	})

	s.SetValue(50)
	if count != 1 || last != 50 {
		t.Fatalf("after SetValue(50): count=%d last=%d", count, last)
	}

	// Real change via key fires the callback.
	s.OnKeyDown(KeyUp, false)
	if count != 2 || last != 55 {
		t.Fatalf("after KeyUp: count=%d last=%d want count=2 last=55", count, last)
	}

	// Jump to max, then End is a no-op and must not fire.
	s.OnKeyDown(KeyEnd, false)
	if count != 3 || last != 100 {
		t.Fatalf("after KeyEnd: count=%d last=%d want count=3 last=100", count, last)
	}
	before := count
	s.OnKeyDown(KeyEnd, false)
	if count != before {
		t.Errorf("KeyEnd at max fired callback: count went %d -> %d", before, count)
	}

	// Up arrow at max is also a clamped no-op.
	s.OnKeyDown(KeyUp, false)
	if count != before {
		t.Errorf("KeyUp at max fired callback: count went %d -> %d", before, count)
	}
}
