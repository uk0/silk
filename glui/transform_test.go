package glui

import (
	"math"
	"testing"
)

const transformEps = 1e-5

func nearly(a, b float32) bool {
	d := float64(a - b)
	if d < 0 {
		d = -d
	}
	return d < transformEps
}

func newTestRenderer() *Renderer {
	r := &Renderer{
		verts:   make([]vertex, 0, 16),
		indices: make([]uint16, 0, 16),
		frameW:  100,
		frameH:  100,
		xform:   identityMatrix3(),
	}
	return r
}

func TestIdentity(t *testing.T) {
	r := newTestRenderer()
	x, y := r.applyXform(7, 11)
	if !nearly(x, 7) || !nearly(y, 11) {
		t.Fatalf("identity changed point: got (%g, %g)", x, y)
	}
}

func TestTranslate(t *testing.T) {
	r := newTestRenderer()
	r.Translate(10, 20)
	x, y := r.applyXform(3, 4)
	if !nearly(x, 13) || !nearly(y, 24) {
		t.Fatalf("translate(10,20) of (3,4) -> (%g,%g), want (13,24)", x, y)
	}
}

func TestScale(t *testing.T) {
	r := newTestRenderer()
	r.Scale(2, 3)
	x, y := r.applyXform(5, 6)
	if !nearly(x, 10) || !nearly(y, 18) {
		t.Fatalf("scale(2,3) of (5,6) -> (%g,%g), want (10,18)", x, y)
	}
}

func TestRotate90(t *testing.T) {
	r := newTestRenderer()
	r.Rotate(float32(math.Pi / 2))
	// Y-down: 90° CW rotation sends (1, 0) to (0, 1).
	x, y := r.applyXform(1, 0)
	if !nearly(x, 0) || !nearly(y, 1) {
		t.Fatalf("rotate(pi/2) of (1,0) -> (%g,%g), want (0,1)", x, y)
	}
	// (0, 1) should land at (-1, 0).
	x, y = r.applyXform(0, 1)
	if !nearly(x, -1) || !nearly(y, 0) {
		t.Fatalf("rotate(pi/2) of (0,1) -> (%g,%g), want (-1,0)", x, y)
	}
}

func TestComposeTranslateScale(t *testing.T) {
	// translate then scale: child operation applies first to user-space points,
	// so a point (1, 1) under T(10,10)·S(2,2) is (10+2, 10+2) = (12, 12).
	r := newTestRenderer()
	r.Translate(10, 10)
	r.Scale(2, 2)
	x, y := r.applyXform(1, 1)
	if !nearly(x, 12) || !nearly(y, 12) {
		t.Fatalf("T(10,10)·S(2,2) of (1,1) -> (%g,%g), want (12,12)", x, y)
	}
}

func TestNestedSaveRestore(t *testing.T) {
	r := newTestRenderer()
	r.Translate(5, 5)
	r.Save()
	r.Translate(2, 0)
	r.Save()
	r.Scale(3, 3)

	// Inside the deepest scope, (1, 0) -> first scale by 3 -> (3, 0)
	// -> translate +2 -> (5, 0) -> translate +5 -> (10, 0).
	x, y := r.applyXform(1, 0)
	if !nearly(x, 10) || !nearly(y, 5) {
		t.Fatalf("deepest scope of (1,0) -> (%g,%g), want (10,5)", x, y)
	}

	// Restore once: lose the scale.
	r.Restore()
	x, y = r.applyXform(1, 0)
	if !nearly(x, 8) || !nearly(y, 5) {
		t.Fatalf("after one restore of (1,0) -> (%g,%g), want (8,5)", x, y)
	}

	// Restore again: lose the second translate.
	r.Restore()
	x, y = r.applyXform(1, 0)
	if !nearly(x, 6) || !nearly(y, 5) {
		t.Fatalf("after two restores of (1,0) -> (%g,%g), want (6,5)", x, y)
	}
}

func TestRestoreUnbalancedNoop(t *testing.T) {
	r := newTestRenderer()
	r.Translate(1, 2)
	// Pop twice though we never pushed.
	r.Restore()
	r.Restore()
	x, y := r.applyXform(0, 0)
	if !nearly(x, 1) || !nearly(y, 2) {
		t.Fatalf("unbalanced restore corrupted state: (%g,%g), want (1,2)", x, y)
	}
}

func TestProjectAppliesTransform(t *testing.T) {
	r := newTestRenderer()
	// Identity should project (0,0) -> (-1, 1) (top-left in clip space).
	cx, cy := r.project(0, 0)
	if !nearly(cx, -1) || !nearly(cy, 1) {
		t.Fatalf("identity project(0,0) -> (%g,%g), want (-1,1)", cx, cy)
	}
	// After Translate(50,50) on a 100x100 frame, (0,0) is mapped to (50,50)
	// in logical coords, then to (0,0) in clip space.
	r.Translate(50, 50)
	cx, cy = r.project(0, 0)
	if !nearly(cx, 0) || !nearly(cy, 0) {
		t.Fatalf("after translate, project(0,0) -> (%g,%g), want (0,0)", cx, cy)
	}
}

func TestPushQuadRotated(t *testing.T) {
	// A 45° rotation should produce four corners that are NOT axis aligned.
	// Verifies that pushQuad transforms each corner independently rather
	// than synthesising rotated corners from two diagonals.
	r := newTestRenderer()
	r.Rotate(float32(math.Pi / 4))
	r.pushQuad(0, 0, 10, 10, 0, 0, 1, 1, Color{1, 1, 1, 1})

	if len(r.verts) != 4 {
		t.Fatalf("pushQuad emitted %d verts, want 4", len(r.verts))
	}
	// Manually compute the expected projected corners.
	corners := [4][2]float32{
		{0, 0}, {10, 0}, {10, 10}, {0, 10},
	}
	for i, c := range corners {
		wantCx, wantCy := r.project(c[0], c[1])
		gotCx := r.verts[i].X
		gotCy := r.verts[i].Y
		if !nearly(gotCx, wantCx) || !nearly(gotCy, wantCy) {
			t.Errorf("corner %d: got (%g,%g) want (%g,%g)", i, gotCx, gotCy, wantCx, wantCy)
		}
	}

	// Sanity: the "top" edge of the quad (verts 0->1) should not be
	// axis-aligned under rotation. Its dy must be non-zero.
	dy := r.verts[1].Y - r.verts[0].Y
	if nearly(dy, 0) {
		t.Errorf("rotated top edge has zero dy=%g — pushQuad ignored rotation", dy)
	}
}
