package main

import "testing"

func TestFormatterDemoControlsRemainContained(t *testing.T) {
	f := newFormatterDemo()
	defer f.editor.CancelGitStatus()
	for _, size := range [][2]float64{{980, 620}, {480, 350}, {180, 100}, {0, 0}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, c := range f.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0]+0.01 || y+h > size[1]+0.01 {
				t.Fatal(size, x, y, w, h)
			}
		}
	}
}
