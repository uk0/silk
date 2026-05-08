package paint

import (
	"image/color"
	"testing"
)

// TestDashedPenRendersGaps: stroke a horizontal line with a 4-on/4-off
// dash pattern across a 64-pixel-wide pixmap, then sample a row of
// pixels and confirm we see both painted and unpainted spans. A solid
// stroke (DashedPen ignored) would paint every pixel along the line —
// the test would fail because no fully-transparent pixel exists in the
// stroke row. Locks in the cairoPainter dash wiring against
// regressions where the cap/join refactor or applyPen reorder forgets
// to call cairo.SetDash.
func TestDashedPenRendersGaps(t *testing.T) {
	pix := NewPixmap(64, 16)
	g := pix.NewPainter()

	// Clear the surface to fully transparent so "unpainted" reads as 0
	// alpha. NewPixmap is already zeroed, but the explicit Paint with
	// a transparent brush makes the precondition obvious for readers.
	g.SetBrush1(Color{0, 0, 0, 0})
	g.Paint()

	pen := NewStyledPen(Color{255, 0, 0, 255}, 2,
		[]float64{4, 4}, 0,
		LineCapButt, LineJoinMiter)
	g.SetPen(pen)

	g.MoveTo(0, 8)
	g.LineTo(64, 8)
	g.Stroke()

	img, err := pix.Image()
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	// Walk the stroke row (y=8) and tally painted vs unpainted spans.
	// Stricter than "any zero alpha" — we expect at least 4 of each
	// at this 4-on/4-off rate over 64 px.
	var painted, unpainted int
	for x := 0; x < 64; x++ {
		_, _, _, a := color.RGBAModel.Convert(img.At(x, 8)).RGBA()
		if a > 0 {
			painted++
		} else {
			unpainted++
		}
	}
	if painted == 0 {
		t.Errorf("dashed stroke produced 0 painted pixels — line not drawn at all")
	}
	if unpainted == 0 {
		t.Errorf("dashed stroke produced 0 unpainted pixels — gaps not honoured (still solid?)")
	}
	if painted < 4 || unpainted < 4 {
		t.Errorf("dash counts look wrong: painted=%d unpainted=%d (each ≥4 expected)",
			painted, unpainted)
	}
}

// TestDashedPenResetWhenPenChanges: after a dashed pen has been
// applied, switching to a plain (non-DashedPen) pen and stroking
// must produce a SOLID line — the previous pen's dash state must
// not leak.
func TestDashedPenResetWhenPenChanges(t *testing.T) {
	pix := NewPixmap(64, 16)
	g := pix.NewPainter()
	g.SetBrush1(Color{0, 0, 0, 0})
	g.Paint()

	dashed := NewStyledPen(Color{255, 0, 0, 255}, 2,
		[]float64{4, 4}, 0,
		LineCapButt, LineJoinMiter)
	g.SetPen(dashed)
	g.MoveTo(0, 4)
	g.LineTo(64, 4)
	g.Stroke()

	plain := NewPen(Color{0, 0, 255, 255}, 2)
	g.SetPen(plain)
	g.MoveTo(0, 12)
	g.LineTo(64, 12)
	g.Stroke()

	img, err := pix.Image()
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	// y=12 is the plain stroke. Every column in [2..62] should be
	// painted (allow a 2-pixel margin for AA at the endpoints). If any
	// gap appears here, dash leaked from the dashed pen.
	for x := 2; x < 62; x++ {
		_, _, _, a := color.RGBAModel.Convert(img.At(x, 12)).RGBA()
		if a == 0 {
			t.Errorf("plain pen at x=%d had alpha 0 — dash leaked from dashed pen", x)
			break
		}
	}
}
