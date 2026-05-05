package glui

import (
	"silk/paint"
	"testing"
)

// Tests for the radial-gradient pipeline. They run off-GL: vertex/index
// accumulation in the Renderer is observable without a real OpenGL
// context. Shader compilation requires GL and is exercised by the
// standalone glui_demo when a Window is available.

// TestFillRadialGradientRectAccumulatesQuad: a single FillRadialGradientRect
// emits one quad (4 vertices, 6 indices) under kindGradientRadial, with the
// requested radii stored on the renderer for later flush().
func TestFillRadialGradientRectAccumulatesQuad(t *testing.T) {
	r := newAdapterTestRenderer()
	stops := []GradientStop{
		{Position: 0, Color: Color{R: 1, A: 1}},
		{Position: 1, Color: Color{B: 1, A: 1}},
	}
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 100, H: 100}, 50, 50, 0, 50, stops)

	if r.curKind != kindGradientRadial {
		t.Fatalf("after FillRadialGradientRect curKind = %v, want kindGradientRadial", r.curKind)
	}
	if len(r.verts) != 4 {
		t.Fatalf("expected 4 vertices for one quad, got %d", len(r.verts))
	}
	if len(r.indices) != 6 {
		t.Fatalf("expected 6 indices for one quad, got %d", len(r.indices))
	}
	if r.radR0 != 0 || r.radR1 != 50 {
		t.Fatalf("radii not stored: r0=%v r1=%v", r.radR0, r.radR1)
	}
}

// TestFillRadialGradientUVCarriesCenterOffset: each vertex's (U, V) must
// be the corner's offset from the gradient centre. The fragment shader
// takes length(v_uv) per pixel — if any corner gets the wrong offset the
// gradient distance reading will skew.
//
// Rect (0,0,100,100) centred at (50,50) — top-left = (-50,-50), top-right =
// (50,-50), bottom-right = (50,50), bottom-left = (-50,50).
func TestFillRadialGradientUVCarriesCenterOffset(t *testing.T) {
	r := newAdapterTestRenderer()
	stops := []GradientStop{
		{Position: 0, Color: Color{R: 1, A: 1}},
		{Position: 1, Color: Color{B: 1, A: 1}},
	}
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 100, H: 100}, 50, 50, 0, 50, stops)
	if len(r.verts) != 4 {
		t.Fatalf("expected 4 vertices, got %d", len(r.verts))
	}

	want := [4][2]float32{
		{-50, -50}, // TL
		{50, -50},  // TR
		{50, 50},   // BR
		{-50, 50},  // BL
	}
	for i, w := range want {
		if r.verts[i].U != w[0] || r.verts[i].V != w[1] {
			t.Errorf("vertex %d UV = (%v,%v), want (%v,%v)",
				i, r.verts[i].U, r.verts[i].V, w[0], w[1])
		}
	}
}

// TestFillRadialGradientSameParamsKeepsBatch: two consecutive calls with
// identical centre/radii/stops batch into one draw — len(r.verts) grows
// past 4 without an interim flush. Same-parameter batching is what makes
// e.g. a row of identical avatar discs cheap.
func TestFillRadialGradientSameParamsKeepsBatch(t *testing.T) {
	r := newAdapterTestRenderer()
	stops := []GradientStop{
		{Position: 0, Color: Color{R: 1, A: 1}},
		{Position: 1, Color: Color{B: 1, A: 1}},
	}
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 50, H: 50}, 25, 25, 0, 25, stops)
	r.FillRadialGradientRect(Rect{X: 60, Y: 0, W: 50, H: 50}, 85, 25, 0, 25, stops)
	if got := len(r.verts); got != 8 {
		t.Fatalf("two same-radii radials should batch: got %d verts", got)
	}
	if got := len(r.indices); got != 12 {
		t.Fatalf("two same-radii radials: got %d indices", got)
	}
}

// TestFillRadialGradientDifferentRadiiFlushes: changing R1 between calls
// must force a flush — uniforms are global per program, so the second
// quad cannot batch with the first. Verified indirectly: the flush nuke
// the prior verts (drained into a draw call we don't observe in a
// ctx==nil test renderer) so r.verts reads back as just the second quad.
//
// Note: the ctx==nil branch in flush() returns early without uploading,
// but it still resets r.verts/r.indices, which is the observable effect
// the test pins.
func TestFillRadialGradientDifferentRadiiFlushes(t *testing.T) {
	r := newAdapterTestRenderer()
	stops := []GradientStop{
		{Position: 0, Color: Color{R: 1, A: 1}},
		{Position: 1, Color: Color{B: 1, A: 1}},
	}
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 50, H: 50}, 25, 25, 0, 25, stops)
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 50, H: 50}, 25, 25, 0, 50, stops) // r1 changed
	// Without a flush, len would be 8. With the radii change forcing a
	// flush before the second quad is emitted, only the second quad
	// remains in the buffer.
	if got := len(r.verts); got != 4 {
		t.Fatalf("radii change should flush prior batch: got %d verts, want 4", got)
	}
	if r.radR1 != 50 {
		t.Fatalf("r1 not updated after flush: %v", r.radR1)
	}
}

// TestFillRadialGradientSingleStopFallsBack: a single-stop call routes
// through FillRect (curKind = kindRect) — the fragment shader needs at
// least two stops to interpolate, so one-stop input is just a solid fill.
func TestFillRadialGradientSingleStopFallsBack(t *testing.T) {
	r := newAdapterTestRenderer()
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 10, H: 10}, 5, 5, 0, 5,
		[]GradientStop{{Position: 0, Color: Color{R: 1, A: 1}}})
	if r.curKind != kindRect {
		t.Fatalf("single-stop radial should route to kindRect, got %v", r.curKind)
	}
}

// TestFillRadialGradientEmptyStops: zero-stop input emits no geometry.
func TestFillRadialGradientEmptyStops(t *testing.T) {
	r := newAdapterTestRenderer()
	r.FillRadialGradientRect(Rect{X: 0, Y: 0, W: 10, H: 10}, 5, 5, 0, 5, nil)
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Fatalf("empty stops should emit nothing, got %d v / %d i", len(r.verts), len(r.indices))
	}
}

// TestCairoCompatRadialGradientBrushSetsState: SetBrush with a
// RadialGradient activates radial mode and snapshots the centre, radii
// and stop list.
func TestCairoCompatRadialGradientBrushSetsState(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewRadialGradient(50, 50, 0, 25)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	if !c.radialActive {
		t.Fatalf("after SetBrush(RadialGradient) radialActive = false")
	}
	if c.radialCx != 50 || c.radialCy != 50 {
		t.Fatalf("radial centre = (%v,%v), want (50,50)", c.radialCx, c.radialCy)
	}
	if c.radialR0 != 0 || c.radialR1 != 25 {
		t.Fatalf("radii = (%v,%v), want (0,25)", c.radialR0, c.radialR1)
	}
	if len(c.radialStops) != 2 {
		t.Fatalf("radialStops len = %d, want 2", len(c.radialStops))
	}
	// Linear gradient flag must NOT be active when a radial brush is set —
	// fillCurrentPath checks both flags in order, and the linear branch
	// stealing the path would silently fall back to a 2-stop linear fill.
	if c.gradientActive {
		t.Fatalf("radial brush wrongly activated linear gradientActive flag")
	}
}

// TestCairoCompatRadialGradientFillRectGoesGPU: with an active radial
// gradient brush, filling a rect routes through Renderer.FillRadialGradientRect
// (curKind becomes kindGradientRadial).
func TestCairoCompatRadialGradientFillRectGoesGPU(t *testing.T) {
	c, r := newCompatTestPainter(t)
	g := paint.NewRadialGradient(50, 50, 0, 50)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)
	c.Rectangle(0, 0, 100, 100)
	c.Fill()

	if r.curKind != kindGradientRadial {
		t.Fatalf("after radial Fill curKind = %v, want kindGradientRadial", r.curKind)
	}
	if r.radR1 != 50 {
		t.Fatalf("renderer r1 = %v, want 50", r.radR1)
	}
}

// TestCairoCompatRadialGradientNonRectFallsBackSolid: a non-axis-aligned
// path with an active radial gradient falls back to solid triangulation
// using the inner stop colour. The rendered geometry should still be
// non-empty.
func TestCairoCompatRadialGradientNonRectFallsBackSolid(t *testing.T) {
	c, r := newCompatTestPainter(t)
	g := paint.NewRadialGradient(25, 25, 0, 25)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	c.MoveTo(0, 0)
	c.LineTo(50, 0)
	c.LineTo(25, 50)
	c.LineTo(0, 0)
	c.Fill()

	if r.curKind == kindGradientRadial {
		t.Fatalf("non-rect path with radial gradient should fall back, got curKind = kindGradientRadial")
	}
	if len(r.indices) == 0 {
		t.Fatalf("non-rect radial fallback emitted no geometry")
	}
}

// TestCairoCompatRadialGradientStateScopedBySaveRestore: Save/Restore
// must preserve the radial gradient brush across nested scopes.
func TestCairoCompatRadialGradientStateScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	g := paint.NewRadialGradient(50, 50, 0, 25)
	g.AddStop(0, paint.Color{R: 255, A: 255})
	g.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(g)

	c.Save()
	c.SetBrush1(paint.Color{G: 255, A: 255})
	if c.radialActive {
		t.Fatalf("inside Save scope SetBrush1 did not clear radialActive")
	}
	c.Restore()
	if !c.radialActive {
		t.Fatalf("Restore did not bring back the radial brush")
	}
	if c.radialR1 != 25 {
		t.Fatalf("Restore did not bring back radialR1, got %v", c.radialR1)
	}
}

// TestCairoCompatRadialAndLinearAreMutuallyExclusive: switching from a
// linear gradient to a radial one (and vice versa) must clear the OTHER
// flag, otherwise fillCurrentPath could match both paths and double-emit.
func TestCairoCompatRadialAndLinearAreMutuallyExclusive(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	lin := paint.NewLinearGradient(0, 0, 10, 0)
	lin.AddStop(0, paint.Color{R: 255, A: 255})
	lin.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(lin)
	if !c.gradientActive || c.radialActive {
		t.Fatalf("after linear: flags g=%v r=%v, want g=true r=false", c.gradientActive, c.radialActive)
	}

	rad := paint.NewRadialGradient(50, 50, 0, 25)
	rad.AddStop(0, paint.Color{R: 255, A: 255})
	rad.AddStop(1, paint.Color{B: 255, A: 255})
	c.SetBrush(rad)
	if c.gradientActive || !c.radialActive {
		t.Fatalf("after radial: flags g=%v r=%v, want g=false r=true", c.gradientActive, c.radialActive)
	}

	c.SetBrush(lin)
	if !c.gradientActive || c.radialActive {
		t.Fatalf("re-set linear: flags g=%v r=%v, want g=true r=false", c.gradientActive, c.radialActive)
	}
}
