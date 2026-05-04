// Package atlas provides a dynamic 2D bin packer for glyph and icon
// atlases used by the glui renderer.
//
// The packer uses a simple skyline algorithm — fast enough for the
// thousands of glyphs a typical app needs while keeping locality good
// enough that updates stream cleanly to the GPU.
package atlas

// Region is a rectangular region inside the atlas: pixels (X,Y) of size W×H.
type Region struct {
	X, Y, W, H int
}

// Atlas is a CPU-side bin packer. The actual GPU texture is owned by the
// caller and updated via TexSubImage2D when a new region is reserved.
type Atlas struct {
	width, height int
	skyline       []skylineNode
}

type skylineNode struct {
	x, y, width int
}

// New creates an atlas of the given size in pixels. Both dimensions
// must be powers of two for best texture compatibility.
func New(width, height int) *Atlas {
	return &Atlas{
		width:   width,
		height:  height,
		skyline: []skylineNode{{0, 0, width}},
	}
}

// Width returns the atlas width in pixels.
func (a *Atlas) Width() int { return a.width }

// Height returns the atlas height in pixels.
func (a *Atlas) Height() int { return a.height }

// Pack reserves a region of size (w, h) and returns its placement.
// Returns ok=false when no space remains.
func (a *Atlas) Pack(w, h int) (Region, bool) {
	if w > a.width || h > a.height {
		return Region{}, false
	}

	bestY := -1
	bestX := -1
	bestI := -1
	bestWasted := -1

	// Find the placement that wastes the least vertical space.
	for i := 0; i < len(a.skyline); i++ {
		y, ok := a.fit(i, w, h)
		if !ok {
			continue
		}
		wasted := y - a.skyline[i].y
		if bestY < 0 || y < bestY || (y == bestY && wasted < bestWasted) {
			bestY = y
			bestX = a.skyline[i].x
			bestI = i
			bestWasted = wasted
		}
	}

	if bestI < 0 {
		return Region{}, false
	}

	a.addNode(bestI, bestX, bestY, w, h)
	return Region{X: bestX, Y: bestY, W: w, H: h}, true
}

// fit tests whether a (w, h) region fits starting at skyline node i.
// Returns the y-coordinate at which it would sit, or false.
func (a *Atlas) fit(i, w, h int) (int, bool) {
	x := a.skyline[i].x
	if x+w > a.width {
		return 0, false
	}

	y := a.skyline[i].y
	remaining := w
	j := i
	for remaining > 0 {
		if j >= len(a.skyline) {
			return 0, false
		}
		if a.skyline[j].y > y {
			y = a.skyline[j].y
		}
		if y+h > a.height {
			return 0, false
		}
		remaining -= a.skyline[j].width
		j++
	}
	return y, true
}

// addNode inserts a new skyline segment and removes any segments it overlaps.
func (a *Atlas) addNode(i, x, y, w, h int) {
	newNode := skylineNode{x: x, y: y + h, width: w}
	a.skyline = append(a.skyline[:i+1], a.skyline[i:]...)
	a.skyline[i] = newNode

	// Remove segments that fall completely under the new node.
	for j := i + 1; j < len(a.skyline); {
		prev := a.skyline[j-1]
		cur := a.skyline[j]
		if cur.x < prev.x+prev.width {
			shrink := prev.x + prev.width - cur.x
			cur.x += shrink
			cur.width -= shrink
			if cur.width <= 0 {
				a.skyline = append(a.skyline[:j], a.skyline[j+1:]...)
				continue
			}
			a.skyline[j] = cur
		}
		break
	}

	// Merge adjacent segments at the same height.
	for j := 0; j+1 < len(a.skyline); {
		if a.skyline[j].y == a.skyline[j+1].y {
			a.skyline[j].width += a.skyline[j+1].width
			a.skyline = append(a.skyline[:j+1], a.skyline[j+2:]...)
		} else {
			j++
		}
	}
}
