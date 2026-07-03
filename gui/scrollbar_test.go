package gui

import "testing"

// TestScrollBarPartClassification locks the pure hit-test helper: a point above
// the thumb is beforeThumb, inside is onThumb, below is afterThumb, the end
// regions are the arrow buttons, and the thumb wins over an arrow it overlaps.
func TestScrollBarPartClassification(t *testing.T) {
	// track length 100, arrow buttons of 10 at each end, thumb [40,60).
	const (
		trackLen   = 100.0
		arrow      = 10.0
		thumbStart = 40.0
		thumbLen   = 20.0
	)
	cases := []struct {
		name string
		pos  float64
		want scrollPartKind
	}{
		{"leading arrow", 5, partArrowDec},
		{"trailing arrow", 95, partArrowInc},
		{"trough before thumb", 25, partBeforeThumb},
		{"thumb top edge (inclusive)", 40, partOnThumb},
		{"thumb middle", 50, partOnThumb},
		{"thumb bottom edge (exclusive)", 60, partAfterThumb},
		{"trough after thumb", 75, partAfterThumb},
	}
	for _, c := range cases {
		if got := scrollPart(c.pos, thumbStart, thumbLen, trackLen, arrow); got != c.want {
			t.Errorf("%s: scrollPart(%v) = %d, want %d", c.name, c.pos, got, c.want)
		}
	}
}

// TestScrollBarPartThumbBeatsArrow: a thumb that sits over the end-arrow region
// must still classify as onThumb (precedence matches the original PointToPart).
func TestScrollBarPartThumbBeatsArrow(t *testing.T) {
	// thumb [2,8) overlaps the leading 10-px arrow region.
	if got := scrollPart(5, 2, 6, 100, 10); got != partOnThumb {
		t.Errorf("thumb over arrow: got %d, want onThumb(%d)", got, partOnThumb)
	}
}

// TestScrollBarPartNoArrows: with arrowSize 0 the ends collapse into the trough,
// so a point near either end is just before/after the thumb, never an arrow.
func TestScrollBarPartNoArrows(t *testing.T) {
	if got := scrollPart(2, 40, 20, 100, 0); got != partBeforeThumb {
		t.Errorf("near-start no-arrow: got %d, want beforeThumb(%d)", got, partBeforeThumb)
	}
	if got := scrollPart(98, 40, 20, 100, 0); got != partAfterThumb {
		t.Errorf("near-end no-arrow: got %d, want afterThumb(%d)", got, partAfterThumb)
	}
}

// newTestScrollBar builds a vertical scrollbar with a known range/value/delta
// and a size that leaves clear trough above and below the thumb.
func newTestScrollBar() *ScrollBar {
	sb := NewScrollBar()
	sb.SetVertical(true)
	sb.SetSize(Theme().ScrollWidth, 300) // ScrollWidth wide, tall enough for visible trough
	sb.SetRange(0, 100) // min..max
	sb.SetDelta(2, 10)  // small (line) = 2, large (page) = 10
	sb.SetValue(50)     // park the thumb in the middle
	return sb
}

// troughClickY returns a y inside the before/after trough of the current thumb.
func troughClickY(sb *ScrollBar, before bool) float64 {
	_, ty, _, th := sb.TrackRect()
	if before {
		// midpoint between the rail top and the thumb top.
		return ty * 0.5
	}
	// midpoint between the thumb bottom and the rail bottom.
	return (ty + th + sb.h) * 0.5
}

// TestScrollBarTroughPagesBackward: a single click in the trough above the thumb
// pages the value down by one page step (large), via the same OnLeftDown path
// the widget uses for a real click.
func TestScrollBarTroughPagesBackward(t *testing.T) {
	sb := newTestScrollBar()
	_, large := sb.Delta()
	start := sb.Value()
	y := troughClickY(sb, true)

	// sanity: the click really lands before the thumb (part 2 == largeBakward).
	if part := sb.PointToPart(0, y); part != 2 {
		t.Fatalf("click y=%v classified as part %d, want 2 (before thumb)", y, part)
	}

	sb.OnLeftDown(0, y)
	sb.OnLeftUp(0, y) // stop the repeat timer started by OnLeftDown

	if got, want := sb.Value(), start-large; got != want {
		t.Errorf("trough-before click: value = %v, want %v (start %v - page %v)", got, want, start, large)
	}
}

// TestScrollBarTroughPagesForward: a click in the trough below the thumb pages the
// value up by one page step.
func TestScrollBarTroughPagesForward(t *testing.T) {
	sb := newTestScrollBar()
	_, large := sb.Delta()
	start := sb.Value()
	y := troughClickY(sb, false)

	if part := sb.PointToPart(0, y); part != 4 {
		t.Fatalf("click y=%v classified as part %d, want 4 (after thumb)", y, part)
	}

	sb.OnLeftDown(0, y)
	sb.OnLeftUp(0, y)

	if got, want := sb.Value(), start+large; got != want {
		t.Errorf("trough-after click: value = %v, want %v (start %v + page %v)", got, want, start, large)
	}
}

// TestScrollBarTroughClampsAtMin: paging backward from near the bottom of the range
// never drops below min.
func TestScrollBarTroughClampsAtMin(t *testing.T) {
	sb := newTestScrollBar()
	min, _ := sb.Range()
	_, large := sb.Delta()
	sb.SetValue(min + large*0.5) // less than one page above the floor
	y := troughClickY(sb, true)

	sb.OnLeftDown(0, y)
	sb.OnLeftUp(0, y)

	if got := sb.Value(); got != min {
		t.Errorf("paging backward past min: value = %v, want %v", got, min)
	}
}

// TestScrollBarTroughClampsAtMax: paging forward from near the top never exceeds max.
func TestScrollBarTroughClampsAtMax(t *testing.T) {
	sb := newTestScrollBar()
	max := func() float64 { _, m := sb.Range(); return m }()
	_, large := sb.Delta()
	sb.SetValue(max - large*0.5)
	y := troughClickY(sb, false)

	sb.OnLeftDown(0, y)
	sb.OnLeftUp(0, y)

	if got := sb.Value(); got != max {
		t.Errorf("paging forward past max: value = %v, want %v", got, max)
	}
}

// TestScrollBarEndClicksPage: the modern bar draws no end-arrow buttons (the
// arrow hit zone is 0), so a click at either rail end lands in the trough and
// pages by one large step instead of line-stepping.
func TestScrollBarEndClicksPage(t *testing.T) {
	sb := newTestScrollBar()
	_, large := sb.Delta()
	start := sb.Value()

	// top end: y near 0 is trough-before-thumb (part 2), never an arrow.
	if part := sb.PointToPart(0, 2); part != 2 {
		t.Fatalf("top-end y classified as part %d, want 2 (before thumb)", part)
	}
	sb.OnLeftDown(0, 2)
	sb.OnLeftUp(0, 2)
	if got, want := sb.Value(), start-large; got != want {
		t.Errorf("top-end click: value = %v, want %v", got, want)
	}

	// bottom end: y near h is trough-after-thumb (part 4), never an arrow.
	start = sb.Value()
	by := sb.h - 2
	if part := sb.PointToPart(0, by); part != 4 {
		t.Fatalf("bottom-end y classified as part %d, want 4 (after thumb)", part)
	}
	sb.OnLeftDown(0, by)
	sb.OnLeftUp(0, by)
	if got, want := sb.Value(), start+large; got != want {
		t.Errorf("bottom-end click: value = %v, want %v", got, want)
	}
}
