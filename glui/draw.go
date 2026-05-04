package glui

import "math"

// FillRoundedRect paints a solid rectangle with all four corners rounded
// to the given radius. Anti-aliasing is handled in the shader via SDF.
func (r *Renderer) FillRoundedRect(rc Rect, radius float32, col Color) {
	if radius <= 0 {
		r.FillRect(rc, col)
		return
	}
	r.setBatch(kindRect, 0)
	hw, hh := rc.W*0.5, rc.H*0.5
	r.pushQuad(rc.X, rc.Y, rc.W, rc.H, -hw, -hh, hw, hh, col)
}

// StrokeRect paints a 1px outlined rectangle by drawing four thin solid
// quads. For more general strokes use Path.Stroke().
func (r *Renderer) StrokeRect(rc Rect, width float32, col Color) {
	if width <= 0 {
		return
	}
	r.FillRect(Rect{rc.X, rc.Y, rc.W, width}, col)                       // top
	r.FillRect(Rect{rc.X, rc.Y + rc.H - width, rc.W, width}, col)        // bottom
	r.FillRect(Rect{rc.X, rc.Y, width, rc.H}, col)                       // left
	r.FillRect(Rect{rc.X + rc.W - width, rc.Y, width, rc.H}, col)        // right
}

// FillCircle paints a filled circle at (cx, cy) with the given radius.
// Implemented as a rounded rect with radius = side/2; the SDF shader
// produces a perfect circle.
func (r *Renderer) FillCircle(cx, cy, radius float32, col Color) {
	r.FillRoundedRect(Rect{cx - radius, cy - radius, radius * 2, radius * 2}, radius, col)
}

// Line draws a 1-px-thick line between two points using two triangles.
// For thicker / anti-aliased strokes, use Path.
func (r *Renderer) Line(x0, y0, x1, y1, width float32, col Color) {
	if width <= 0 {
		return
	}
	dx := x1 - x0
	dy := y1 - y0
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length == 0 {
		return
	}
	// Perpendicular unit vector × half-width.
	hw := width * 0.5
	px := -dy / length * hw
	py := dx / length * hw

	// Four corners of the line quad.
	r.setBatch(kindRect, 0)
	base := uint16(len(r.verts))
	cs := [4][2]float32{
		{x0 + px, y0 + py},
		{x1 + px, y1 + py},
		{x1 - px, y1 - py},
		{x0 - px, y0 - py},
	}
	for _, c := range cs {
		cx, cy := r.project(c[0], c[1])
		r.verts = append(r.verts, vertex{cx, cy, 0, 0, col.R, col.G, col.B, col.A})
	}
	r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
}

// FillTriangle paints a filled triangle.
func (r *Renderer) FillTriangle(x0, y0, x1, y1, x2, y2 float32, col Color) {
	r.setBatch(kindPath, 0)
	base := uint16(len(r.verts))
	for _, p := range [3][2]float32{{x0, y0}, {x1, y1}, {x2, y2}} {
		cx, cy := r.project(p[0], p[1])
		r.verts = append(r.verts, vertex{cx, cy, 0, 0, col.R, col.G, col.B, col.A})
	}
	r.indices = append(r.indices, base, base+1, base+2)
}
