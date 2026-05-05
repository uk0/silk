package glui

import "testing"

// TestBoxShadowAccumulatesQuad verifies FillBoxShadow emits a single
// rect-kind quad (4 vertices, 6 indices) and does not panic without a
// real GL context. The setBatch transition from kindNone → kindRect
// fires a flush(), but flush returns early when no indices are pending,
// so no GL calls happen.
func TestBoxShadowAccumulatesQuad(t *testing.T) {
	r := newTestRenderer()
	r.FillBoxShadow(Rect{X: 50, Y: 50, W: 100, H: 50}, 8, 12,
		Color{0, 0, 0, 0.3})
	if len(r.verts) != 4 {
		t.Errorf("expected 4 vertices, got %d", len(r.verts))
	}
	if len(r.indices) != 6 {
		t.Errorf("expected 6 indices, got %d", len(r.indices))
	}
}

// TestBoxShadowDegenerate verifies the zero-input guards: a zero blur or
// zero-area rect must emit nothing rather than push a degenerate quad.
func TestBoxShadowDegenerate(t *testing.T) {
	r := newTestRenderer()
	r.FillBoxShadow(Rect{X: 0, Y: 0, W: 100, H: 50}, 8, 0,
		Color{0, 0, 0, 0.3})
	if len(r.verts) != 0 {
		t.Errorf("zero blur: expected 0 vertices, got %d", len(r.verts))
	}

	r.FillBoxShadow(Rect{X: 0, Y: 0, W: 0, H: 50}, 8, 12,
		Color{0, 0, 0, 0.3})
	if len(r.verts) != 0 {
		t.Errorf("zero width: expected 0 vertices, got %d", len(r.verts))
	}
}

// TestBoxShadowQuadIsOutset verifies the emitted quad covers the rect
// inflated by blur on each side. We project the corners back through the
// renderer's clip-space to point coordinates and check the bounding box.
func TestBoxShadowQuadIsOutset(t *testing.T) {
	r := newTestRenderer()
	r.frameW = 200
	r.frameH = 200
	r.FillBoxShadow(Rect{X: 50, Y: 50, W: 100, H: 50}, 0, 10,
		Color{0, 0, 0, 0.3})
	if len(r.verts) != 4 {
		t.Fatalf("expected 4 vertices, got %d", len(r.verts))
	}

	// Vertex 0 is the top-left corner of the inflated quad. Reverse the
	// projection: clip x = (px/w)*2-1 → px = (clip+1)/2 * w.
	v0 := r.verts[0]
	px0 := (v0.X + 1) * 0.5 * r.frameW
	py0 := (1 - v0.Y) * 0.5 * r.frameH
	if !nearly(px0, 40) || !nearly(py0, 40) {
		t.Errorf("top-left = (%g, %g), want (40, 40)", px0, py0)
	}

	v2 := r.verts[2]
	px2 := (v2.X + 1) * 0.5 * r.frameW
	py2 := (1 - v2.Y) * 0.5 * r.frameH
	if !nearly(px2, 160) || !nearly(py2, 110) {
		t.Errorf("bot-right = (%g, %g), want (160, 110)", px2, py2)
	}
}
