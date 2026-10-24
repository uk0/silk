package gui

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// editorNoopPainter is a default-everything paint.Painter used as the
// embedded base of test painters. We need it because paint.Painter
// has 60+ methods; the test-specific recording painter only overrides
// the small subset the SVG renderer actually calls. By embedding
// editorNoopPainter the test struct doesn't have to redeclare the full
// surface.
type editorNoopPainter struct{}

// Compile-time interface satisfaction.
var _ paint.Painter = (*editorNoopPainter)(nil)

func (editorNoopPainter) Target() paint.Surface                                     { return nil }
func (editorNoopPainter) Save() int                                                 { return 0 }
func (editorNoopPainter) Restore() int                                              { return 0 }
func (editorNoopPainter) RestoreTo(target int) bool                                 { return true }
func (editorNoopPainter) CurrentState() int                                         { return 0 }
func (editorNoopPainter) Translate(tx, ty float64)                                  {}
func (editorNoopPainter) Scale(sx, sy float64)                                      {}
func (editorNoopPainter) Rotate(rad float64)                                        {}
func (editorNoopPainter) ResetMatrix()                                              {}
func (editorNoopPainter) Transform(m *geom.Mat3x2)                                  {}
func (editorNoopPainter) SetMatrix(m *geom.Mat3x2)                                  {}
func (editorNoopPainter) GetMatrix(m *geom.Mat3x2)                                  {}
func (editorNoopPainter) MoveTo(x, y float64)                                       {}
func (editorNoopPainter) LineTo(x, y float64)                                       {}
func (editorNoopPainter) CurveTo(x1, y1, x2, y2, x, y float64)                      {}
func (editorNoopPainter) Arc(cx, cy, r, a0, a1 float64)                             {}
func (editorNoopPainter) ArcNegative(cx, cy, r, a0, a1 float64)                     {}
func (editorNoopPainter) Rectangle(x, y, w, h float64)                              {}
func (editorNoopPainter) Rectangle1(rc geom.Rect)                                   {}
func (editorNoopPainter) Line(x1, y1, x2, y2 float64)                               {}
func (editorNoopPainter) CurrentPoint() (float64, float64)                          { return 0, 0 }
func (editorNoopPainter) Fill()                                                     {}
func (editorNoopPainter) FillPreserve()                                             {}
func (editorNoopPainter) Stroke()                                                   {}
func (editorNoopPainter) StrokePreserve()                                           {}
func (editorNoopPainter) Paint()                                                    {}
func (editorNoopPainter) PaintWithAlpha(a uint8)                                    {}
func (editorNoopPainter) Clip()                                                     {}
func (editorNoopPainter) ClipPreserve()                                             {}
func (editorNoopPainter) ResetClip()                                                {}
func (editorNoopPainter) ClipBounds() (x, y, w, h float64)                          { return 0, 0, 0, 0 }
func (editorNoopPainter) ClipBounds1() geom.Rect                                    { return geom.Rect{} }
func (editorNoopPainter) SetOperator(op paint.Operator)                             {}
func (editorNoopPainter) SetPen(pen paint.Pen)                                      {}
func (editorNoopPainter) SetPen1(c paint.Color, w float64)                          {}
func (editorNoopPainter) SetBrush(b paint.Brush)                                    {}
func (editorNoopPainter) SetBrush1(c paint.Color)                                   {}
func (editorNoopPainter) SetFont(f paint.Font)                                      {}
func (editorNoopPainter) Font() paint.Font                                          { return nil }
func (editorNoopPainter) ScaledFont() paint.ScaledFont                              { return nil }
func (editorNoopPainter) DrawText(s string)                                         {}
func (editorNoopPainter) DrawText1(x, y float64, s string)                          {}
func (editorNoopPainter) DrawGlyphs(g []paint.Glyph)                                {}
func (editorNoopPainter) DrawGlyph(g *paint.Glyph)                                  {}
func (editorNoopPainter) DrawPixmap(pm paint.Pixmap)                                {}
func (editorNoopPainter) DrawPixmap1(x, y float64, pm paint.Pixmap)                 {}
func (editorNoopPainter) DrawPixmap2(x, y float64, pm paint.Pixmap, x0, y0 float64) {}
func (editorNoopPainter) DrawPixmap5(x, y, w, h float64, pm paint.Pixmap)           {}
func (editorNoopPainter) DrawIcon(ic paint.Icon, sz float64, grayed bool)           {}
func (editorNoopPainter) DrawIcon1(ic paint.Icon, x, y, sz float64, grayed bool)    {}
