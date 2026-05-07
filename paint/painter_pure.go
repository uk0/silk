//go:build silk_no_cairo

package paint

import "silk/geom"

// nullPainter is a no-op Painter satisfying the Painter interface
// without any drawing backend. silk/glui's CairoCompat is the actual
// Painter used by widgets in pure-Go mode; this stub exists so
// imagePixmap.NewPainter() can return SOMETHING valid (the Pixmap
// interface contract requires a Painter).
//
// Calls into a nullPainter discard the input. State methods (Save /
// Restore / RestoreTo) maintain a tiny depth counter so widget code
// that asserts balanced save/restore doesn't trip.
type nullPainter struct {
	depth int
}

// newNullPainter returns a fresh nullPainter. Lowercase factory so
// only paint-internal code (imagePixmap.NewPainter) constructs them.
func newNullPainter() Painter { return &nullPainter{} }

// --- Surface / state ---

func (p *nullPainter) Target() Surface     { return nil }
func (p *nullPainter) Save() int           { p.depth++; return p.depth }
func (p *nullPainter) Restore() int {
	if p.depth > 0 {
		p.depth--
	}
	return p.depth
}
func (p *nullPainter) RestoreTo(target int) bool {
	if target < 0 || target > p.depth {
		return false
	}
	p.depth = target
	return true
}
func (p *nullPainter) CurrentState() int { return p.depth }

// --- Path queries ---

func (p *nullPainter) CurrentPoint() (x, y float64) { return 0, 0 }

// --- Path construction (no-op) ---

func (*nullPainter) Arc(cx, cy, r, a0, a1 float64)         {}
func (*nullPainter) ArcNegative(cx, cy, r, a0, a1 float64) {}
func (*nullPainter) CurveTo(x1, y1, x2, y2, x, y float64)  {}
func (*nullPainter) Line(x1, y1, x2, y2 float64)           {}
func (*nullPainter) LineTo(x, y float64)                   {}
func (*nullPainter) MoveTo(x, y float64)                   {}
func (*nullPainter) Rectangle(x, y, w, h float64)          {}
func (*nullPainter) Rectangle1(rc geom.Rect)               {}

// --- Stroke / fill ---

func (*nullPainter) Stroke()              {}
func (*nullPainter) StrokePreserve()      {}
func (*nullPainter) Fill()                {}
func (*nullPainter) FillPreserve()        {}
func (*nullPainter) Paint()               {}
func (*nullPainter) PaintWithAlpha(a uint8) {}

// --- Clip ---

func (*nullPainter) ResetClip()                       {}
func (*nullPainter) Clip()                            {}
func (*nullPainter) ClipPreserve()                    {}
func (*nullPainter) ClipBounds() (x, y, w, h float64) { return 0, 0, 0, 0 }
func (*nullPainter) ClipBounds1() geom.Rect           { return geom.Rect{} }

// --- Operator ---

func (*nullPainter) SetOperator(op Operator) {}

// --- Transform ---

func (*nullPainter) ResetMatrix()             {}
func (*nullPainter) Translate(tx, ty float64) {}
func (*nullPainter) Scale(sx, sy float64)     {}
func (*nullPainter) Rotate(rad float64)       {}
func (*nullPainter) Transform(m *geom.Mat3x2) {}
func (*nullPainter) SetMatrix(m *geom.Mat3x2) {}
func (*nullPainter) GetMatrix(m *geom.Mat3x2) {}

// --- Pen / brush / font ---

func (*nullPainter) SetPen(pen Pen)               {}
func (*nullPainter) SetPen1(c Color, w float64)   {}
func (*nullPainter) SetBrush(b Brush)             {}
func (*nullPainter) SetBrush1(c Color)            {}
func (*nullPainter) SetFont(f Font)               {}
func (*nullPainter) Font() Font                   { return nil }
func (*nullPainter) ScaledFont() ScaledFont       { return nil }

// --- Text / glyphs ---

func (*nullPainter) DrawText(text string)               {}
func (*nullPainter) DrawText1(x, y float64, text string) {}
func (*nullPainter) DrawGlyphs(glyphs []Glyph)          {}
func (*nullPainter) DrawGlyph(g *Glyph)                 {}

// --- Pixmap / icon ---

func (*nullPainter) DrawPixmap(pm Pixmap)                              {}
func (*nullPainter) DrawPixmap1(x, y float64, pm Pixmap)               {}
func (*nullPainter) DrawPixmap2(x, y float64, pm Pixmap, x0, y0 float64) {}
func (*nullPainter) DrawPixmap5(x, y, w, h float64, pm Pixmap)         {}
func (*nullPainter) DrawIcon(ic Icon, sz float64, grayed bool)         {}
func (*nullPainter) DrawIcon1(ic Icon, x, y, sz float64, grayed bool)  {}
