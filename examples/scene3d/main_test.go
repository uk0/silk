package main

import "testing"

func TestDemoToolbarReflowsWithoutOverlap(t *testing.T) {
	f := newDemo("assembly.gltf")
	defer f.cancel()
	defer f.view.Close()
	for _, width := range []float64{280, 500, 700, 980} {
		f.SetSize(width, 680)
		f.OnResize()
		for i, b := range f.buttons {
			if b.X() < 0 || b.X()+b.Width() > width || b.Y()+b.Height() > f.view.Y() {
				t.Fatal("button outside toolbar", width, i, b.X(), b.Y(), b.Width(), b.Height())
			}
			for j, c := range f.buttons {
				if j <= i {
					continue
				}
				if b.X() < c.X()+c.Width() && c.X() < b.X()+b.Width() && b.Y() < c.Y()+c.Height() && c.Y() < b.Y()+b.Height() {
					t.Fatal("overlapping buttons", width, i, j)
				}
			}
		}
		if f.view.Y()+f.view.Height() > f.status.Y() || f.status.Y()+f.status.Height() > f.notice.Y() {
			t.Fatal("overlapping status rows")
		}
	}
}
