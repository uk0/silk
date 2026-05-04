// Package path provides 2D path construction and triangulation for glui.
//
// Paths are sequences of move/line/curve commands. Closed paths are filled
// using ear-clipping triangulation. Open paths can be stroked into a strip
// of triangles approximating the line with miter or bevel joins.
package path

import "math"

// Path is a builder for a 2D path.
type Path struct {
	pts   [][2]float32 // flattened points; sub-path boundaries marked in subs
	subs  []int        // indices into pts where each sub-path starts
	curX  float32
	curY  float32
}

// New creates an empty path.
func New() *Path {
	return &Path{}
}

// MoveTo starts a new sub-path at (x, y).
func (p *Path) MoveTo(x, y float32) {
	p.subs = append(p.subs, len(p.pts))
	p.pts = append(p.pts, [2]float32{x, y})
	p.curX, p.curY = x, y
}

// LineTo adds a straight segment from the current point to (x, y).
func (p *Path) LineTo(x, y float32) {
	p.pts = append(p.pts, [2]float32{x, y})
	p.curX, p.curY = x, y
}

// QuadTo adds a quadratic Bezier from the current point through control
// (cx, cy) to (x, y), flattened into 8 line segments.
func (p *Path) QuadTo(cx, cy, x, y float32) {
	const steps = 8
	for i := 1; i <= steps; i++ {
		t := float32(i) / steps
		omt := 1 - t
		bx := omt*omt*p.curX + 2*omt*t*cx + t*t*x
		by := omt*omt*p.curY + 2*omt*t*cy + t*t*y
		p.pts = append(p.pts, [2]float32{bx, by})
	}
	p.curX, p.curY = x, y
}

// Arc approximates a circular arc from angle a0 to a1 (radians, CCW)
// centered at (cx, cy) with radius r. Flattened to 16 segments per quarter.
func (p *Path) Arc(cx, cy, r, a0, a1 float32) {
	span := a1 - a0
	if span < 0 {
		span = -span
	}
	steps := int(span/(math.Pi/8)) + 1
	if steps < 4 {
		steps = 4
	}
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		a := a0 + (a1-a0)*t
		x := cx + r*float32(math.Cos(float64(a)))
		y := cy + r*float32(math.Sin(float64(a)))
		p.pts = append(p.pts, [2]float32{x, y})
	}
	if steps > 0 {
		end := p.pts[len(p.pts)-1]
		p.curX, p.curY = end[0], end[1]
	}
}

// Close appends a segment back to the sub-path's first point.
func (p *Path) Close() {
	if len(p.subs) == 0 {
		return
	}
	first := p.pts[p.subs[len(p.subs)-1]]
	p.pts = append(p.pts, first)
	p.curX, p.curY = first[0], first[1]
}

// Points returns the flattened point list. Triangulators consume this.
func (p *Path) Points() [][2]float32 { return p.pts }

// SubPaths returns the indices marking sub-path boundaries.
func (p *Path) SubPaths() []int { return p.subs }

// Reset clears the path so it can be reused without reallocation.
func (p *Path) Reset() {
	p.pts = p.pts[:0]
	p.subs = p.subs[:0]
	p.curX = 0
	p.curY = 0
}
