package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestDispatchDemoLayoutContainsControls(t *testing.T) {
	f := newDispatchDemo()
	defer f.stop()
	for _, c := range f.widgets {
		if b, ok := c.(*gui.Button); ok && (!b.IsTextVisible() || b.Text() == "") {
			t.Fatalf("action caption hidden: %q", b.Text())
		}
	}
	for _, s := range [][2]float64{{920, 480}, {320, 240}, {0, 0}, {1200, 900}} {
		f.SetSize(s[0], s[1])
		f.OnResize()
		for _, c := range f.widgets {
			x, y, w, h := c.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > s[0]+1e-6 || y+h > s[1]+1e-6 {
				t.Fatal("control escaped form", x, y, w, h, s)
			}
		}
	}
}
