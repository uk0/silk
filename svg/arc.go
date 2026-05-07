package svg

import "math"

// arcSegment is one cubic Bezier slice produced by decomposeArc.
// startX/Y is the starting endpoint of the segment (= previous
// segment's endX/Y for the second and later slices). c1/c2 are the
// two cubic control points. endX/Y is the endpoint.
type arcSegment struct {
	startX, startY float64
	c1x, c1y       float64
	c2x, c2y       float64
	endX, endY     float64
}

// decomposeArc converts an SVG path "A" command into a sequence of
// cubic Bezier segments approximating the elliptical arc. The
// caller's pen position before the arc is (x0, y0); the arc lands at
// (x, y); the ellipse has radii (rx, ry); xAxisRotDeg is the angle
// (in degrees) of the ellipse's x-axis relative to the user's x-axis;
// largeArcFlag and sweepFlag are the SVG semantics.
//
// Algorithm: W3C SVG 1.1 implementation notes appendix B.2.4
// (endpoint to center parameterisation), then slice the arc into
// 90°-or-smaller pieces and approximate each piece with one cubic
// Bezier using k = 4/3 * tan(θ/4) for the control-point distance.
//
// Edge cases:
//   - rx == 0 || ry == 0: arc degenerates to a straight line; we
//     return a single segment with c1 = c2 = midpoint, equivalent
//     to a line under cubic Bezier interpolation
//   - x0 == x && y0 == y: zero-length arc, return empty slice (the
//     SVG spec says skip the segment entirely)
//   - rx or ry negative: take absolute value (per spec)
//
// Numeric stability: when the input radii can't fit the chord, scale
// them up so the arc remains well-defined (per W3C appendix B.2.5).
func decomposeArc(x0, y0, rx, ry, xAxisRotDeg float64, largeArcFlag, sweepFlag bool, x, y float64) []arcSegment {
	if x0 == x && y0 == y {
		// Zero-length arc — per spec, skip the segment.
		return nil
	}
	if rx == 0 || ry == 0 {
		// Degenerate ellipse — render as a straight line via a
		// degenerate cubic with both control points at the
		// midpoint.
		mx, my := (x0+x)*0.5, (y0+y)*0.5
		return []arcSegment{{
			startX: x0, startY: y0,
			c1x: mx, c1y: my,
			c2x: mx, c2y: my,
			endX: x, endY: y,
		}}
	}

	// Take absolute values per spec.
	rx = math.Abs(rx)
	ry = math.Abs(ry)

	phi := xAxisRotDeg * math.Pi / 180

	// Step 1: compute (x1', y1') — the endpoint pair in a coordinate
	// system rotated -phi about the midpoint.
	cosPhi := math.Cos(phi)
	sinPhi := math.Sin(phi)
	dx := (x0 - x) * 0.5
	dy := (y0 - y) * 0.5
	x1p := cosPhi*dx + sinPhi*dy
	y1p := -sinPhi*dx + cosPhi*dy

	// Step 2: ensure radii are large enough (W3C B.2.5). If not, scale
	// up uniformly so the arc fits.
	rxSq := rx * rx
	rySq := ry * ry
	x1pSq := x1p * x1p
	y1pSq := y1p * y1p
	radiiCheck := x1pSq/rxSq + y1pSq/rySq
	if radiiCheck > 1 {
		s := math.Sqrt(radiiCheck)
		rx *= s
		ry *= s
		rxSq = rx * rx
		rySq = ry * ry
	}

	// Step 3: compute (cx', cy') — center in the rotated frame.
	sign := 1.0
	if largeArcFlag == sweepFlag {
		sign = -1.0
	}
	num := rxSq*rySq - rxSq*y1pSq - rySq*x1pSq
	den := rxSq*y1pSq + rySq*x1pSq
	if num < 0 {
		// Numerical noise can drive a small positive into a small
		// negative. Clamp to zero rather than NaN.
		num = 0
	}
	coef := sign * math.Sqrt(num/den)
	cxp := coef * (rx * y1p) / ry
	cyp := coef * -(ry * x1p) / rx

	// Step 4: compute (cx, cy) in the original coordinate frame.
	cx := cosPhi*cxp - sinPhi*cyp + (x0+x)*0.5
	cy := sinPhi*cxp + cosPhi*cyp + (y0+y)*0.5

	// Step 5: compute angles theta1 and deltaTheta.
	theta1 := angleBetween(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	deltaTheta := angleBetween(
		(x1p-cxp)/rx, (y1p-cyp)/ry,
		(-x1p-cxp)/rx, (-y1p-cyp)/ry,
	)

	// Adjust deltaTheta per the sweep flag.
	if !sweepFlag && deltaTheta > 0 {
		deltaTheta -= 2 * math.Pi
	} else if sweepFlag && deltaTheta < 0 {
		deltaTheta += 2 * math.Pi
	}

	// Step 6: split into ≤90° slices and emit cubic Beziers.
	const maxSlice = math.Pi * 0.5 // 90 degrees
	numSegs := int(math.Ceil(math.Abs(deltaTheta) / maxSlice))
	if numSegs == 0 {
		numSegs = 1
	}
	dtheta := deltaTheta / float64(numSegs)

	var segs []arcSegment
	curX, curY := x0, y0
	curTheta := theta1
	for i := 0; i < numSegs; i++ {
		nextTheta := curTheta + dtheta
		seg := arcSegmentFromAngles(cx, cy, rx, ry, phi, curTheta, nextTheta)
		// Override start/end with the running pen position so floating
		// noise across slice boundaries doesn't accumulate.
		seg.startX, seg.startY = curX, curY
		segs = append(segs, seg)
		curX, curY = seg.endX, seg.endY
		curTheta = nextTheta
	}

	// Force the very last endpoint to exactly the SVG-specified (x, y)
	// so caller-visible accuracy doesn't drift on multi-slice arcs.
	if len(segs) > 0 {
		segs[len(segs)-1].endX = x
		segs[len(segs)-1].endY = y
	}
	return segs
}

// arcSegmentFromAngles builds a single 90°-or-smaller cubic Bezier
// approximating the ellipse arc from theta1 to theta2. Uses the
// standard k = 4/3 * tan(d/4) formula where d is the half-arc-angle.
func arcSegmentFromAngles(cx, cy, rx, ry, phi, theta1, theta2 float64) arcSegment {
	dt := theta2 - theta1
	t := math.Tan(dt * 0.25)
	alpha := math.Sin(dt) * (math.Sqrt(4+3*t*t) - 1) / 3

	cosPhi := math.Cos(phi)
	sinPhi := math.Sin(phi)

	cos1, sin1 := math.Cos(theta1), math.Sin(theta1)
	cos2, sin2 := math.Cos(theta2), math.Sin(theta2)

	// Ellipse points in unrotated frame.
	p1x := rx * cos1
	p1y := ry * sin1
	p2x := rx * cos2
	p2y := ry * sin2

	// Tangents at start and end (derivative of the ellipse).
	t1x := -rx * sin1
	t1y := ry * cos1
	t2x := -rx * sin2
	t2y := ry * cos2

	// Control points before rotation/translation.
	c1xLocal := p1x + alpha*t1x
	c1yLocal := p1y + alpha*t1y
	c2xLocal := p2x - alpha*t2x
	c2yLocal := p2y - alpha*t2y

	// Apply rotation by phi and translation by (cx, cy).
	rot := func(x, y float64) (float64, float64) {
		return cosPhi*x - sinPhi*y + cx, sinPhi*x + cosPhi*y + cy
	}
	startX, startY := rot(p1x, p1y)
	c1xR, c1yR := rot(c1xLocal, c1yLocal)
	c2xR, c2yR := rot(c2xLocal, c2yLocal)
	endX, endY := rot(p2x, p2y)

	return arcSegment{
		startX: startX, startY: startY,
		c1x: c1xR, c1y: c1yR,
		c2x: c2xR, c2y: c2yR,
		endX: endX, endY: endY,
	}
}

// angleBetween returns the signed angle (radians) from vector (ux, uy)
// to vector (vx, vy). Used in the SVG arc parameterisation step 5.
func angleBetween(ux, uy, vx, vy float64) float64 {
	dot := ux*vx + uy*vy
	den := math.Sqrt(ux*ux+uy*uy) * math.Sqrt(vx*vx+vy*vy)
	if den == 0 {
		return 0
	}
	c := dot / den
	if c < -1 {
		c = -1
	} else if c > 1 {
		c = 1
	}
	a := math.Acos(c)
	if ux*vy-uy*vx < 0 {
		a = -a
	}
	return a
}
