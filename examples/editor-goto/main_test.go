package main

import "testing"

func TestGoToLayout(t *testing.T) {
	f := newGoToDemo()
	for _, size := range [][2]float64{{920, 640}, {480, 400}, {220, 200}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, c := range f.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatal(size, x, y, w, h)
			}
		}
	}
}
