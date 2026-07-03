package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// TestWrapField locks the pure stepping helper: a +1 off the top of the range
// rolls to 0, a -1 below 0 rolls to the max, and a step inside the range is a
// plain add. The ranges used are the two real field maxima (23 for hours, 59
// for minutes/seconds).
func TestWrapField(t *testing.T) {
	cases := []struct {
		val, delta, max, want int
	}{
		{59, 1, 59, 0},   // minute 59 +1 → 00
		{0, -1, 59, 59},  // minute 00 -1 → 59
		{23, 1, 23, 0},   // hour 23 +1 → 00
		{0, -1, 23, 23},  // hour 00 -1 → 23
		{30, 1, 59, 31},  // in-range step up
		{30, -1, 59, 29}, // in-range step down
		{12, 0, 23, 12},  // no-op delta
	}
	for _, c := range cases {
		if got := wrapField(c.val, c.delta, c.max); got != c.want {
			t.Errorf("wrapField(%d, %d, %d) = %d, want %d",
				c.val, c.delta, c.max, got, c.want)
		}
	}
}

// TestSetTimeNormalises asserts the documented SetTime rule: each argument is
// folded into its field range by euclidean modulo (no carry between fields),
// so 25:70:80 becomes 01:10:20 and negatives wrap up from the top.
func TestSetTimeNormalises(t *testing.T) {
	tp := NewTimePicker()

	tp.SetTime(25, 70, 80)
	if tp.Hour() != 1 || tp.Minute() != 10 || tp.Second() != 20 {
		t.Errorf("SetTime(25,70,80) = %02d:%02d:%02d, want 01:10:20",
			tp.Hour(), tp.Minute(), tp.Second())
	}

	tp.SetTime(-1, -1, -1)
	if tp.Hour() != 23 || tp.Minute() != 59 || tp.Second() != 59 {
		t.Errorf("SetTime(-1,-1,-1) = %02d:%02d:%02d, want 23:59:59",
			tp.Hour(), tp.Minute(), tp.Second())
	}

	tp.SetTime(9, 5, 0)
	if tp.Hour() != 9 || tp.Minute() != 5 || tp.Second() != 0 {
		t.Errorf("SetTime(9,5,0) = %02d:%02d:%02d, want 09:05:00",
			tp.Hour(), tp.Minute(), tp.Second())
	}
}

// TestSigTimeChangedFiresOnRealChange covers the callback contract: a SetTime
// that lands on a new value fires once with the new fields, while a SetTime to
// the already-current value is a silent no-op.
func TestSigTimeChangedFiresOnRealChange(t *testing.T) {
	tp := NewTimePicker()
	tp.SetTime(10, 30, 0)

	count := 0
	var gh, gm, gs int
	tp.SigTimeChanged(func(h, m, s int) {
		count++
		gh, gm, gs = h, m, s
	})

	// No-op: same value → must not fire.
	tp.SetTime(10, 30, 0)
	if count != 0 {
		t.Fatalf("SetTime to the same value fired %d times, want 0", count)
	}

	// Real change → fires once with the new value.
	tp.SetTime(11, 30, 0)
	if count != 1 {
		t.Fatalf("SetTime to a new value fired %d times, want 1", count)
	}
	if gh != 11 || gm != 30 || gs != 0 {
		t.Errorf("callback got %02d:%02d:%02d, want 11:30:00", gh, gm, gs)
	}
}

// TestOnKeyDownStepsFocusedField drives the keyboard path that OnKeyDown
// delegates to: Up/Down step the focused field with wraparound and Left/Right
// move focus. We start on the hour field, walk to minute, and check the 59→00
// wrap leaves the hour untouched (no carry, per the documented rule).
func TestOnKeyDownStepsFocusedField(t *testing.T) {
	tp := NewTimePicker()
	tp.SetTime(10, 59, 0)

	count := 0
	tp.SigTimeChanged(func(h, m, s int) { count++ })

	// Focus starts on the hour. Up steps the hour.
	tp.OnKeyDown(KeyUp, false)
	if tp.Hour() != 11 || tp.Minute() != 59 {
		t.Fatalf("Up on hour = %02d:%02d, want 11:59", tp.Hour(), tp.Minute())
	}

	// Right moves focus to the minute field; Up wraps 59→00 with NO carry
	// into the hour.
	tp.OnKeyDown(KeyRight, false)
	tp.OnKeyDown(KeyUp, false)
	if tp.Minute() != 0 {
		t.Errorf("minute wrap = %02d, want 00", tp.Minute())
	}
	if tp.Hour() != 11 {
		t.Errorf("minute wrap carried into hour: hour = %02d, want 11 (no carry)", tp.Hour())
	}

	// Down on the minute wraps 00→59.
	tp.OnKeyDown(KeyDown, false)
	if tp.Minute() != 59 {
		t.Errorf("minute Down wrap = %02d, want 59", tp.Minute())
	}

	if count == 0 {
		t.Errorf("key steps should have fired SigTimeChanged")
	}
}

// TestSetShowSecondsFocusFallback verifies hiding the seconds field while it
// is focused drops focus back to the minute, and that fieldCount tracks the
// flag. The stored second value is retained.
func TestSetShowSecondsFocusFallback(t *testing.T) {
	tp := NewTimePicker()
	tp.SetShowSeconds(true)
	tp.SetTime(8, 15, 42)
	if tp.fieldCount() != 3 {
		t.Fatalf("fieldCount with seconds = %d, want 3", tp.fieldCount())
	}

	// Focus the seconds field via Right×2, then hide seconds.
	tp.OnKeyDown(KeyRight, false)
	tp.OnKeyDown(KeyRight, false)
	if tp.focusField != 2 {
		t.Fatalf("focusField after Right×2 = %d, want 2", tp.focusField)
	}

	tp.SetShowSeconds(false)
	if tp.focusField != 1 {
		t.Errorf("focusField after hiding seconds = %d, want 1 (fallback to minute)", tp.focusField)
	}
	if tp.fieldCount() != 2 {
		t.Errorf("fieldCount after hide = %d, want 2", tp.fieldCount())
	}
	if tp.Second() != 42 {
		t.Errorf("hiding seconds dropped the value: Second() = %d, want 42", tp.Second())
	}
}

// TestTimePickerSizeHints documents the footprint: two fields (HH:MM) by
// default, three when seconds show. Height is the fixed 32.
func TestTimePickerSizeHints(t *testing.T) {
	tp := NewTimePicker()
	h := tp.SizeHints()
	want2 := timePickerPad*2 + 2*timeFieldW + 1*timeSepW
	if h.Width != want2 || h.Height != 32 {
		t.Errorf("HH:MM SizeHints = %v×%v, want %v×32", h.Width, h.Height, want2)
	}

	tp.SetShowSeconds(true)
	h = tp.SizeHints()
	want3 := timePickerPad*2 + 3*timeFieldW + 2*timeSepW
	if h.Width != want3 {
		t.Errorf("HH:MM:SS SizeHints width = %v, want %v", h.Width, want3)
	}
}

// TestTimePickerDrawNoPanic is a smoke test: drawing in both 2-field and
// 3-field modes, focused and hovered, must not panic. We drive a nil-safe
// painter shim (real theme font supplies TextExtents) since there is no GL
// surface in a unit test — same pattern as calendar_test.go.
func TestTimePickerDrawNoPanic(t *testing.T) {
	tp := NewTimePicker()
	tp.SetSize(120, 32)
	tp.SetFocus()
	tp.SetTime(13, 45, 30)

	rec := timeNopPainter{}
	tp.Draw(rec) // HH:MM, hour focused

	tp.SetShowSeconds(true)
	tp.SetSize(160, 32)
	tp.hoverField = 1 // simulate a hover wash on the minute stepper
	tp.hoverUp = true
	tp.Draw(rec) // HH:MM:SS

	tp.hoverField = -1
	tp.Draw(rec) // no hover
}

// timeNopPainter satisfies paint.Painter with no-op stubs (embeds a nil
// Painter) so Draw can run without a render target. Only the methods
// TimePicker.Draw actually calls need behaviour; the embedded nil supplies the
// rest, which Draw never reaches. Models the calendar_test nopPainter pattern.
type timeNopPainter struct{ paint.Painter }

func (timeNopPainter) MoveTo(x, y float64)                  {}
func (timeNopPainter) LineTo(x, y float64)                  {}
func (timeNopPainter) Rectangle(x, y, w, h float64)         {}
func (timeNopPainter) Fill()                                {}
func (timeNopPainter) Stroke()                              {}
func (timeNopPainter) SetBrush1(c paint.Color)              {}
func (timeNopPainter) SetPen1(c paint.Color, width float64) {}
func (timeNopPainter) SetFont(f paint.Font)                 {}
func (timeNopPainter) DrawText1(x, y float64, text string)  {}
