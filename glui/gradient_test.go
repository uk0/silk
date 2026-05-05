package glui

import (
	"silk/paint"
	"testing"
)

// Tests for the linear-gradient pipeline added in this round. They run
// off-GL: vertex/index accumulation in the Renderer is observable without a
// real OpenGL context. Shader compilation requires GL and is exercised by
// the smoke-test harness when a Window is available.

// TestFillGradientRectAccumulatesQuad: a single FillGradientRect emits one
// quad (4 vertices, 6 indices) with the requested kind active, regardless
// of axis.
func TestFillGradientRectAccumulatesQuad(t *testing.T) {
	r := newAdapterTestRenderer()
	start := Color{R: 1, G: 0, B: 0, A: 1}
	end := Color{R: 0, G: 1, B: 0, A: 1}
	r.FillGradientRect(Rect{X: 0, Y: 0, W: 50, H: 50}, start, end, false)

	if r.curKind != kindGradient {
		t.Fatalf("after FillGradientRect curKind = %v, want kindGradient", r.curKind)
	}
	if got := len(r.verts); got != 4 {
		t.Fatalf("expected 4 vertices for one quad, got %d", got)
	}
	if got := len(r.indices); got != 6 {
		t.Fatalf("expected 6 indices for one quad, got %d", got)
	}
	if r.gradStart != start || r.gradEnd != end {
		t.Fatalf("gradient stops not stored: start=%+v end=%+v", r.gradStart, r.gradEnd)
	}
}

// TestFillGradientRectHorizontalT: horizontal axis must place t=0 at the
// two left vertices and t=1 at the two right vertices. The shader reads
// v_uv.x, so the U component is what matters.
func TestFillGradientRectHorizontalT(t *testing.T) {
	r := newAdapterTestRenderer()
	r.FillGradientRect(Rect{X: 0, Y: 0, W: 10, H: 10}, Color{R: 1}, Color{G: 1}, false)
	if len(r.verts) != 4 {
		t.Fatalf("expected 4 vertices, got %d", len(r.verts))
	}
	// Vertex order from the renderer: top-left, top-right, bottom-right,
	// bottom-left.  Horizontal: U(top-left) = U(bottom-left) = 0; U(top-
	// right) = U(bottom-right) = 1.
	if r.verts[0].U != 0 || r.verts[3].U != 0 {
		t.Fatalf("horizontal gradient: expected U=0 at left vertices, got TL=%v BL=%v",
			r.verts[0].U, r.verts[3].U)
	}
	if r.verts[1].U != 1 || r.verts[2].U != 1 {
		t.Fatalf("horizontal gradient: expected U=1 at right vertices, got TR=%v BR=%v",
			r.verts[1].U, r.verts[2].U)
	}
}

// TestFillGradientRectVerticalT: vertical axis places t=0 at the two top
// vertices and t=1 at the two bottom vertices. We pack the parameter into
// v_uv.x (not .y) so the same fragment shader works for both axes — the
// vertex emit must therefore put the t value into U.
func TestFillGradientRectVerticalT(t *testing.T) {
	r := newAdapterTestRenderer()
	r.FillGradientRect(Rect{X: 0, Y: 0, W: 10, H: 10}, Color{R: 1}, Color{G: 1}, true)
	if len(r.verts) != 4 {
		t.Fatalf("expected 4 vertices, got %d", len(r.verts))
	}
	// Top-left (idx 0) and top-right (idx 1) should carry t=0 in U.
	// Bottom-right (idx 2) and bottom-left (idx 3) should carry t=1 in U.
	if r.verts[0].U != 0 || r.verts[1].U != 0 {
		t.Fatalf("vertical gradient: expected U=0 at top vertices, got TL=%v TR=%v",
			r.verts[0].U, r.verts[1].U)
	}
	if r.verts[2].U != 1 || r.verts[3].U != 1 {
		t.Fatalf("vertical gradient: expected U=1 at bottom vertices, got BR=%v BL=%v",
			r.verts[2].U, r.verts[3].U)
	}
}

// TestFillGradientRectSameStopsKeepsBatch: two consecutive
// FillGradientRect calls with identical stop colours must NOT trigger a
// flush — the second quad's vertices are appended onto the first batch.
// This is what makes the gradient fill cheap when many widgets reuse the
// same brush (e.g. a row of identical buttons).
//
// We can verify this without GL: when no flush fires, len(r.verts) keeps
// growing across calls and len(r.indices) tracks both quads.
func TestFillGradientRectSameStopsKeepsBatch(t *testing.T) {
	r := newAdapterTestRenderer()
	r.FillGradientRect(Rect{X: 0, Y: 0, W: 10, H: 10}, Color{R: 1}, Color{G: 1}, false)
	r.FillGradientRect(Rect{X: 20, Y: 0, W: 10, H: 10}, Color{R: 1}, Color{G: 1}, false)
	if got := len(r.verts); got != 8 {
		t.Fatalf("two same-stop quads should batch: expected 8 verts, got %d", got)
	}
	if got := len(r.indices); got != 12 {
		t.Fatalf("two same-stop quads: expected 12 indices, got %d", got)
	}
}

// TestCairoCompatLinearGradientBrushSetsState: SetBrush with a
// LinearGradient activates gradient mode and stores the first/last stops.
func TestCairoCompatLinearGradientBrushSetsState(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 100, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	if !c.gradientActive {
		t.Fatalf("after SetBrush(LinearGradient) gradientActive = false")
	}
	if c.gradientStart.R != 255 {
		t.Fatalf("gradientStart = %+v, want first stop (red)", c.gradientStart)
	}
	if c.gradientEnd.B != 255 {
		t.Fatalf("gradientEnd = %+v, want last stop (blue)", c.gradientEnd)
	}
	if c.gradientVertical {
		t.Fatalf("axis (0,0)-(100,0) should resolve as horizontal, got vertical")
	}
}

// TestCairoCompatLinearGradientVerticalAxis: an axis with |dy| > |dx|
// resolves as vertical.
func TestCairoCompatLinearGradientVerticalAxis(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 0, 50)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{G: 255, A: 255})
	c.SetBrush(g)
	if !c.gradientVertical {
		t.Fatalf("axis (0,0)-(0,50) should resolve as vertical")
	}
}

// TestCairoCompatLinearGradientMultiStopCollapsesToEnds: a 4-stop gradient
// keeps only the first and last stop. This is the documented limitation;
// the test pins it so a future regression is visible.
func TestCairoCompatLinearGradientMultiStopCollapsesToEnds(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 100, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(0.33, paint.Color{G: 255, A: 255})
	g.AddStop(0.66, paint.Color{B: 255, A: 255})
	g.AddStop(1, paint.Color{R: 255, G: 255, A: 255})
	c.SetBrush(g)
	if c.gradientStart.R != 255 || c.gradientStart.G != 0 {
		t.Fatalf("multi-stop start collapsed wrong: %+v", c.gradientStart)
	}
	if c.gradientEnd.R != 255 || c.gradientEnd.G != 255 {
		t.Fatalf("multi-stop end collapsed wrong: %+v", c.gradientEnd)
	}
}

// TestCairoCompatSetBrush1ClearsGradient: switching back to a solid brush
// must clear the gradient flag, so subsequent fills take the solid path.
func TestCairoCompatSetBrush1ClearsGradient(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 10, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)
	c.SetBrush1(paint.Color{G: 255, A: 255})
	if c.gradientActive {
		t.Fatalf("SetBrush1 did not clear gradientActive")
	}
}

// TestCairoCompatSingleStopGradientAsSolid: a one-stop gradient is
// treated as a solid brush (gradientActive stays false, brushColor mirrors
// the single stop). Many widgets build gradients incrementally and we
// must not render a half-built gradient as transparent.
func TestCairoCompatSingleStopGradientAsSolid(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 10, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	c.SetBrush(g)
	if c.gradientActive {
		t.Fatalf("single-stop gradient should not activate gradient mode")
	}
	if c.brushColor.R != 255 {
		t.Fatalf("single-stop gradient should set solid brush to that stop, got %+v", c.brushColor)
	}
}

// TestCairoCompatGradientFillRectGoesGPU: with an active linear gradient
// brush, filling a rect routes through Renderer.FillGradientRect (kind
// becomes kindGradient and gradStart/gradEnd are the converted stops).
func TestCairoCompatGradientFillRectGoesGPU(t *testing.T) {
	c, r := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 100, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)
	c.Rectangle(0, 0, 100, 100)
	c.Fill()

	if r.curKind != kindGradient {
		t.Fatalf("after gradient Fill curKind = %v, want kindGradient", r.curKind)
	}
	// Start stop: red, A=255 → R=1, G=0, B=0, A=1 in normalised colour.
	if r.gradStart.R == 0 || r.gradEnd.B == 0 {
		t.Fatalf("gradient stops not stored on renderer: start=%+v end=%+v", r.gradStart, r.gradEnd)
	}
}

// TestCairoCompatGradientFillNonRectFallsBackSolid: a non-axis-aligned
// path with an active gradient falls back to solid triangulation using
// the start stop. The rendered geometry should still be non-empty so the
// shape is visible.
func TestCairoCompatGradientFillNonRectFallsBackSolid(t *testing.T) {
	c, r := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 100, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	// Triangle (3 unique points) — should not match the rect detector.
	c.MoveTo(0, 0)
	c.LineTo(50, 0)
	c.LineTo(25, 50)
	c.LineTo(0, 0)
	c.Fill()

	if r.curKind == kindGradient {
		t.Fatalf("non-rect path with gradient should fall back, but curKind = kindGradient")
	}
	if len(r.indices) == 0 {
		t.Fatalf("non-rect gradient fallback emitted no geometry")
	}
}

// TestCairoCompatGradientStateScopedBySaveRestore: Save snapshots the
// gradient state; Restore brings it back. Without this, a child widget
// that switches to a solid brush would permanently clobber its parent's
// gradient.
func TestCairoCompatGradientStateScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewLinearGradient(0, 0, 10, 0)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	c.Save()
	c.SetBrush1(paint.Color{G: 255, A: 255})
	if c.gradientActive {
		t.Fatalf("inside Save scope SetBrush1 did not clear gradientActive")
	}
	c.Restore()
	if !c.gradientActive {
		t.Fatalf("Restore did not bring back the gradient brush")
	}
	if c.gradientStart.R != 255 {
		t.Fatalf("Restore did not bring back gradientStart, got %+v", c.gradientStart)
	}
}

// TestSingleAxisAlignedRectPathDetector: pin the four key cases of the
// detector.
func TestSingleAxisAlignedRectPathDetector(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Case 1: Rectangle() builds the canonical 5-point closed form. Detect.
	c.Rectangle(10, 20, 30, 40)
	if rc, ok := c.singleAxisAlignedRectPath(); !ok ||
		rc.X != 10 || rc.Y != 20 || rc.W != 30 || rc.H != 40 {
		t.Fatalf("Rectangle path detector failed: rc=%+v ok=%v", rc, ok)
	}

	// Case 2: triangle (3 pts) → not a rect.
	c.pathPts = c.pathPts[:0]
	c.pathSubs = c.pathSubs[:0]
	c.MoveTo(0, 0)
	c.LineTo(10, 0)
	c.LineTo(5, 10)
	if _, ok := c.singleAxisAlignedRectPath(); ok {
		t.Fatalf("triangle wrongly detected as rect")
	}

	// Case 3: quad with a non-axis-aligned vertex (rotated 30°) → not a rect.
	c.pathPts = c.pathPts[:0]
	c.pathSubs = c.pathSubs[:0]
	c.MoveTo(0, 0)
	c.LineTo(10, 0)
	c.LineTo(10, 10)
	c.LineTo(2, 11) // not at minY/maxY
	if _, ok := c.singleAxisAlignedRectPath(); ok {
		t.Fatalf("non-axis-aligned quad wrongly detected as rect")
	}

	// Case 4: two sub-paths → not a single rect.
	c.pathPts = c.pathPts[:0]
	c.pathSubs = c.pathSubs[:0]
	c.Rectangle(0, 0, 10, 10)
	c.Rectangle(20, 20, 10, 10)
	if _, ok := c.singleAxisAlignedRectPath(); ok {
		t.Fatalf("two-rect path wrongly detected as a single rect")
	}
}
