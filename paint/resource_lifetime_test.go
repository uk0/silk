package paint

import "testing"

func TestOwnedNativeResourcesReleaseWithoutGC(t *testing.T) {
	s := NewPixmap(16, 16)
	g := s.NewPainter()
	native := g.(*cairoPainter)
	g.SetBrush1(Color{10, 20, 30, 255})
	g.Rectangle(0, 0, 16, 16)
	g.Fill()
	beforePainters := cairoPainterCount.Load()
	ClosePainter(g)
	if native.cairo != nil || cairoPainterCount.Load() > beforePainters-1 {
		t.Fatal("context did not release synchronously")
	}
	ClosePainter(g)
	beforeSurfaces := cairoSurfaceCount.Load()
	CloseSurface(s)
	if s.Surface != nil || cairoSurfaceCount.Load() > beforeSurfaces-1 {
		t.Fatal("surface did not release synchronously")
	}
	CloseSurface(s)
	ClosePainter(nil)
	CloseSurface(nil)
	ClosePainter(&struct{ Painter }{})
	CloseSurface(&struct{ Surface }{})
	// A finalizer racing explicit release is forbidden by ownership, but its
	// shared release routine still needs to tolerate an already retired object.
	native.releaseNative()
	s.releaseNative()
}
