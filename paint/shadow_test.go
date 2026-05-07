package paint

import (
	"testing"

	"silk/geom"
)

// recordingPainter is a minimal Painter mock that counts Fill calls
// and records the brush colors set. Enough to drive DrawShadowRect's
// layer count + alpha-falloff invariants without spinning up a real
// rasteriser.
type recordingPainter struct {
	fillCount int
	brushes   []Color
}

func (p *recordingPainter) Target() Surface                            { return nil }
func (p *recordingPainter) Save() int                                  { return 0 }
func (p *recordingPainter) Restore() int                               { return 0 }
func (p *recordingPainter) RestoreTo(int) bool                         { return true }
func (p *recordingPainter) CurrentState() int                          { return 0 }
func (p *recordingPainter) CurrentPoint() (float64, float64)           { return 0, 0 }
func (p *recordingPainter) Arc(float64, float64, float64, float64, float64)         {}
func (p *recordingPainter) ArcNegative(float64, float64, float64, float64, float64) {}
func (p *recordingPainter) CurveTo(float64, float64, float64, float64, float64, float64) {
}
func (p *recordingPainter) Line(float64, float64, float64, float64) {}
func (p *recordingPainter) LineTo(float64, float64)                {}
func (p *recordingPainter) MoveTo(float64, float64)                {}
func (p *recordingPainter) Rectangle(float64, float64, float64, float64) {}
func (p *recordingPainter) Rectangle1(geom.Rect)                          {}
func (p *recordingPainter) Stroke()                                       {}
func (p *recordingPainter) StrokePreserve()                               {}
func (p *recordingPainter) Fill()                                         { p.fillCount++ }
func (p *recordingPainter) FillPreserve()                                 {}
func (p *recordingPainter) Paint()                                        {}
func (p *recordingPainter) PaintWithAlpha(uint8)                          {}
func (p *recordingPainter) ResetClip()                                    {}
func (p *recordingPainter) Clip()                                         {}
func (p *recordingPainter) ClipPreserve()                                 {}
func (p *recordingPainter) ClipBounds() (float64, float64, float64, float64) {
	return 0, 0, 0, 0
}
func (p *recordingPainter) ClipBounds1() geom.Rect       { return geom.Rect{} }
func (p *recordingPainter) SetOperator(Operator)         {}
func (p *recordingPainter) ResetMatrix()                 {}
func (p *recordingPainter) Translate(float64, float64)   {}
func (p *recordingPainter) Scale(float64, float64)       {}
func (p *recordingPainter) Rotate(float64)               {}
func (p *recordingPainter) Transform(*geom.Mat3x2)       {}
func (p *recordingPainter) SetMatrix(*geom.Mat3x2)       {}
func (p *recordingPainter) GetMatrix(*geom.Mat3x2)       {}
func (p *recordingPainter) SetPen(Pen)                   {}
func (p *recordingPainter) SetPen1(Color, float64)       {}
func (p *recordingPainter) SetBrush(Brush)               {}
func (p *recordingPainter) SetBrush1(c Color)            { p.brushes = append(p.brushes, c) }
func (p *recordingPainter) SetFont(Font)                 {}
func (p *recordingPainter) Font() Font                   { return nil }
func (p *recordingPainter) ScaledFont() ScaledFont       { return nil }
func (p *recordingPainter) DrawText(string)              {}
func (p *recordingPainter) DrawText1(float64, float64, string) {}
func (p *recordingPainter) DrawGlyphs([]Glyph)           {}
func (p *recordingPainter) DrawGlyph(*Glyph)             {}
func (p *recordingPainter) DrawPixmap(Pixmap)            {}
func (p *recordingPainter) DrawPixmap1(float64, float64, Pixmap) {}
func (p *recordingPainter) DrawPixmap2(float64, float64, Pixmap, float64, float64) {
}
func (p *recordingPainter) DrawPixmap5(float64, float64, float64, float64, Pixmap) {}
func (p *recordingPainter) DrawIcon(Icon, float64, bool)                           {}
func (p *recordingPainter) DrawIcon1(Icon, float64, float64, float64, bool)        {}

// TestDrawShadowRectLayerCount: blur=4 should emit 5 fill layers
// (innermost outset 0 plus 4 outset rings). Ensures the algorithm's
// "N+1 layers" invariant matches the doc comment.
func TestDrawShadowRectLayerCount(t *testing.T) {
	rp := &recordingPainter{}
	DrawShadowRect(rp, 0, 0, 100, 50, 4, 4, Color{0, 0, 0, 200})
	if rp.fillCount != 5 {
		t.Errorf("blur=4 → %d fills, want 5", rp.fillCount)
	}
}

// TestDrawShadowRectAlphaFalloff: alphas should fall monotonically
// from inner to outer layer. Designers expect a soft outward
// gradient — a flat or non-monotonic alpha series would look like
// stacked outlines instead of a shadow.
func TestDrawShadowRectAlphaFalloff(t *testing.T) {
	rp := &recordingPainter{}
	DrawShadowRect(rp, 0, 0, 100, 50, 4, 4, Color{0, 0, 0, 250})
	if len(rp.brushes) < 2 {
		t.Fatalf("expected ≥2 brush sets, got %d", len(rp.brushes))
	}
	for i := 1; i < len(rp.brushes); i++ {
		if rp.brushes[i].A >= rp.brushes[i-1].A {
			t.Errorf("alpha not monotonic: layer %d alpha %d, layer %d alpha %d",
				i-1, rp.brushes[i-1].A, i, rp.brushes[i].A)
		}
	}
}

// TestDrawShadowRectDegenerateInputs: zero blur, zero size, and
// fully-transparent color all short-circuit without emitting any
// draw operations. Guards against the helper accidentally adding a
// "ghost" layer when called from a host with stale parameters.
func TestDrawShadowRectDegenerateInputs(t *testing.T) {
	cases := []struct {
		name                       string
		x, y, w, h, radius, blur float64
		color                      Color
	}{
		{"zero blur", 0, 0, 100, 50, 4, 0, Color{0, 0, 0, 200}},
		{"zero width", 0, 0, 0, 50, 4, 4, Color{0, 0, 0, 200}},
		{"zero height", 0, 0, 100, 0, 4, 4, Color{0, 0, 0, 200}},
		{"zero alpha", 0, 0, 100, 50, 4, 4, Color{0, 0, 0, 0}},
		{"negative blur", 0, 0, 100, 50, 4, -2, Color{0, 0, 0, 200}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rp := &recordingPainter{}
			DrawShadowRect(rp, c.x, c.y, c.w, c.h, c.radius, c.blur, c.color)
			if rp.fillCount != 0 {
				t.Errorf("expected no fills, got %d", rp.fillCount)
			}
		})
	}
}
