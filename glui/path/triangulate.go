package path

// Triangulate decomposes a simple (non-self-intersecting) polygon into
// triangles using the ear-clipping algorithm. The polygon may be convex or
// concave; holes and self-intersections are not supported.
//
// The returned slice contains 3 indices per triangle, indexing back into
// the input points. Winding of the output triangles matches the winding of
// the input polygon.
//
// Complexity is O(n²) — perfectly acceptable for UI paths up to a few
// hundred vertices. For heavier geometry plug in a tessellator like
// libtess2 or use a constrained Delaunay implementation.
//
// If the input is degenerate (fewer than three vertices, or unable to find
// any ears) Triangulate returns whatever progress was made; callers may
// safely render a partial result.
func Triangulate(points [][2]float32) []uint16 {
	n := len(points)
	if n < 3 {
		return nil
	}

	// Doubly-linked list of vertex indices. We never compact the slices —
	// we only re-wire prev/next around removed ears, so an index that
	// participates in any active triangle still satisfies
	// next[prev[i]] == i. That invariant lets us identify which indices
	// are still live without an extra "alive" bitmap.
	prev := make([]int, n)
	next := make([]int, n)
	for i := 0; i < n; i++ {
		prev[i] = (i + n - 1) % n
		next[i] = (i + 1) % n
	}

	// Polygon orientation determines which way "convex corner" goes; the
	// ear test depends on it.
	ccw := isCCW(points)

	indices := make([]uint16, 0, (n-2)*3)
	remaining := n
	bail := 0
	for remaining > 3 {
		earFound := false
		for i := 0; i < n && remaining > 3; i++ {
			// Skip indices whose linked-list slot has already been
			// snipped — only live vertices satisfy the invariant.
			if next[prev[i]] != i {
				continue
			}
			if isEar(points, prev, next, i, ccw) {
				indices = append(indices, uint16(prev[i]), uint16(i), uint16(next[i]))
				next[prev[i]] = next[i]
				prev[next[i]] = prev[i]
				remaining--
				earFound = true
				break
			}
		}
		if !earFound {
			// Degenerate input (e.g. all colinear, or self-intersecting).
			// Bail out with the partial result rather than spinning.
			return indices
		}
		bail++
		if bail > n*n {
			return indices
		}
	}

	// One triangle remains; emit it. Live vertices satisfy the
	// next[prev[i]] == i invariant, so we walk the array until we find
	// the first live index and emit (prev[i], i, next[i]).
	for i := 0; i < n; i++ {
		if next[prev[i]] == i {
			indices = append(indices, uint16(prev[i]), uint16(i), uint16(next[i]))
			break
		}
	}
	return indices
}

// isCCW returns true if the polygon's vertices are in counter-clockwise
// order. Uses the shoelace formula. Note our coordinate system is Y-down,
// so the sign convention here is inverted relative to math-class CCW: a
// negative signed area in screen-space corresponds to CCW traversal as a
// human looking at the screen would describe it.
func isCCW(points [][2]float32) bool {
	var sum float32
	n := len(points)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		sum += (points[j][0] - points[i][0]) * (points[j][1] + points[i][1])
	}
	return sum < 0
}

// isEar tests whether vertex i forms an "ear": the triangle (prev[i], i,
// next[i]) is fully inside the polygon and contains no other polygon
// vertex.
func isEar(points [][2]float32, prev, next []int, i int, ccw bool) bool {
	a := points[prev[i]]
	b := points[i]
	c := points[next[i]]

	// The corner must turn the right way for the polygon's winding.
	cross := (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
	if ccw {
		if cross <= 0 {
			return false
		}
	} else {
		if cross >= 0 {
			return false
		}
	}

	// No other live polygon vertex may sit inside the candidate triangle.
	// Walk the linked list starting two steps after i, stopping before we
	// reach prev[i].
	for j := next[next[i]]; j != prev[i]; j = next[j] {
		if pointInTriangle(points[j], a, b, c) {
			return false
		}
	}
	return true
}

// pointInTriangle returns true if p lies inside the triangle (a, b, c)
// using barycentric sign tests. Edge-coincident points count as outside,
// matching the ear-clipping convention.
func pointInTriangle(p, a, b, c [2]float32) bool {
	s := (a[0]-c[0])*(p[1]-c[1]) - (a[1]-c[1])*(p[0]-c[0])
	t := (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])
	if (s < 0) != (t < 0) && s != 0 && t != 0 {
		return false
	}
	d := (c[0]-b[0])*(p[1]-b[1]) - (c[1]-b[1])*(p[0]-b[0])
	return d == 0 || (d < 0) == (s+t <= 0)
}
