package ged

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
)

// Linear layouts use ordered siblings in the canvas, saved document and both
// generated runtime forms. The slot is absolute in the tree, while the hint
// ordinal refers only to actual layout widgets.
type linearPaletteInsertion struct {
	valid          bool
	index, ordinal int
	marker         geom.Rect // zero axis thickness; stroked in scene space
}

func linearPaletteSlot(parent *FakeWidget, visible geom.Rect, x, y float64) linearPaletteInsertion {
	return linearLayoutSlot(parent, visible, x, y, parent.Children(), nil)
}

func linearLayoutSlot(parent *FakeWidget, visible geom.Rect, x, y float64, children []graph.IItem, omitted graph.IItem) linearPaletteInsertion {
	horizontal := parent.factoryName == string(LayoutHorizontal)
	if !horizontal && parent.factoryName != string(LayoutVertical) {
		return linearPaletteInsertion{}
	}
	if visible.IsEmpty() {
		return linearPaletteInsertion{}
	}
	pointer, lo, hi := y, visible.Y, visible.Bottom()
	crossLo, crossHi := visible.X, visible.Right()
	if horizontal {
		pointer, lo, hi = x, visible.X, visible.Right()
		crossLo, crossHi = visible.Y, visible.Bottom()
	}
	ox, oy := graph.ParentOrigin(parent)
	out := linearPaletteInsertion{valid: true, ordinal: 1}
	edge := lo
	previous := false
	for _, c := range children {
		if c == omitted {
			continue
		}
		child, ok := c.(*FakeWidget)
		if !ok {
			out.index++
			continue
		}
		bounds := child.Bounds1()
		start, size := oy+bounds.Y, bounds.Height
		if horizontal {
			start, size = ox+bounds.X, bounds.Width
		}
		if !finiteDropCoordinate(start+size) || size <= 0 {
			out.index++
			out.ordinal++
			continue
		}
		if pointer < start+size/2 {
			if previous && edge <= start {
				edge += (start - edge) / 2
			} else {
				edge = start
			}
			break
		}
		previous = true
		edge = start + size
		out.index++
		out.ordinal++
	}
	// Clip the insertion line to the same visible ancestor intersection as the
	// drop. Insetting keeps the stroke legible even at the parent's outer edge.
	axisInset := math.Min(1, (hi-lo)/4)
	crossInset := math.Min(2, (crossHi-crossLo)/4)
	edge = math.Max(lo+axisInset, math.Min(edge, hi-axisInset))
	crossLo += crossInset
	crossHi -= crossInset
	if horizontal {
		out.marker = geom.Rect{X: edge, Y: crossLo, Height: crossHi - crossLo}
	} else {
		out.marker = geom.Rect{X: crossLo, Y: edge, Width: crossHi - crossLo}
	}
	return out
}
