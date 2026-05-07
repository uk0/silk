package paint

import "silk/geom"

// Round rounds x to the nearest integer for "snap-to-pixel" use.
// NOT a general-purpose rounder — undefined behaviour for NaN/Inf.
// Kept here because it's used by widget code and by the Cairo painter
// implementation alike, and was historically defined in this file.
func Round(x float64) float64 {
	if x < 0.0 {
		return float64(int(x - 0.5))
	} else if x > 0.0 {
		return float64(int(x + 0.5))
	}
	return 0
}

// Painter is the abstract drawing interface every backend implements.
//
// Two concrete impls live in this package, one per build tag:
//
//   - cairoPainter (default build, painter_cairo.go) — wraps cairo.Context
//   - nullPainter  (silk_no_cairo, painter_pure.go) — no-op stub
//
// silk_no_cairo callers expect to use silk/glui directly (through
// CairoCompat). nullPainter exists so paint.Pixmap.NewPainter can
// satisfy its contract without forcing every Pixmap to know about
// silk/glui (cycle-prevention).
type Painter interface {
	Target() Surface

	Save() int
	Restore() int
	RestoreTo(int) bool
	CurrentState() int

	CurrentPoint() (x, y float64)

	Arc(xc, yc, radius, angle1, angle2 float64)
	ArcNegative(xc, yc, radius, angle1, angle2 float64)
	CurveTo(x1, y1, x2, y2, x3, y3 float64)
	Line(x1, y1, x2, y2 float64)
	LineTo(x, y float64)
	MoveTo(x, y float64)
	Rectangle(x, y, width, height float64)
	Rectangle1(rect geom.Rect)

	Stroke()
	StrokePreserve()

	Fill()
	FillPreserve()

	Paint()
	PaintWithAlpha(alpha uint8)

	ResetClip()
	Clip()
	ClipPreserve()
	ClipBounds() (x, y, width, height float64)
	ClipBounds1() geom.Rect

	SetOperator(op Operator)

	ResetMatrix()
	Translate(tx, ty float64)
	Scale(sx, sy float64)
	Rotate(radians float64)
	Transform(m *geom.Mat3x2)
	SetMatrix(m *geom.Mat3x2)
	GetMatrix(m *geom.Mat3x2)

	SetPen(pen Pen)
	SetPen1(cr Color, width float64)

	SetBrush(br Brush)
	SetBrush1(cr Color)

	SetFont(f Font)
	Font() Font

	ScaledFont() ScaledFont

	DrawText(text string)
	DrawText1(x, y float64, text string)
	DrawGlyphs(glyphs []Glyph)
	DrawGlyph(glyph *Glyph)

	DrawPixmap(pixmap Pixmap)
	DrawPixmap1(x, y float64, pixmap Pixmap)
	DrawPixmap2(x, y float64, pixmap Pixmap, x0, y0 float64)
	DrawPixmap5(x, y, w, h float64, pixmap Pixmap)

	DrawIcon(ico Icon, fSize float64, grayed bool)
	DrawIcon1(ico Icon, x, y, fSize float64, grayed bool)
}

// ShadowPainter is implemented by painters that support GPU-accelerated
// box shadows. Widget code can type-assert any Painter to ShadowPainter
// to draw a soft drop shadow on backends that have a shader for it (the
// pure-OpenGL renderer in silk/glui), and gracefully degrade — by
// rendering the shape without a shadow — on backends that do not.
type ShadowPainter interface {
	FillBoxShadow(rc geom.Rect, radius, blur float64, col Color)
}
