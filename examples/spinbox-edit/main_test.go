package main

import (
	"math"
	"testing"
)

func TestSpinDemoStepAndCommit(t *testing.T) {
	f := newSpinDemo()
	f.count.SelectAll()
	f.count.OnTextInput("42")
	if !f.count.Commit() || f.count.Value() != 42 || f.changes != 1 {
		t.Fatal("draft not committed exactly once")
	}
	f.edge.OnMouseWheel(1, 1, 1)
	if f.edge.Value() != math.MaxInt || f.changes != 2 {
		t.Fatal("step wrapped")
	}
}
func TestSpinDemoTinyLayout(t *testing.T) {
	f := newSpinDemo()
	for _, size := range [][2]float64{{0, 0}, {8, 6}, {320, 280}, {720, 480}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, w := range f.widgets {
			b := w.Bounds1()
			if b.X < 0 || b.Y < 0 || b.Width < 0 || b.Height < 0 || b.Right() > size[0] || b.Bottom() > size[1] {
				t.Fatal("child escaped", b)
			}
		}
	}
}
