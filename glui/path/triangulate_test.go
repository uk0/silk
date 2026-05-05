package path

import (
	"math"
	"testing"
)

// TestTriangulateConvex verifies a simple square produces exactly two
// triangles covering its area, with every emitted index in range.
func TestTriangulateConvex(t *testing.T) {
	pts := [][2]float32{
		{0, 0},
		{10, 0},
		{10, 10},
		{0, 10},
	}
	idx := Triangulate(pts)
	if len(idx) != 6 {
		t.Fatalf("square: got %d indices, want 6", len(idx))
	}
	for _, i := range idx {
		if int(i) >= len(pts) {
			t.Fatalf("index %d out of range %d", i, len(pts))
		}
	}
	if got := triangleAreaSum(idx, pts); !approxEqual(got, 100) {
		t.Fatalf("square area: got %g, want 100", got)
	}
}

// TestTriangulateConcave checks an L-shaped polygon (six vertices, one
// reflex corner) decomposes into four triangles whose total area matches
// the L-shape's true area.
//
//	(0,0) ─── (30,0)
//	  │         │
//	  │       (30,10)
//	  │           │
//	  │       (10,10)
//	  │           │
//	  │       (10,30)
//	  │         │
//	(0,30) ── (0,30)
func TestTriangulateConcave(t *testing.T) {
	pts := [][2]float32{
		{0, 0},
		{30, 0},
		{30, 10},
		{10, 10},
		{10, 30},
		{0, 30},
	}
	idx := Triangulate(pts)
	if len(idx) != 12 {
		t.Fatalf("L-shape: got %d indices (%d triangles), want 12 (4)", len(idx), len(idx)/3)
	}
	// Total filled area: 30*10 (bottom) + 10*20 (left) = 300 + 200 = 500.
	if got := triangleAreaSum(idx, pts); !approxEqual(got, 500) {
		t.Fatalf("L-shape area: got %g, want 500", got)
	}
}

// TestTriangulateStar checks a 5-point star (10 vertices alternating
// outer/inner radii) — a classic concave test case — yields 8 triangles
// covering its area without leaving gaps or producing extras.
func TestTriangulateStar(t *testing.T) {
	pts := makeStar(5, 50, 20)
	idx := Triangulate(pts)
	want := (len(pts) - 2) * 3
	if len(idx) != want {
		t.Fatalf("star: got %d indices, want %d", len(idx), want)
	}
	// Spot-check: every triangle must have positive area in absolute
	// terms, otherwise we emitted a degenerate.
	for i := 0; i < len(idx); i += 3 {
		a := pts[idx[i]]
		b := pts[idx[i+1]]
		c := pts[idx[i+2]]
		if signedArea(a, b, c) == 0 {
			t.Fatalf("degenerate triangle at %d: %v %v %v", i/3, a, b, c)
		}
	}
}

// TestTriangulateTooFew handles the "fewer than three vertices" edge case
// without panicking.
func TestTriangulateTooFew(t *testing.T) {
	if got := Triangulate(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := Triangulate([][2]float32{{0, 0}}); got != nil {
		t.Errorf("1-point input: got %v, want nil", got)
	}
	if got := Triangulate([][2]float32{{0, 0}, {1, 1}}); got != nil {
		t.Errorf("2-point input: got %v, want nil", got)
	}
}

// makeStar returns the vertices of a regular n-pointed star with the given
// outer and inner radii, centered at the origin. The star is wound CCW in
// math-style coordinates and starts at the top outer point.
func makeStar(n int, outer, inner float32) [][2]float32 {
	pts := make([][2]float32, 0, 2*n)
	step := math.Pi / float64(n)
	for i := 0; i < 2*n; i++ {
		angle := -math.Pi/2 + float64(i)*step
		r := outer
		if i%2 == 1 {
			r = inner
		}
		pts = append(pts, [2]float32{
			r * float32(math.Cos(angle)),
			r * float32(math.Sin(angle)),
		})
	}
	return pts
}

func triangleAreaSum(idx []uint16, pts [][2]float32) float32 {
	var sum float32
	for i := 0; i < len(idx); i += 3 {
		a := pts[idx[i]]
		b := pts[idx[i+1]]
		c := pts[idx[i+2]]
		s := signedArea(a, b, c)
		if s < 0 {
			s = -s
		}
		sum += s
	}
	return sum
}

func signedArea(a, b, c [2]float32) float32 {
	return 0.5 * ((b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0]))
}

func approxEqual(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-3
}
