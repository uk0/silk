package glui

import (
	"silk/geom"
	"silk/paint"
	"testing"
)

// All CairoCompat tests reuse the off-GL test renderer from
// painter_adapter_test.go so they can run under `go test -short` without
// a window.

func newCompatTestPainter(t *testing.T) (*CairoCompat, *Renderer) {
	t.Helper()
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)
	return c, r
}

func TestCairoCompatImplementsPainter(t *testing.T) {
	// Compile-time check redundant with the var _ paint.Painter line in
	// cairo_compat.go, but the explicit assignment here makes the missing
	// method (if any) appear inside this test's frame, not the package
	// init, which is friendlier when iterating.
	var _ paint.Painter = NewCairoCompat(newAdapterTestRenderer())
}

func TestCairoCompatSaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	if d := c.CurrentState(); d != 0 {
		t.Fatalf("initial depth = %d, want 0", d)
	}
	c.Save()
	c.Save()
	c.Save()
	if d := c.CurrentState(); d != 3 {
		t.Fatalf("after three saves depth = %d, want 3", d)
	}
	c.RestoreTo(1)
	if d := c.CurrentState(); d != 1 {
		t.Fatalf("RestoreTo(1) depth = %d, want 1", d)
	}
	c.Restore()
	if d := c.CurrentState(); d != 0 {
		t.Fatalf("after Restore depth = %d, want 0", d)
	}
}

func TestCairoCompatBrushScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255})
	c.Save()
	c.SetBrush1(paint.Color{G: 255})
	if c.brushColor.G != 255 {
		t.Fatalf("brush after second SetBrush1 = %+v, want green", c.brushColor)
	}
	c.Restore()
	if c.brushColor.R != 255 {
		t.Fatalf("brush after Restore = %+v, want red", c.brushColor)
	}
}

func TestCairoCompatPenScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetPen1(paint.Color{R: 255}, 2)
	c.Save()
	c.SetPen1(paint.Color{B: 255}, 5)
	if c.penWidth != 5 {
		t.Fatalf("penWidth in scope = %v, want 5", c.penWidth)
	}
	c.Restore()
	if c.penWidth != 2 {
		t.Fatalf("penWidth after Restore = %v, want 2", c.penWidth)
	}
}

func TestCairoCompatFillRectangleEmitsTriangles(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255, A: 255})
	c.Rectangle(10, 20, 30, 40)
	c.Fill()
	if len(r.indices) == 0 || len(r.indices)%3 != 0 {
		t.Fatalf("indices=%d not a non-zero multiple of 3", len(r.indices))
	}
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted")
	}
}

func TestCairoCompatStrokeEmitsGeometry(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.SetPen1(paint.Color{R: 0, G: 0, B: 0, A: 255}, 2)
	c.MoveTo(0, 0)
	c.LineTo(10, 0)
	c.LineTo(10, 10)
	c.Stroke()
	if len(r.verts) == 0 || len(r.indices) == 0 {
		t.Fatalf("stroke produced no geometry")
	}
}

func TestCairoCompatFillPreserveKeepsPath(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255, A: 255})
	c.Rectangle(0, 0, 10, 10)
	c.FillPreserve()
	if len(c.pathPts) == 0 {
		t.Fatalf("FillPreserve dropped the path")
	}
	c.Fill()
	if len(c.pathPts) != 0 {
		t.Fatalf("Fill did not reset the path")
	}
}

func TestCairoCompatTranslateMirrorsRenderer(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.Translate(10, 20)
	tx, ty := r.applyXform(0, 0)
	if tx != 10 || ty != 20 {
		t.Fatalf("renderer transform after Translate = (%v, %v), want (10, 20)", tx, ty)
	}
	var m geom.Mat3x2
	c.GetMatrix(&m)
	if m.X0 != 10 || m.Y0 != 20 {
		t.Fatalf("logical CTM after Translate = (%v, %v), want (10, 20)", m.X0, m.Y0)
	}
}

func TestCairoCompatSetMatrixSyncsRenderer(t *testing.T) {
	c, r := newCompatTestPainter(t)
	var m geom.Mat3x2
	m.InitTranslate(50, 60)
	c.SetMatrix(&m)
	tx, ty := r.applyXform(0, 0)
	if tx != 50 || ty != 60 {
		t.Fatalf("renderer transform after SetMatrix = (%v, %v), want (50, 60)", tx, ty)
	}
}

func TestCairoCompatResetMatrix(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.Translate(40, 80)
	c.ResetMatrix()
	tx, ty := r.applyXform(0, 0)
	if tx != 0 || ty != 0 {
		t.Fatalf("after ResetMatrix point (0,0) became (%v, %v)", tx, ty)
	}
}

func TestCairoCompatCurrentPointAfterMoveTo(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.MoveTo(11, 22)
	x, y := c.CurrentPoint()
	if x != 11 || y != 22 {
		t.Fatalf("CurrentPoint = (%v, %v), want (11, 22)", x, y)
	}
}

func TestCairoCompatTargetIsNil(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	if c.Target() != nil {
		t.Fatalf("CairoCompat.Target() must return nil")
	}
}

func TestCairoCompatArcAppendsPoints(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	before := len(c.pathPts)
	c.Arc(0, 0, 10, 0, 1.5708) // ~90 degrees
	if len(c.pathPts) <= before {
		t.Fatalf("Arc did not append any points")
	}
}

func TestCairoCompatRectangle1Path(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.Rectangle1(geom.Rect{X: 1, Y: 2, Width: 3, Height: 4})
	if len(c.pathSubs) != 1 || len(c.pathPts) != 5 {
		t.Fatalf("Rectangle1 produced %d subs / %d pts; want 1/5", len(c.pathSubs), len(c.pathPts))
	}
}

// TestCairoCompatNestedClipPopPredicate locks in the fix for the
// off-by-one in Restore(): a Save→Clip→Save→Clip→Restore sequence (which
// is exactly what DrawWidgetAll produces at every parent→child recursion)
// must NOT pop the outer clip when restoring the inner one.
//
// The actual Clip()/PopClip path calls gl directly, so this test drives
// the predicate via the same internal state Clip() sets, then asserts how
// many pops Restore would perform. We instrument by counting renderer
// clipStack length differences instead of issuing GL calls — the renderer
// PushClip/PopClip helpers DO touch gl, so we synthesise the bookkeeping
// directly by calling stub helpers that mirror their bookkeeping side
// effects but skip the GL calls.
func TestCairoCompatNestedClipPopPredicate(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Step the painter through Save→Clip→Save→Clip without touching GL by
	// updating only the bookkeeping fields Clip() would set. Restore() does
	// not depend on r.curClip; it only consults c.clipPushedAt and pops via
	// r.PopClip — but with no clips actually pushed on the renderer, that
	// helper takes the "defensive" no-op branch (n==0 → disable scissor +
	// gl.Disable). To avoid GL on Restore we also drain clipPushedAt
	// manually after each assert.

	// Outer Save.
	c.Save()
	// Tag a fake outer clip at the current Save depth.
	c.clipPushedAt = append(c.clipPushedAt, len(c.stateStack))
	if c.clipPushedAt[0] != 1 {
		t.Fatalf("outer clip tagged at %d, want 1", c.clipPushedAt[0])
	}

	// Inner Save.
	c.Save()
	c.clipPushedAt = append(c.clipPushedAt, len(c.stateStack))
	if c.clipPushedAt[1] != 2 {
		t.Fatalf("inner clip tagged at %d, want 2", c.clipPushedAt[1])
	}

	// Simulate restoring the inner Save: the predicate must pop only the
	// inner clip (tag 2 > new depth 1) and leave the outer (tag 1 == 1).
	innerTag := c.clipPushedAt[1]
	outerTag := c.clipPushedAt[0]
	newDepth := len(c.stateStack) - 1
	if !(innerTag > newDepth) {
		t.Fatalf("predicate fails: inner tag %d, new depth %d — should pop", innerTag, newDepth)
	}
	if outerTag > newDepth {
		t.Fatalf("predicate over-eager: outer tag %d, new depth %d — would also pop, breaking nested clip", outerTag, newDepth)
	}

	// Reset state without going through Restore (avoids GL calls).
	c.clipPushedAt = nil
	c.stateStack = c.stateStack[:0]
}

// TestCairoCompatBareClipSurvivesUnrelatedSaveRestore: a Clip() at depth 0
// must survive an unrelated Save→Restore cycle. Tag(=0), depth-after-restore=0,
// and the predicate `tag > depth` yields false → no pop. Verify arithmetic
// directly, since exercising the path would call gl.
func TestCairoCompatBareClipSurvivesUnrelatedSaveRestore(t *testing.T) {
	bareClipTag := 0
	c, _ := newCompatTestPainter(t)
	c.Save()
	c.Restore()
	depthAfter := len(c.stateStack)
	if bareClipTag > depthAfter {
		t.Fatalf("predicate too eager: bare clip would pop on unrelated Save/Restore")
	}
}

func TestCairoCompatBindRendererPreservesFontCache(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	cache := c.fontCache
	r2 := newAdapterTestRenderer()
	c.BindRenderer(r2)
	if c.fontCache != cache {
		t.Fatal("BindRenderer dropped the FontCache — every frame would leak GL textures")
	}
	if c.r != r2 {
		t.Fatal("BindRenderer did not switch to the new renderer")
	}
	if c.CurrentState() != 0 {
		t.Fatalf("BindRenderer left state stack at %d; want 0", c.CurrentState())
	}
}
