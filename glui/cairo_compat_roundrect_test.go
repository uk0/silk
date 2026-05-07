package glui

import (
	"math"
	"testing"

	"silk/paint"
)

// emitCanonicalRoundedRect reproduces the path the bench's RoundedRect
// scenario builds: MoveTo + LineTo + 4 quarter-circle arcs at the
// corners, walking clockwise from the top edge.
func emitCanonicalRoundedRect(g paint.Painter, x, y, w, h, r float64) {
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
}

// TestDetectRoundedRectRecognisesCanonicalShape feeds the canonical
// rounded-rect path through CairoCompat and asserts that
// detectRoundedRect returns the expected outer rect + radius.
func TestDetectRoundedRectRecognisesCanonicalShape(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	emitCanonicalRoundedRect(c, 10, 20, 100, 60, 8)

	rect, radius, ok := c.detectRoundedRect()
	if !ok {
		t.Fatalf("canonical rounded-rect path should be detected")
	}
	if math.Abs(float64(rect.X-10)) > 1e-3 || math.Abs(float64(rect.Y-20)) > 1e-3 {
		t.Errorf("rect origin = (%v, %v), want (10, 20)", rect.X, rect.Y)
	}
	if math.Abs(float64(rect.W-100)) > 1e-3 || math.Abs(float64(rect.H-60)) > 1e-3 {
		t.Errorf("rect size = (%v x %v), want (100 x 60)", rect.W, rect.H)
	}
	if math.Abs(float64(radius-8)) > 1e-3 {
		t.Errorf("radius = %v, want 8", radius)
	}
}

// TestRoundedRectFastPathFillRoutesToFillRoundedRect verifies that the
// fast path sets the renderer batch kind to kindRect — that's the SDF
// rect shader's batch, the same one Renderer.FillRoundedRect emits to.
// If the slow tessellation path took over, the batch would be kindPath.
func TestRoundedRectFastPathFillRoutesToFillRoundedRect(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetBrush(&paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}})
	emitCanonicalRoundedRect(c, 0, 0, 50, 30, 6)
	c.Fill()

	if r.curKind != kindRect {
		t.Errorf("after rounded-rect Fill, curKind=%d, want kindRect=%d (slow path engaged?)", r.curKind, kindRect)
	}
	// Exactly one quad worth of geometry: 4 verts + 6 indices.
	if len(r.verts) != 4 {
		t.Errorf("verts = %d, want 4 (one SDF quad)", len(r.verts))
	}
	if len(r.indices) != 6 {
		t.Errorf("indices = %d, want 6", len(r.indices))
	}
}

// TestRoundedRectFastPathSkippedForGradientBrush keeps the rounded-rect
// detector solid-only for now (gradient + rounded-rect on the GPU
// would need a combined shader). When a gradient is active the slow
// path must still kick in.
func TestRoundedRectFastPathSkippedForGradientBrush(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	grad := paint.NewLinearGradient(0, 0, 0, 30)
	grad.AddStop(0, paint.Color{R: 0, G: 0, B: 0, A: 255})
	grad.AddStop(1, paint.Color{R: 255, G: 255, B: 255, A: 255})
	c.SetBrush(grad)

	emitCanonicalRoundedRect(c, 0, 0, 50, 30, 6)
	c.Fill()

	// Either the gradient path took over (kindGradient) or the slow
	// triangulation (kindPath) — anything but kindRect, which would be
	// the rounded-rect fast path mis-firing under gradient brush.
	if r.curKind == kindRect {
		t.Errorf("gradient brush should not engage rounded-rect SDF fast path")
	}
}

// TestRoundedRectFastPathSkippedForArbitraryArcs ensures a path that
// happens to contain four arcs but is NOT a rounded rect (e.g. four
// disconnected circles) doesn't fool the detector.
func TestRoundedRectFastPathSkippedForArbitraryArcs(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetBrush(&paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}})
	// Four full circles at scattered positions — same-radius but not
	// the canonical rounded-rect shape (each arc spans 2π not π/2).
	c.MoveTo(10, 10)
	c.Arc(15, 15, 5, 0, 2*math.Pi)
	c.Arc(35, 15, 5, 0, 2*math.Pi)
	c.Arc(35, 35, 5, 0, 2*math.Pi)
	c.Arc(15, 35, 5, 0, 2*math.Pi)
	c.Fill()

	// detectRoundedRect should reject (each arc spans 2π not π/2). The
	// fill consequently goes through path triangulation (kindPath),
	// not the SDF rect shader.
	if r.curKind == kindRect {
		t.Errorf("4-circle path falsely matched rounded-rect SDF; curKind=%d", r.curKind)
	}
}

// TestRoundedRectFastPathSkippedForMismatchedRadii rejects the case
// where corner arcs have different radii (a non-uniform corner-rounded
// shape can't be expressed by FillRoundedRect's single-radius shader).
func TestRoundedRectFastPathSkippedForMismatchedRadii(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetBrush(&paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}})
	// Same as canonical, but the top-right arc has a different radius.
	x, y, w, h := 0.0, 0.0, 60.0, 40.0
	r1 := 8.0
	r2 := 4.0
	c.MoveTo(x+r1, y)
	c.LineTo(x+w-r2, y)
	c.Arc(x+w-r2, y+r2, r2, -math.Pi/2, 0)
	c.LineTo(x+w, y+h-r1)
	c.Arc(x+w-r1, y+h-r1, r1, 0, math.Pi/2)
	c.LineTo(x+r1, y+h)
	c.Arc(x+r1, y+h-r1, r1, math.Pi/2, math.Pi)
	c.LineTo(x, y+r1)
	c.Arc(x+r1, y+r1, r1, math.Pi, 3*math.Pi/2)
	c.Fill()

	if _, _, ok := c.detectRoundedRect(); ok {
		t.Errorf("mismatched radii should not pass detectRoundedRect")
	}
	if r.curKind == kindRect {
		t.Errorf("mismatched-radius path engaged the SDF rect fast path")
	}
}

// TestMoveToResetsArcTracker locks in the contract that starting a new
// sub-path with MoveTo clears the tracker — otherwise stale arcs from
// the previous path could combine with new ones and falsely match.
func TestMoveToResetsArcTracker(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	// First path: emit two arcs (incomplete rounded rect).
	c.MoveTo(0, 0)
	c.Arc(10, 10, 5, 0, math.Pi/2)
	c.Arc(20, 20, 5, 0, math.Pi/2)
	if got := len(c.arcsInPath); got != 2 {
		t.Fatalf("after 2 arcs, len(arcsInPath)=%d, want 2", got)
	}

	// New path: must reset the tracker.
	c.MoveTo(100, 100)
	if got := len(c.arcsInPath); got != 0 {
		t.Errorf("MoveTo should clear arcsInPath; got %d still recorded", got)
	}
}
