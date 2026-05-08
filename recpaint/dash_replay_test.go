package recpaint

import (
	"image/color"
	"testing"

	"silk/paint"
)

// TestDashedPenRoundTripsThroughReplay: record a dashed-pen stroke
// through recpaint, replay onto a cairo painter, and confirm the
// rasterised pixmap shows actual gaps. Two cross-cutting guarantees
// land here at once:
//
//   - recpaint.SetPen captures the styled pen by reference, so the
//     DashedPen extension survives the closure.
//   - cairoPainter.applyPen reads DashedPen on every set, so a
//     replayed pen is treated identically to a direct one.
//
// Without either of those, this test would fail — we'd see a solid
// stroke (no gaps) in the pixmap row.
func TestDashedPenRoundTripsThroughReplay(t *testing.T) {
	rec := New()

	// Record: clear, set dashed pen, stroke a horizontal line.
	rec.SetBrush1(paint.Color{0, 0, 0, 0})
	rec.Paint()

	pen := paint.NewStyledPen(
		paint.Color{255, 0, 0, 255}, 2,
		[]float64{4, 4}, 0,
		paint.LineCapButt, paint.LineJoinMiter,
	)
	rec.SetPen(pen)
	rec.MoveTo(0, 8)
	rec.LineTo(64, 8)
	rec.Stroke()

	// Replay onto a fresh cairo pixmap painter.
	pix := paint.NewPixmap(64, 16)
	g := pix.NewPainter()
	rec.Replay(g)

	img, err := pix.Image()
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

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
		t.Errorf("replayed dashed stroke produced 0 painted pixels — line not drawn")
	}
	if unpainted == 0 {
		t.Errorf("replayed dashed stroke produced 0 unpainted pixels — gaps not honoured (recpaint stripped dash, or applyPen forgot it)")
	}
}
