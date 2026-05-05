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
//
// Dash, when non-empty, switches the polyline to a dash-aware path that
// breaks every segment into on/off pieces of the lengths listed. The pattern
// loops as the cursor advances along the polyline, so the gaps stay consistent
// across joins. Empty Dash (the zero value) renders a fully solid stroke —
// the historical behaviour.
type StrokeStyle struct {
	Width      float32
	Color      Color
	Join       StrokeJoin
	Cap        StrokeCap
	MiterLimit float32 // default: 10 — switch to bevel beyond this
	Dash       []float32 // alternating on/off lengths in points; nil = solid
	DashOffset float32   // initial phase along the dash pattern
}

// Polyline strokes a sequence of points with join handling. For closed
// shapes, repeat the first point at the end.
//
// When style.Dash is non-empty, the call routes to PolylineDashed which
// emits a separate quad per on-piece. Joins are skipped on the dashed path
// because a join only makes sense between two contiguous on-pieces and the
// cursor crossing a segment boundary while inside a dash is best handled by
// just continuing the dash into the next segment as a fresh quad.
func (r *Renderer) Polyline(points [][2]float32, style StrokeStyle) {
	if len(points) < 2 || style.Width <= 0 {
		return
	}
	if style.MiterLimit == 0 {
		style.MiterLimit = 10
	}
	if len(style.Dash) > 0 {
		r.PolylineDashed(points, style)
		return
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

// PolylineDashed strokes points using style.Dash as an alternating on/off
// length pattern (in points). The first entry is "on", the second "off",
// then it loops. style.DashOffset advances the initial phase so the pattern
// can be shifted without changing the array.
//
// Algorithm: walk along the polyline maintaining a remaining-distance cursor
// inside the current dash entry. While "on", accumulate sub-segment endpoints
// and emit a regular line quad each time we cross from on→off (or hit the
// end of the polyline). Phase persists across vertices so the pattern looks
// continuous through joins, matching Cairo's dash semantics.
//
// Joins are not emitted on the dashed path: a join is only meaningful between
// two contiguous on-pieces meeting at a vertex, and that case already gets
// the right look from the two adjacent quads' shared corner. Diagnosing every
// join condition correctly inside a dash run added complexity for little
// visual gain, so we keep the implementation tight.
func (r *Renderer) PolylineDashed(points [][2]float32, style StrokeStyle) {
	if len(points) < 2 || style.Width <= 0 || len(style.Dash) == 0 {
		return
	}
	r.setBatch(kindPath, 0)
	hw := style.Width * 0.5
	col := style.Color

	// Position the cursor inside the dash pattern. Negative DashOffset is
	// allowed; we wrap the offset into [0, totalLen) so the rest of the
	// algorithm only needs to look forward.
	totalLen := float32(0)
	for _, d := range style.Dash {
		if d > 0 {
			totalLen += d
		}
	}
	if totalLen <= 0 {
		// Pathological: every entry zero. Avoid an infinite-loop later.
		return
	}
	offset := style.DashOffset
	for offset < 0 {
		offset += totalLen
	}
	for offset >= totalLen {
		offset -= totalLen
	}

	// Find the dash index and remaining distance for the starting offset.
	dashIdx := 0
	consumed := float32(0)
	for dashIdx < len(style.Dash) {
		if offset < consumed+style.Dash[dashIdx] {
			break
		}
		consumed += style.Dash[dashIdx]
		dashIdx++
	}
	if dashIdx >= len(style.Dash) {
		dashIdx = 0
	}
	remaining := style.Dash[dashIdx] - (offset - consumed)
	if remaining <= 0 {
		// Land exactly on a boundary — start of next entry.
		dashIdx = (dashIdx + 1) % len(style.Dash)
		remaining = style.Dash[dashIdx]
	}
	// "on" if dashIdx is even (entries 0, 2, 4, … are draws; 1, 3, … are gaps).
	on := dashIdx%2 == 0

	// runStart marks the world-space start of the current "on" run when on==true.
	// Until we open one, runOpen stays false so we don't emit empty quads.
	var runStart [2]float32
	runOpen := false

	for i := 0; i < len(points)-1; i++ {
		x0, y0 := points[i][0], points[i][1]
		x1, y1 := points[i+1][0], points[i+1][1]

		dx := x1 - x0
		dy := y1 - y0
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length == 0 {
			continue
		}
		ux := dx / length
		uy := dy / length

		// Walk along the segment from cur (= 0) to length, peeling off the
		// current dash chunk's remaining distance until the segment is
		// exhausted. Each cross from on→off closes the quad; each cross from
		// off→on opens a new one. If a run is still open at the segment end
		// it carries through into the next segment.
		if on && !runOpen {
			runStart = [2]float32{x0, y0}
			runOpen = true
		}

		cur := float32(0)
		for cur < length {
			step := remaining
			if cur+step >= length {
				// The dash continues into (or past) the segment's end; stop
				// at the segment end, advance the dash counter, and break.
				step = length - cur
				cur = length
				remaining -= step
				if remaining <= 0 {
					// Boundary hit at the vertex; close any open "on" run
					// here and flip phase. Reusing the segment's end-point
					// as the run terminus keeps geometry contiguous.
					if on && runOpen {
						r.emitDashedQuad(runStart, [2]float32{x1, y1}, hw, col)
						runOpen = false
					}
					dashIdx = (dashIdx + 1) % len(style.Dash)
					remaining = style.Dash[dashIdx]
					on = dashIdx%2 == 0
					if on {
						runStart = [2]float32{x1, y1}
						runOpen = true
					}
				}
				break
			}

			// The dash boundary lies inside this segment — split here.
			cur += step
			remaining = 0
			endX := x0 + ux*cur
			endY := y0 + uy*cur

			if on && runOpen {
				r.emitDashedQuad(runStart, [2]float32{endX, endY}, hw, col)
				runOpen = false
			}
			// Move to the next dash entry.
			dashIdx = (dashIdx + 1) % len(style.Dash)
			remaining = style.Dash[dashIdx]
			on = dashIdx%2 == 0
			if on {
				runStart = [2]float32{endX, endY}
				runOpen = true
			}
		}
	}

	// Close any "on" run still open at the polyline's end with the last point.
	if on && runOpen {
		last := points[len(points)-1]
		r.emitDashedQuad(runStart, [2]float32{last[0], last[1]}, hw, col)
	}
}

// emitDashedQuad pushes a 4-vertex 6-index line quad from p0 to p1 with
// half-width hw. Skips zero-length runs (which would emit a degenerate
// triangle that the GPU still draws as a hairline).
func (r *Renderer) emitDashedQuad(p0, p1 [2]float32, hw float32, col Color) {
	dx := p1[0] - p0[0]
	dy := p1[1] - p0[1]
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length == 0 {
		return
	}
	nx := -dy / length
	ny := dx / length
	base := uint16(len(r.verts))
	r.appendStrokeVert(p0[0]+nx*hw, p0[1]+ny*hw, col)
	r.appendStrokeVert(p1[0]+nx*hw, p1[1]+ny*hw, col)
	r.appendStrokeVert(p1[0]-nx*hw, p1[1]-ny*hw, col)
	r.appendStrokeVert(p0[0]-nx*hw, p0[1]-ny*hw, col)
	r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
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
