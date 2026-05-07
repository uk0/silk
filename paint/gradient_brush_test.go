package paint

import (
	"testing"
)

// TestLinearGradientSetBrushNoWarn: handing a *LinearGradient to
// cairoPainter.SetBrush + Fill must not fall through to the default
// "type error" warn branch. The bench used to skip with that exact
// message — once this test is green, the gradient brush is part of
// the supported Painter contract.
func TestLinearGradientSetBrushNoWarn(t *testing.T) {
	pix := NewPixmap(64, 64)
	g := pix.NewPainter()

	grad := NewLinearGradient(0, 0, 64, 0)
	grad.AddStop(0, Color{255, 0, 0, 255})
	grad.AddStop(1, Color{0, 0, 255, 255})

	g.SetBrush(grad)
	g.Rectangle(0, 0, 64, 64)
	g.Fill()
}

// TestRadialGradientSetBrushNoWarn: same contract for radials.
func TestRadialGradientSetBrushNoWarn(t *testing.T) {
	pix := NewPixmap(64, 64)
	g := pix.NewPainter()

	grad := NewRadialGradient(32, 32, 0, 32)
	grad.AddStop(0, Color{255, 255, 255, 255})
	grad.AddStop(1, Color{0, 0, 0, 0})

	g.SetBrush(grad)
	g.Rectangle(0, 0, 64, 64)
	g.Fill()
}

// TestLinearGradientPatternCached: the cairo pattern build is deferred
// to first cairoPattern() call, then memoised — repeat calls without
// stop-list mutations return the same handle, so applyBrush during a
// list-row scroll doesn't pay to rebuild the pattern every frame.
func TestLinearGradientPatternCached(t *testing.T) {
	grad := NewLinearGradient(0, 0, 100, 0)
	grad.AddStop(0, Color{255, 0, 0, 255})
	grad.AddStop(1, Color{0, 255, 0, 255})

	first := grad.cairoPattern()
	if first == nil {
		t.Fatal("first cairoPattern() returned nil")
	}
	again := grad.cairoPattern()
	if again != first {
		t.Errorf("repeat cairoPattern() returned different handle: %p vs %p", first, again)
	}
}

// TestLinearGradientAddStopAfterBuildClearsCache: after the pattern has
// been built once, a fresh AddStop must drop the cached handle so the
// next build picks up the new stop list.
func TestLinearGradientAddStopAfterBuildClearsCache(t *testing.T) {
	grad := NewLinearGradient(0, 0, 100, 0)
	grad.AddStop(0, Color{255, 0, 0, 255})
	grad.AddStop(1, Color{0, 255, 0, 255})
	_ = grad.cairoPattern()
	if grad.pat == nil {
		t.Fatal("pat should be cached after first build")
	}

	grad.AddStop(0.5, Color{0, 0, 255, 255})
	if grad.pat != nil {
		t.Errorf("AddStop after build should clear cached pat (still %p)", grad.pat)
	}
}
