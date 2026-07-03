package svg

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// noopPainter is a default-everything paint.Painter used as the
// embedded base of test painters. We need it because paint.Painter
// has 60+ methods; the test-specific recording painter only overrides
// the small subset the SVG renderer actually calls. By embedding
// noopPainter the test struct doesn't have to redeclare the full
// surface.
type noopPainter struct{}

// Compile-time interface satisfaction.
var _ paint.Painter = (*noopPainter)(nil)

func (noopPainter) Target() paint.Surface                                     { return nil }
func (noopPainter) Save() int                                                 { return 0 }
func (noopPainter) Restore() int                                              { return 0 }
func (noopPainter) RestoreTo(target int) bool                                 { return true }
func (noopPainter) CurrentState() int                                         { return 0 }
func (noopPainter) Translate(tx, ty float64)                                  {}
func (noopPainter) Scale(sx, sy float64)                                      {}
func (noopPainter) Rotate(rad float64)                                        {}
func (noopPainter) ResetMatrix()                                              {}
func (noopPainter) Transform(m *geom.Mat3x2)                                  {}
func (noopPainter) SetMatrix(m *geom.Mat3x2)                                  {}
func (noopPainter) GetMatrix(m *geom.Mat3x2)                                  {}
func (noopPainter) MoveTo(x, y float64)                                       {}
func (noopPainter) LineTo(x, y float64)                                       {}
func (noopPainter) CurveTo(x1, y1, x2, y2, x, y float64)                      {}
func (noopPainter) Arc(cx, cy, r, a0, a1 float64)                             {}
func (noopPainter) ArcNegative(cx, cy, r, a0, a1 float64)                     {}
func (noopPainter) Rectangle(x, y, w, h float64)                              {}
func (noopPainter) Rectangle1(rc geom.Rect)                                   {}
func (noopPainter) Line(x1, y1, x2, y2 float64)                               {}
func (noopPainter) CurrentPoint() (float64, float64)                          { return 0, 0 }
func (noopPainter) Fill()                                                     {}
func (noopPainter) FillPreserve()                                             {}
func (noopPainter) Stroke()                                                   {}
func (noopPainter) StrokePreserve()                                           {}
func (noopPainter) Paint()                                                    {}
func (noopPainter) PaintWithAlpha(a uint8)                                    {}
func (noopPainter) Clip()                                                     {}
func (noopPainter) ClipPreserve()                                             {}
func (noopPainter) ResetClip()                                                {}
func (noopPainter) ClipBounds() (x, y, w, h float64)                          { return 0, 0, 0, 0 }
func (noopPainter) ClipBounds1() geom.Rect                                    { return geom.Rect{} }
func (noopPainter) SetOperator(op paint.Operator)                             {}
func (noopPainter) SetPen(pen paint.Pen)                                      {}
func (noopPainter) SetPen1(c paint.Color, w float64)                          {}
func (noopPainter) SetBrush(b paint.Brush)                                    {}
func (noopPainter) SetBrush1(c paint.Color)                                   {}
func (noopPainter) SetFont(f paint.Font)                                      {}
func (noopPainter) Font() paint.Font                                          { return nil }
func (noopPainter) ScaledFont() paint.ScaledFont                              { return nil }
func (noopPainter) DrawText(s string)                                         {}
func (noopPainter) DrawText1(x, y float64, s string)                          {}
func (noopPainter) DrawGlyphs(g []paint.Glyph)                                {}
func (noopPainter) DrawGlyph(g *paint.Glyph)                                  {}
func (noopPainter) DrawPixmap(pm paint.Pixmap)                                {}
func (noopPainter) DrawPixmap1(x, y float64, pm paint.Pixmap)                 {}
func (noopPainter) DrawPixmap2(x, y float64, pm paint.Pixmap, x0, y0 float64) {}
func (noopPainter) DrawPixmap5(x, y, w, h float64, pm paint.Pixmap)           {}
func (noopPainter) DrawIcon(ic paint.Icon, sz float64, grayed bool)           {}
func (noopPainter) DrawIcon1(ic paint.Icon, x, y, sz float64, grayed bool)    {}
