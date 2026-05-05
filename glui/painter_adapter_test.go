package glui

import "testing"

func newAdapterTestRenderer() *Renderer {
	return &Renderer{
		verts:   make([]vertex, 0, 16),
		indices: make([]uint16, 0, 16),
		frameW:  100,
		frameH:  100,
		xform:   identityMatrix3(),
	}
}

func TestPainterAdapterSaveRestore(t *testing.T) {
	r := newAdapterTestRenderer()
	p := NewPainterAdapter(r)
	if d := p.CurrentState(); d != 0 {
		t.Fatalf("initial depth = %d, want 0", d)
	}
	p.Save()
	p.Save()
	if d := p.CurrentState(); d != 2 {
		t.Fatalf("after two saves depth = %d, want 2", d)
	}
	p.Restore()
	if d := p.CurrentState(); d != 1 {
		t.Fatalf("after one restore depth = %d, want 1", d)
	}
	p.RestoreTo(0)
	if d := p.CurrentState(); d != 0 {
		t.Fatalf("after RestoreTo(0) depth = %d, want 0", d)
	}
}

func TestPainterAdapterBrushScopedBySaveRestore(t *testing.T) {
	r := newAdapterTestRenderer()
	p := NewPainterAdapter(r)
	p.SetBrush1(Color{1, 0, 0, 1})
	p.Save()
	p.SetBrush1(Color{0, 1, 0, 1})
	if p.brush.G != 1 {
		t.Fatalf("brush after second SetBrush1 = %+v, want green", p.brush)
	}
	p.Restore()
	if p.brush.R != 1 {
		t.Fatalf("brush after Restore = %+v, want red", p.brush)
	}
}

func TestPainterAdapterFillRectangleEmitsTriangles(t *testing.T) {
	r := newAdapterTestRenderer()
	p := NewPainterAdapter(r)
	p.SetBrush1(Color{1, 1, 1, 1})
	p.Rectangle(10, 20, 30, 40)
	p.Fill()

	// Rectangle records a 5-point closed path; the close-vertex is stripped
	// before triangulation, leaving 4 unique points → 2 ear-clip triangles.
	if len(r.indices)%3 != 0 || len(r.indices) == 0 {
		t.Fatalf("indices=%d not a multiple of 3 or empty", len(r.indices))
	}
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted")
	}
}

func TestPainterAdapterStrokeEmitsLines(t *testing.T) {
	r := newAdapterTestRenderer()
	p := NewPainterAdapter(r)
	p.SetPen1(Color{0, 0, 0, 1}, 2)
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Stroke()

	if len(r.verts) == 0 || len(r.indices) == 0 {
		t.Fatalf("stroke produced no geometry")
	}
}

func TestPainterAdapterFillPreserveKeepsPath(t *testing.T) {
	r := newAdapterTestRenderer()
	p := NewPainterAdapter(r)
	p.SetBrush1(Color{1, 1, 1, 1})
	p.Rectangle(0, 0, 10, 10)
	p.FillPreserve()
	if len(p.pathPts) == 0 {
		t.Fatalf("FillPreserve dropped the path")
	}
	p.Fill()
	if len(p.pathPts) != 0 {
		t.Fatalf("Fill did not reset the path")
	}
}
