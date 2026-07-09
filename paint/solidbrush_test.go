package paint

import "testing"

// TestSetBrush1NoAlloc proves the color setter every widget calls before a fill
// no longer allocates a *SolidBrush per call.
func TestSetBrush1NoAlloc(t *testing.T) {
	pm := NewPixmap(8, 8)
	g := pm.NewPainter()
	g.SetBrush1(Color{R: 1, A: 255}) // warm up
	avg := testing.AllocsPerRun(200, func() {
		g.SetBrush1(Color{R: 9, G: 8, B: 7, A: 255})
	})
	if avg != 0 {
		t.Errorf("SetBrush1 allocs/op = %v, want 0", avg)
	}
}

// TestSetBrush1AppliesColor verifies the shared brush carries the exact color
// set, so fills paint the requested color.
func TestSetBrush1AppliesColor(t *testing.T) {
	pm := NewPixmap(8, 8)
	g := pm.NewPainter().(*cairoPainter)
	want := Color{R: 200, G: 100, B: 50, A: 255}
	g.SetBrush1(want)
	sb, ok := g.state.brush.(*SolidBrush)
	if !ok {
		t.Fatalf("state.brush is %T, want *SolidBrush", g.state.brush)
	}
	if sb.Color != want {
		t.Errorf("brush color = %+v, want %+v", sb.Color, want)
	}
}

// TestSetBrush1SaveRestoreConsistent documents and guards the semantics the
// shared brush relies on: Restore does not roll back the Go-side brush, so the
// current color after Restore is the last one set — identical to the behaviour
// before the reuse optimization.
func TestSetBrush1SaveRestoreConsistent(t *testing.T) {
	pm := NewPixmap(8, 8)
	g := pm.NewPainter().(*cairoPainter)
	red := Color{R: 255, A: 255}
	blue := Color{B: 255, A: 255}

	g.SetBrush1(red)
	g.Save()
	g.SetBrush1(blue)
	g.Restore()

	if got := g.state.brush.(*SolidBrush).Color; got != blue {
		t.Errorf("after Restore brush = %+v, want last-set %+v", got, blue)
	}
}

// TestSetPen1NoAlloc proves the stroke-color setter no longer allocates a *pen
// per call.
func TestSetPen1NoAlloc(t *testing.T) {
	pm := NewPixmap(8, 8)
	g := pm.NewPainter()
	g.SetPen1(Color{R: 1, A: 255}, 1) // warm up
	avg := testing.AllocsPerRun(200, func() {
		g.SetPen1(Color{R: 9, A: 255}, 2)
	})
	if avg != 0 {
		t.Errorf("SetPen1 allocs/op = %v, want 0", avg)
	}
}

// TestSetPen1AppliesColorWidth verifies the reused pen carries the exact color
// and width set.
func TestSetPen1AppliesColorWidth(t *testing.T) {
	pm := NewPixmap(8, 8)
	g := pm.NewPainter().(*cairoPainter)
	g.SetPen1(Color{R: 10, G: 20, B: 30, A: 255}, 3.5)
	if c := g.state.pen.Color(); c != (Color{R: 10, G: 20, B: 30, A: 255}) {
		t.Errorf("pen color = %+v, want {10 20 30 255}", c)
	}
	if w := g.state.pen.Width(); w != 3.5 {
		t.Errorf("pen width = %v, want 3.5", w)
	}
}
