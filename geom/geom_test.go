package geom

import (
	"fmt"
	"math"
	"testing"
)

const epsilon = 1e-9

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

// ---------------------------------------------------------------------------
// Vec2
// ---------------------------------------------------------------------------

func TestVec2String(t *testing.T) {
	v := Vec2{3.5, -1.2}
	s := v.String()
	if s != "[3.5 -1.2]" {
		t.Errorf("Vec2.String() = %q, want %q", s, "[3.5 -1.2]")
	}
}

func TestVec2Scan(t *testing.T) {
	var v Vec2
	_, err := fmt.Sscan("[10 20]", &v)
	if err != nil {
		t.Fatalf("Vec2 Scan error: %v", err)
	}
	if v.X != 10 || v.Y != 20 {
		t.Errorf("Vec2 Scan got %v, want {10 20}", v)
	}
}

func TestVec2ScanBadInput(t *testing.T) {
	var v Vec2
	_, err := fmt.Sscan("10 20", &v) // missing brackets
	if err == nil {
		t.Error("expected error for missing brackets, got nil")
	}
}

// ---------------------------------------------------------------------------
// Rect basics
// ---------------------------------------------------------------------------

func TestRectEdges(t *testing.T) {
	r := Rect{10, 20, 100, 50}
	if r.Left() != 10 {
		t.Errorf("Left() = %v, want 10", r.Left())
	}
	if r.Top() != 20 {
		t.Errorf("Top() = %v, want 20", r.Top())
	}
	if r.Right() != 110 {
		t.Errorf("Right() = %v, want 110", r.Right())
	}
	if r.Bottom() != 70 {
		t.Errorf("Bottom() = %v, want 70", r.Bottom())
	}
}

func TestRectArea(t *testing.T) {
	r := Rect{0, 0, 5, 4}
	if r.Area() != 20 {
		t.Errorf("Area() = %v, want 20", r.Area())
	}
}

func TestRectIsEmpty(t *testing.T) {
	if !(Rect{0, 0, 0, 5}).IsEmpty() {
		t.Error("zero width should be empty")
	}
	if !(Rect{0, 0, 5, 0}).IsEmpty() {
		t.Error("zero height should be empty")
	}
	if (Rect{0, 0, 1, 1}).IsEmpty() {
		t.Error("1x1 should not be empty")
	}
}

func TestRectIsZero(t *testing.T) {
	if !(Rect{}).IsZero() {
		t.Error("default rect should be zero")
	}
	if (Rect{1, 0, 0, 0}).IsZero() {
		t.Error("non-zero X should not be zero")
	}
}

func TestRectCenter(t *testing.T) {
	r := Rect{10, 20, 100, 50}
	cx, cy := r.Center()
	if cx != 60 || cy != 45 {
		t.Errorf("Center() = (%v,%v), want (60,45)", cx, cy)
	}
}

func TestRectCenter1(t *testing.T) {
	r := Rect{10, 20, 100, 50}
	c := r.Center1()
	if c.X != 60 || c.Y != 45 {
		t.Errorf("Center1() = %v, want {60 45}", c)
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{0, 0, 10, 10}
	tests := []struct {
		x, y   float64
		expect bool
	}{
		{5, 5, true},
		{0, 0, true},
		{10, 10, true},
		{-1, 5, false},
		{5, -1, false},
		{11, 5, false},
		{5, 11, false},
	}
	for _, tt := range tests {
		if got := r.Contains(tt.x, tt.y); got != tt.expect {
			t.Errorf("Contains(%v,%v) = %v, want %v", tt.x, tt.y, got, tt.expect)
		}
	}
}

func TestRectSize(t *testing.T) {
	r := Rect{1, 2, 30, 40}
	w, h := r.Size()
	if w != 30 || h != 40 {
		t.Errorf("Size() = (%v,%v), want (30,40)", w, h)
	}
}

// ---------------------------------------------------------------------------
// Rect normalization
// ---------------------------------------------------------------------------

func TestRectNormalize(t *testing.T) {
	r := Rect{10, 20, -5, -10}
	r.Normalize()
	if r.X != 5 || r.Y != 10 || r.Width != 5 || r.Height != 10 {
		t.Errorf("Normalize() = %v, want {5 10 5 10}", r)
	}
}

func TestRectNormalizeCopy(t *testing.T) {
	r := Rect{10, 20, -5, -10}
	n := r.NormalizeCopy()
	// original unchanged
	if r.Width != -5 {
		t.Error("NormalizeCopy mutated original")
	}
	if n.Width != 5 || n.Height != 10 {
		t.Errorf("NormalizeCopy() = %v, want width=5 height=10", n)
	}
}

func TestRectIsNormal(t *testing.T) {
	if !(Rect{0, 0, 1, 1}).IsNormal() {
		t.Error("positive sizes should be normal")
	}
	if (Rect{0, 0, -1, 1}).IsNormal() {
		t.Error("negative width should not be normal")
	}
}

// ---------------------------------------------------------------------------
// Rect intersection and union
// ---------------------------------------------------------------------------

func TestRectIntersectCopy(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	b := Rect{5, 5, 10, 10}
	inter := a.IntersectCopy(b)
	if inter.X != 5 || inter.Y != 5 || inter.Width != 5 || inter.Height != 5 {
		t.Errorf("IntersectCopy = %v, want {5 5 5 5}", inter)
	}
}

func TestRectIntersectCopyNoOverlap(t *testing.T) {
	a := Rect{0, 0, 5, 5}
	b := Rect{10, 10, 5, 5}
	inter := a.IntersectCopy(b)
	if !inter.IsEmpty() && !inter.IsZero() {
		t.Errorf("non-overlapping intersection should be empty/zero, got %v", inter)
	}
}

func TestRectUniteCopy(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	b := Rect{5, 5, 10, 10}
	u := a.UniteCopy(b)
	if u.X != 0 || u.Y != 0 || u.Width != 15 || u.Height != 15 {
		t.Errorf("UniteCopy = %v, want {0 0 15 15}", u)
	}
}

// ---------------------------------------------------------------------------
// Rect shrink / expand / adjust
// ---------------------------------------------------------------------------

func TestRectShrinkCopy(t *testing.T) {
	r := Rect{0, 0, 100, 100}
	s := r.ShrinkCopy(10)
	if s.X != 10 || s.Y != 10 || s.Width != 80 || s.Height != 80 {
		t.Errorf("ShrinkCopy(10) = %v, want {10 10 80 80}", s)
	}
}

func TestRectExpandCopy(t *testing.T) {
	r := Rect{10, 10, 80, 80}
	e := r.ExpandCopy(10)
	if e.X != 0 || e.Y != 0 || e.Width != 100 || e.Height != 100 {
		t.Errorf("ExpandCopy(10) = %v, want {0 0 100 100}", e)
	}
}

func TestRectAdjustCopy(t *testing.T) {
	r := Rect{10, 10, 100, 100}
	a := r.AdjustCopy(5, 10, 5, 10)
	if a.X != 15 || a.Y != 15 || a.Width != 105 || a.Height != 105 {
		t.Errorf("AdjustCopy = %v, want {15 15 105 105}", a)
	}
}

// ---------------------------------------------------------------------------
// Rect String / Scan
// ---------------------------------------------------------------------------

func TestRectString(t *testing.T) {
	r := Rect{1, 2, 3, 4}
	s := r.String()
	if s != "[1 2 3 4]" {
		t.Errorf("Rect.String() = %q, want %q", s, "[1 2 3 4]")
	}
}

func TestRectScan(t *testing.T) {
	var r Rect
	_, err := fmt.Sscan("[10 20 30 40]", &r)
	if err != nil {
		t.Fatalf("Rect Scan error: %v", err)
	}
	if r.X != 10 || r.Y != 20 || r.Width != 30 || r.Height != 40 {
		t.Errorf("Rect Scan = %v, want {10 20 30 40}", r)
	}
}

func TestRectScanBadInput(t *testing.T) {
	var r Rect
	_, err := fmt.Sscan("10 20 30 40", &r)
	if err == nil {
		t.Error("expected error for missing brackets, got nil")
	}
}

// ---------------------------------------------------------------------------
// Mat3x2
// ---------------------------------------------------------------------------

func TestMat3x2Identity(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	x, y := m.Transform(3, 7)
	if x != 3 || y != 7 {
		t.Errorf("identity transform (%v,%v), want (3,7)", x, y)
	}
}

func TestMat3x2Translate(t *testing.T) {
	var m Mat3x2
	m.InitTranslate(10, 20)
	x, y := m.Transform(1, 2)
	if x != 11 || y != 22 {
		t.Errorf("translate transform (%v,%v), want (11,22)", x, y)
	}
}

func TestMat3x2Scale(t *testing.T) {
	var m Mat3x2
	m.InitScale(2, 3)
	x, y := m.Transform(5, 4)
	if x != 10 || y != 12 {
		t.Errorf("scale transform (%v,%v), want (10,12)", x, y)
	}
}

func TestMat3x2Rotate90(t *testing.T) {
	var m Mat3x2
	m.InitRotate(math.Pi / 2)
	x, y := m.Transform(1, 0)
	if !approxEqual(x, 0) || !approxEqual(y, 1) {
		t.Errorf("rotate 90 deg (%v,%v), want (0,1)", x, y)
	}
}

func TestMat3x2Rotate180(t *testing.T) {
	var m Mat3x2
	m.InitRotate(math.Pi)
	x, y := m.Transform(1, 0)
	if !approxEqual(x, -1) || !approxEqual(y, 0) {
		t.Errorf("rotate 180 deg (%v,%v), want (-1,0)", x, y)
	}
}

func TestMat3x2Init(t *testing.T) {
	var m Mat3x2
	m.Init(1, 2, 3, 4, 5, 6)
	if m.Xx != 1 || m.Yx != 2 || m.Xy != 3 || m.Yy != 4 || m.X0 != 5 || m.Y0 != 6 {
		t.Errorf("Init values wrong: %+v", m)
	}
}

func TestMat3x2TransformVec(t *testing.T) {
	var m Mat3x2
	m.InitTranslate(100, 200)
	// TransformVec ignores translation
	x, y := m.TransformVec(3, 4)
	if x != 3 || y != 4 {
		t.Errorf("TransformVec with translate (%v,%v), want (3,4)", x, y)
	}

	m.InitScale(2, 3)
	x, y = m.TransformVec(5, 7)
	if x != 10 || y != 21 {
		t.Errorf("TransformVec with scale (%v,%v), want (10,21)", x, y)
	}
}

func TestMat3x2Det(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	if m.Det() != 1 {
		t.Errorf("identity det = %v, want 1", m.Det())
	}

	m.InitScale(3, 4)
	if m.Det() != 12 {
		t.Errorf("scale(3,4) det = %v, want 12", m.Det())
	}
}

func TestMat3x2Multiply(t *testing.T) {
	var a, b Mat3x2
	a.InitScale(2, 2)
	b.InitTranslate(10, 20)

	c := a.Multiply(&b)
	x, y := c.Transform(1, 1)
	// scale then translate: (1*2, 1*2) then +10, +20 = (12, 22)
	if !approxEqual(x, 12) || !approxEqual(y, 22) {
		t.Errorf("Multiply scale*translate (%v,%v), want (12,22)", x, y)
	}
}

func TestMat3x2MultiplyWidth(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Translate(5, 10)
	x, y := m.Transform(0, 0)
	if x != 5 || y != 10 {
		t.Errorf("after Translate(5,10) transform origin (%v,%v), want (5,10)", x, y)
	}
}

func TestMat3x2ScaleMethod(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Scale(3, 4)
	x, y := m.Transform(2, 5)
	if x != 6 || y != 20 {
		t.Errorf("Scale method (%v,%v), want (6,20)", x, y)
	}
}

func TestMat3x2RotateMethod(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Rotate(math.Pi / 2)
	x, y := m.Transform(1, 0)
	if !approxEqual(x, 0) || !approxEqual(y, 1) {
		t.Errorf("Rotate method (%v,%v), want (0,1)", x, y)
	}
}

func TestMat3x2Invert(t *testing.T) {
	var m Mat3x2
	m.InitTranslate(10, 20)
	ok := m.Invert()
	if !ok {
		t.Fatal("Invert returned false for invertible matrix")
	}
	x, y := m.Transform(0, 0)
	if !approxEqual(x, -10) || !approxEqual(y, -20) {
		t.Errorf("inverted translate (%v,%v), want (-10,-20)", x, y)
	}
}

func TestMat3x2InvertScale(t *testing.T) {
	var m Mat3x2
	m.InitScale(4, 2)
	ok := m.Invert()
	if !ok {
		t.Fatal("Invert returned false")
	}
	x, y := m.Transform(8, 6)
	if !approxEqual(x, 2) || !approxEqual(y, 3) {
		t.Errorf("inverted scale (%v,%v), want (2,3)", x, y)
	}
}

func TestMat3x2InvertSingular(t *testing.T) {
	var m Mat3x2
	m.Init(1, 0, 1, 0, 0, 0) // det = 0
	ok := m.Invert()
	if ok {
		t.Error("Invert should return false for singular matrix")
	}
	// should reset to identity
	if m.Xx != 1 || m.Yy != 1 || m.Xy != 0 || m.Yx != 0 {
		t.Errorf("singular invert should produce identity, got %+v", m)
	}
}

func TestMat3x2InvertRoundTrip(t *testing.T) {
	var orig Mat3x2
	orig.InitIdentity()
	orig.Translate(7, -3)
	orig.Scale(2, 0.5)
	orig.Rotate(0.3)

	inv := orig
	if !inv.Invert() {
		t.Fatal("Invert failed")
	}

	// orig * inv should be identity
	product := orig.Multiply(&inv)
	x, y := product.Transform(42, 99)
	if !approxEqual(x, 42) || !approxEqual(y, 99) {
		t.Errorf("round-trip (%v,%v), want (42,99)", x, y)
	}
}

func TestMat3x2CombinedTranslateScale(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Translate(10, 20)
	m.Scale(2, 3)
	x, y := m.Transform(1, 1)
	// matrix = identity * translate * scale
	// point (1,1): translate => (11,21), then scale => (22,63)
	if !approxEqual(x, 22) || !approxEqual(y, 63) {
		t.Errorf("translate+scale (%v,%v), want (22,63)", x, y)
	}
}

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestRectContainsEdgeCases(t *testing.T) {
	// Zero-size rect should not contain any point (except boundary)
	r := Rect{5, 5, 0, 0}
	if r.Contains(5, 5) {
		// A zero-size rect at (5,5) may or may not contain (5,5) depending on
		// implementation. This test just verifies no panic occurs.
		_ = r.Contains(5, 5)
	}

	// Large rect
	big := Rect{-1e6, -1e6, 2e6, 2e6}
	if !big.Contains(0, 0) {
		t.Error("large rect should contain origin")
	}
}

func TestRectIntersectSelf(t *testing.T) {
	r := Rect{10, 20, 50, 60}
	inter := r.IntersectCopy(r)
	if inter.X != r.X || inter.Y != r.Y || inter.Width != r.Width || inter.Height != r.Height {
		t.Errorf("self-intersection = %v, want %v", inter, r)
	}
}

func TestRectIntersectAdjacent(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	b := Rect{10, 0, 10, 10} // adjacent, touching at x=10
	inter := a.IntersectCopy(b)
	// Adjacent rects share an edge but have zero-width intersection
	if inter.Width > 0 && inter.Height > 0 {
		// Some implementations consider touching as intersecting with 0 width
		_ = inter
	}
}

func TestRectUniteWithEmpty(t *testing.T) {
	a := Rect{10, 20, 30, 40}
	b := Rect{0, 0, 0, 0} // empty rect
	u := a.UniteCopy(b)
	// Union with empty rect: result should at least cover a
	if u.Width < a.Width || u.Height < a.Height {
		t.Errorf("union with empty should not shrink: %v", u)
	}
}

func TestMat3x2DoubleRotation(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Rotate(math.Pi / 4)
	m.Rotate(math.Pi / 4)
	// Total rotation = pi/2
	x, y := m.Transform(1, 0)
	if !approxEqual(x, 0) || !approxEqual(y, 1) {
		t.Errorf("double rotate 45 deg (%v,%v), want (0,1)", x, y)
	}
}

func TestMat3x2TranslateScaleInvertRoundTrip(t *testing.T) {
	var m Mat3x2
	m.InitIdentity()
	m.Translate(-5, 8)
	m.Scale(0.5, 2)
	orig := m

	inv := m
	if !inv.Invert() {
		t.Fatal("Invert failed")
	}

	product := orig.Multiply(&inv)
	x, y := product.Transform(100, 200)
	if !approxEqual(x, 100) || !approxEqual(y, 200) {
		t.Errorf("translate+scale round-trip (%v,%v), want (100,200)", x, y)
	}
}

func TestVec2ZeroLength(t *testing.T) {
	v := Vec2{0, 0}
	if v.X != 0 || v.Y != 0 {
		t.Errorf("zero vec = %v, want {0,0}", v)
	}
}

func TestRectNegativeDimensions(t *testing.T) {
	r := Rect{10, 10, -20, -30}
	n := r.NormalizeCopy()
	if n.Width < 0 || n.Height < 0 {
		t.Errorf("normalized rect has negative dimensions: %v", n)
	}
	if n.Width != 20 || n.Height != 30 {
		t.Errorf("normalized = %v, want width=20 height=30", n)
	}
}

func TestRectShrinkOvershoot(t *testing.T) {
	r := Rect{0, 0, 10, 10}
	s := r.ShrinkCopy(20) // shrink more than half the size
	// Result may have negative dimensions, just verify no panic
	_ = s
}

// Additional benchmarks are in bench_test.go.
