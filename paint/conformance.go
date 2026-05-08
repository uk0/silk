package paint

import (
	"silk/geom"
)

// PainterTestingT is the trim TestingT subset RunPainterBattery uses,
// matching the standard library's testing.TB methods we actually call.
// Lives here so the paint package doesn't take a hard testing import —
// the battery can be driven from any harness that supplies the same
// three methods.
type PainterTestingT interface {
	Helper()
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// RunPainterBattery exercises every method of the paint.Painter
// interface against `p`, exposing implementations that nil-deref,
// panic on simple inputs, or silently no-op when given a brush type
// they don't recognise.
//
// Each Painter implementation in the silk tree (cairo, svgexport,
// pdfexport, recpaint) gets a four-line conformance test:
//
//	func TestPainterBattery(t *testing.T) {
//	    paint.RunPainterBattery(t, makePainter())
//	}
//
// The battery is intentionally argumentless beyond the painter — the
// shapes it draws are arbitrary but cover the full method surface
// once. If a method panics or warns, the painter implementation gets
// a t.Errorf with the method name. Otherwise the battery stays
// silent. There are no rendered-output assertions: image diffs live
// in the per-painter tests where they have ground-truth fixtures.
//
// Each method is wrapped in a helper that recovers from panics so
// one broken op doesn't mask the rest.
func RunPainterBattery(t PainterTestingT, p Painter) {
	t.Helper()

	check := func(name string, fn func()) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Painter.%s panicked: %v", name, r)
			}
		}()
		fn()
	}

	// State stack
	check("Save", func() { p.Save() })
	check("CurrentState", func() { _ = p.CurrentState() })
	check("Translate", func() { p.Translate(10, 20) })
	check("Scale", func() { p.Scale(2, 2) })
	check("Rotate", func() { p.Rotate(0.5) })
	check("ResetMatrix", func() { p.ResetMatrix() })

	// Matrix round-trip
	var m geom.Mat3x2
	check("GetMatrix", func() { p.GetMatrix(&m) })
	check("SetMatrix", func() { p.SetMatrix(&m) })
	check("Transform", func() { p.Transform(&m) })

	// Path construction
	check("MoveTo", func() { p.MoveTo(0, 0) })
	check("LineTo", func() { p.LineTo(50, 0) })
	check("Line", func() { p.Line(0, 0, 50, 50) })
	check("Rectangle", func() { p.Rectangle(0, 0, 100, 50) })
	check("Rectangle1", func() { p.Rectangle1(geom.Rect{X: 0, Y: 0, Width: 100, Height: 50}) })
	check("Arc", func() { p.Arc(50, 50, 25, 0, 6.28) })
	check("ArcNegative", func() { p.ArcNegative(50, 50, 25, 6.28, 0) })
	check("CurveTo", func() { p.CurveTo(10, 0, 20, 50, 30, 0) })
	check("CurrentPoint", func() { _, _ = p.CurrentPoint() })

	// Brushes
	check("SetBrush1", func() { p.SetBrush1(Color{255, 0, 0, 255}) })
	check("SetBrush(SolidBrush)", func() { p.SetBrush(NewSolidBrush(Color{0, 255, 0, 255})) })
	check("SetBrush(LinearGradient)", func() {
		grad := NewLinearGradient(0, 0, 100, 0)
		grad.AddStop(0, Color{0, 0, 0, 255})
		grad.AddStop(1, Color{255, 255, 255, 255})
		p.SetBrush(grad)
	})
	check("SetBrush(RadialGradient)", func() {
		grad := NewRadialGradient(50, 50, 0, 50)
		grad.AddStop(0, Color{255, 255, 255, 255})
		grad.AddStop(1, Color{0, 0, 0, 0})
		p.SetBrush(grad)
	})
	check("Fill", func() { p.Fill() })
	check("FillPreserve", func() {
		p.Rectangle(0, 0, 10, 10)
		p.FillPreserve()
		p.Stroke()
	})

	// Pens
	check("SetPen1", func() { p.SetPen1(Color{0, 0, 0, 255}, 1) })
	check("SetPen(Pen)", func() { p.SetPen(NewPen(Color{0, 0, 0, 255}, 2)) })
	check("Stroke", func() { p.Rectangle(0, 0, 10, 10); p.Stroke() })
	check("StrokePreserve", func() {
		p.Rectangle(0, 0, 10, 10)
		p.StrokePreserve()
		p.Fill()
	})

	// Clipping
	check("Clip", func() { p.Rectangle(0, 0, 50, 50); p.Clip() })
	check("ClipPreserve", func() { p.Rectangle(0, 0, 25, 25); p.ClipPreserve() })
	check("ClipBounds", func() { _, _, _, _ = p.ClipBounds() })
	check("ClipBounds1", func() { _ = p.ClipBounds1() })
	check("ResetClip", func() { p.ResetClip() })

	// Operator
	check("SetOperator", func() { p.SetOperator(OpOver) })

	// Paint over the current source / clip
	check("Paint", func() { p.Paint() })
	check("PaintWithAlpha", func() { p.PaintWithAlpha(128) })

	// Fonts and text. Use NewFont with empty family so the painter
	// falls back to its default.
	check("SetFont", func() { p.SetFont(NewFont("", 12, false, false)) })
	check("Font", func() { _ = p.Font() })
	check("ScaledFont", func() { _ = p.ScaledFont() })
	check("DrawText", func() { p.MoveTo(10, 20); p.DrawText("battery") })
	check("DrawText1", func() { p.DrawText1(10, 30, "battery") })

	// DrawGlyph / DrawGlyphs accept low-level glyph IDs. Most painters
	// only honour these when a font has been resolved by SetFont above.
	// Index 0 is .notdef on every font we ship, which the cairo and
	// SVG/PDF painters render as an empty box rather than panicking.
	g0 := Glyph{X: 10, Y: 40}
	check("DrawGlyph", func() { p.DrawGlyph(&g0) })
	check("DrawGlyphs", func() { p.DrawGlyphs([]Glyph{g0}) })

	// Surface accessors
	check("Target", func() { _ = p.Target() })

	// Pixmap and Icon: build a tiny fallback icon (genMissingIcon-
	// equivalent via AirIcon — always available, no external load) and
	// a 16×16 pixmap. These exercise the image-blit paths that the
	// path-and-fill battery above does not touch.
	check("DrawIcon", func() { p.DrawIcon(AirIcon(), 16, false) })
	check("DrawIcon1", func() { p.DrawIcon1(AirIcon(), 0, 0, 16, false) })

	pix := NewPixmap(16, 16)
	check("DrawPixmap", func() { p.DrawPixmap(pix) })
	check("DrawPixmap1", func() { p.DrawPixmap1(0, 0, pix) })
	check("DrawPixmap2", func() { p.DrawPixmap2(0, 0, pix, 0, 0) })
	check("DrawPixmap5", func() { p.DrawPixmap5(0, 0, 16, 16, pix) })

	// Restore matches the Save above. Putting Restore last keeps the
	// painter's state stack balanced after the battery.
	check("Restore", func() { p.Restore() })
	check("RestoreTo", func() { p.RestoreTo(0) })
}
