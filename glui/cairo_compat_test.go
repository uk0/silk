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
