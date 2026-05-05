package glui

import "math"

// StrokeJoin selects how connected line segments meet.
type StrokeJoin uint8

const (
	JoinMiter StrokeJoin = iota
	JoinBevel
	JoinRound
)

// StrokeCap selects how the endpoints of a polyline are rendered.
type StrokeCap uint8

const (
	CapButt   StrokeCap = iota // square cut at the endpoint
	CapSquare                  // extends half-width past the endpoint
	CapRound                   // semicircle past the endpoint
)

// StrokeStyle parameters for Polyline().
type StrokeStyle struct {
	Width      float32
	Color      Color
	Join       StrokeJoin
	Cap        StrokeCap
	MiterLimit float32 // default: 10 — switch to bevel beyond this
}

// Polyline strokes a sequence of points with join handling. For closed
// shapes, repeat the first point at the end.
func (r *Renderer) Polyline(points [][2]float32, style StrokeStyle) {
	if len(points) < 2 || style.Width <= 0 {
		return
	}
	if style.MiterLimit == 0 {
		style.MiterLimit = 10
	}

	r.setBatch(kindPath, 0)
	hw := style.Width * 0.5
	col := style.Color

	// For each segment compute the perpendicular offset and emit a quad.
	// At each interior junction, emit a join geometry based on style.
	for i := 0; i < len(points)-1; i++ {
		x0, y0 := points[i][0], points[i][1]
		x1, y1 := points[i+1][0], points[i+1][1]

		dx := x1 - x0
		dy := y1 - y0
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length == 0 {
			continue
		}
		nx := -dy / length
		ny := dx / length

		// Quad for this segment.
		base := uint16(len(r.verts))
		r.appendStrokeVert(x0+nx*hw, y0+ny*hw, col)
		r.appendStrokeVert(x1+nx*hw, y1+ny*hw, col)
		r.appendStrokeVert(x1-nx*hw, y1-ny*hw, col)
		r.appendStrokeVert(x0-nx*hw, y0-ny*hw, col)
		r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)

		// Join with next segment.
		if i+2 < len(points) {
			x2, y2 := points[i+2][0], points[i+2][1]
			r.emitJoin(x1, y1, x0, y0, x2, y2, hw, style)
		}
	}
}

func (r *Renderer) appendStrokeVert(x, y float32, col Color) {
	cx, cy := r.project(x, y)
	r.verts = append(r.verts, vertex{cx, cy, 0, 0, col.R, col.G, col.B, col.A, 0, 0, 0, 0})
}

// emitJoin emits geometry to fill the gap between segments meeting at (x, y).
// (px, py) is the previous point; (nx, ny) is the next point.
func (r *Renderer) emitJoin(x, y, px, py, nx, ny, hw float32, style StrokeStyle) {
	// Direction from current to prev (normalized).
	dx1 := px - x
	dy1 := py - y
	l1 := float32(math.Hypot(float64(dx1), float64(dy1)))
	if l1 == 0 {
		return
	}
	dx1 /= l1
	dy1 /= l1

	// Direction from current to next (normalized).
	dx2 := nx - x
	dy2 := ny - y
	l2 := float32(math.Hypot(float64(dx2), float64(dy2)))
	if l2 == 0 {
		return
	}
	dx2 /= l2
	dy2 /= l2

	// Cross product determines which side is the outer corner.
	cross := dx1*dy2 - dy1*dx2
	if cross == 0 {
		return // collinear
	}

	// Outer perpendicular at the join: rotate the bisector 90deg.
	// We approximate as a triangle fan (bevel) or arc (round).

	switch style.Join {
	case JoinBevel:
		// Triangle from junction to two segment-corner outer points.
		// Outer perpendiculars of incoming & outgoing.
		var ipx, ipy, opx, opy float32
		if cross > 0 {
			ipx, ipy = -dy1*hw, dx1*hw // outer = +90 of incoming dir (toward x)
			opx, opy = dy2*hw, -dx2*hw // outer = -90 of outgoing dir (toward x)
		} else {
			ipx, ipy = dy1*hw, -dx1*hw
			opx, opy = -dy2*hw, dx2*hw
		}
		base := uint16(len(r.verts))
		r.appendStrokeVert(x, y, style.Color)
		r.appendStrokeVert(x+ipx, y+ipy, style.Color)
		r.appendStrokeVert(x+opx, y+opy, style.Color)
		r.indices = append(r.indices, base, base+1, base+2)

	case JoinRound:
		// Approximate the outer arc with N triangles.
		const segs = 6
		var startAng, endAng float64
		// Compute outer arc angles. For now, just approximate with bevel.
		// (Proper round join needs the outer normals' arc.)
		startAng = math.Atan2(float64(-dy1), float64(-dx1)) + math.Pi/2
		endAng = math.Atan2(float64(-dy2), float64(-dx2)) - math.Pi/2
		if cross < 0 {
			startAng, endAng = endAng, startAng
		}
		delta := (endAng - startAng) / segs
		base := uint16(len(r.verts))
		r.appendStrokeVert(x, y, style.Color)
		for j := 0; j <= segs; j++ {
			a := startAng + delta*float64(j)
			r.appendStrokeVert(
				x+float32(math.Cos(a))*hw,
				y+float32(math.Sin(a))*hw,
				style.Color,
			)
		}
		for j := 0; j < segs; j++ {
			r.indices = append(r.indices, base, base+1+uint16(j), base+2+uint16(j))
		}

	case JoinMiter:
		// Miter: extend outer edges to their intersection.
		// bisector of incoming + outgoing dirs (away from junction), scaled
		// so its perpendicular distance to either edge equals hw.
		bx := -(dx1 + dx2)
		by := -(dy1 + dy2)
		bl := float32(math.Hypot(float64(bx), float64(by)))
		if bl == 0 {
			return // 180-degree, no miter
		}
		bx /= bl
		by /= bl

		// sin(half-angle) between -dir1 and bisector
		sinHalf := -dx1*by + dy1*bx
		if sinHalf == 0 {
			return
		}
		miterLen := hw / float32(math.Abs(float64(sinHalf)))

		// Cap miter to limit (fall back to bevel-like flat).
		if miterLen > hw*style.MiterLimit {
			miterLen = hw * style.MiterLimit
		}

		var outerSign float32 = 1
		if cross < 0 {
			outerSign = -1
		}
		_ = outerSign

		mx := x + bx*miterLen
		my := y + by*miterLen

		// Triangle: junction + two segment-end outer points + miter point.
		// For simplicity emit two triangles forming the miter quad.
		var ipx, ipy, opx, opy float32
		if cross > 0 {
			ipx, ipy = -dy1*hw, dx1*hw
			opx, opy = dy2*hw, -dx2*hw
		} else {
			ipx, ipy = dy1*hw, -dx1*hw
			opx, opy = -dy2*hw, dx2*hw
		}
		base := uint16(len(r.verts))
		r.appendStrokeVert(x, y, style.Color)
		r.appendStrokeVert(x+ipx, y+ipy, style.Color)
		r.appendStrokeVert(mx, my, style.Color)
		r.appendStrokeVert(x+opx, y+opy, style.Color)
		r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
	}
}
