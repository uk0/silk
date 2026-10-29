package scene3d

import (
	"math"
	"math/bits"
	"slices"
)

const cullLeafSize = 8
const cullHierarchyThreshold = 64

// CullingStats describes one visibility query, not a rendered frame or FPS.
// InstanceTests counts exact per-instance Frustum tests; BoundsTests counts BVH
// tests. RefittedNodes includes initial construction and subsequent changed nodes.
// A repeated camera query on the same immutable Snapshot does not rebuild bounds.
type CullingStats struct {
	BoundsTests, InstanceTests, RefittedNodes int
	// LinearFallbackInstances bounds overhead when ordered subtrees overlap heavily.
	LinearFallbackInstances          int
	HierarchyNodes                   int
	CulledInstances, CulledTriangles int
}

// Culler accelerates repeated camera queries on immutable snapshots. Its Morton-ordered
// bounding-volume hierarchy groups spatial neighbors internally; a compact bitset
// restores original instance order before returning. Equal-depth draw winners,
// picking IDs and consecutive-mesh instancing therefore remain unchanged. It is
// not an occlusion test or a SAH tree. Small scenes use the linear frustum test.
//
// The zero value is ready to use. Each render owner needs its own Culler; a Culler
// is not safe for concurrent calls. Immutable snapshots can be shared between
// independent cullers. No goroutines, timers or native resources are created.
// Reset releases the cached snapshot and hierarchy storage.
type Culler struct {
	snapshot  *Snapshot
	nodes     []cullNode
	leaves    int
	order     []uint64
	centroids []Vec3
	visible   []uint64
}

type cullNode struct {
	lo, hi, magnitude Vec3
	first, end        int
	triangles         int
	usable, dirty     bool
}

func (q *Culler) Reset() { *q = Culler{} }

// Cull resets dst to length zero and appends original, increasing instance
// indices which may be visible. The returned storage belongs to the caller;
// capacity can be reused on the next query. Invalid/uncertain bounds fail open,
// exactly as Frustum does. This optimization never replaces renderer validation
// or resource budgets. A nil snapshot produces an empty result.
func (q *Culler) Cull(dst []int, s *Snapshot, f Frustum) ([]int, CullingStats) {
	dst = dst[:0]
	stats := CullingStats{}
	n := s.InstanceCount()
	if n < cullHierarchyThreshold || n > MaxNodes {
		q.snapshot, q.leaves = nil, 0
		q.nodes = q.nodes[:0]
		for i := 0; i < n; i++ {
			dst = cullInstance(dst, s, i, &f, &stats)
		}
		return dst, stats
	}
	stats.RefittedNodes = q.prepare(s)
	stats.HierarchyNodes = len(q.nodes) - 1
	clear(q.visible)
	// MaxNodes bounds the balanced tree depth to 12. Iterative, left-first
	// traversal avoids recursion and does not allocate per camera movement.
	var stack [32]int
	stack[0] = 1
	top := 1
	for top > 0 {
		top--
		i := stack[top]
		node := &q.nodes[i]
		if node.first == node.end {
			continue
		}
		// Very large overlapping bounds may make every subtree overlap the view.
		// Bound hierarchy overhead and finish disjoint pending
		// ranges linearly rather than paying for an ineffective full traversal.
		if stats.BoundsTests >= max(16, n/64) {
			stats.LinearFallbackInstances += node.end - node.first
			for j := node.first; j < node.end; j++ {
				q.testInstance(s, int(uint32(q.order[j])), &f, &stats)
			}
			continue
		}
		stats.BoundsTests++
		switch node.classify(&f) {
		case cullOutside:
			stats.CulledInstances += node.end - node.first
			stats.CulledTriangles += node.triangles
			continue
		case cullInside:
			if i == 1 {
				for j := 0; j < n; j++ {
					dst = append(dst, j)
				}
				return dst, stats
			}
			for j := node.first; j < node.end; j++ {
				id := uint32(q.order[j])
				q.visible[id/64] |= uint64(1) << (id % 64)
			}
			continue
		}
		if i >= q.leaves {
			for j := node.first; j < node.end; j++ {
				q.testInstance(s, int(uint32(q.order[j])), &f, &stats)
			}
		} else {
			stack[top], stack[top+1] = i*2+1, i*2
			top += 2
		}
	}
	for wordIndex, word := range q.visible {
		for word != 0 {
			dst = append(dst, wordIndex*64+bits.TrailingZeros64(word))
			word &= word - 1
		}
	}
	return dst, stats
}

func (q *Culler) testInstance(s *Snapshot, i int, f *Frustum, stats *CullingStats) {
	in := &s.instances[i]
	stats.InstanceTests++
	if f.Intersects(in.mesh, in.world) {
		q.visible[i/64] |= uint64(1) << (i % 64)
	} else {
		stats.CulledInstances++
		stats.CulledTriangles += in.mesh.TriangleCount()
	}
}

func cullInstance(dst []int, s *Snapshot, i int, f *Frustum, stats *CullingStats) []int {
	in := &s.instances[i]
	stats.InstanceTests++
	if f.Intersects(in.mesh, in.world) {
		return append(dst, i)
	}
	stats.CulledInstances++
	stats.CulledTriangles += in.mesh.TriangleCount()
	return dst
}

func (q *Culler) prepare(s *Snapshot) int {
	if q.snapshot == s {
		return 0
	}
	n := len(s.instances)
	leaves := 1
	for leaves*cullLeafSize < n {
		leaves *= 2
	}
	rebuild := q.snapshot == nil || len(q.snapshot.instances) != n || q.leaves != leaves
	if rebuild {
		if cap(q.nodes) < leaves*2 {
			q.nodes = make([]cullNode, leaves*2)
		} else {
			q.nodes = q.nodes[:leaves*2]
		}
		q.leaves = leaves
		q.sortSpatially(s)
		words := (n + 63) / 64
		if cap(q.visible) < words {
			q.visible = make([]uint64, words)
		} else {
			q.visible = q.visible[:words]
		}
	}
	refits := 0
	for leaf := 0; leaf < leaves; leaf++ {
		first, end := min(leaf*cullLeafSize, n), min((leaf+1)*cullLeafSize, n)
		dirty := rebuild
		if !dirty {
			for j := first; j < end; j++ {
				i := int(uint32(q.order[j]))
				a, b := &s.instances[i], &q.snapshot.instances[i]
				if a.mesh != b.mesh || a.world != b.world {
					dirty = true
					break
				}
			}
		}
		if dirty {
			q.nodes[leaves+leaf] = makeCullLeaf(s, q.order, first, end)
			refits++
		}
		q.nodes[leaves+leaf].dirty = dirty
	}
	for i := leaves - 1; i > 0; i-- {
		a, b := q.nodes[i*2], q.nodes[i*2+1]
		if a.dirty || b.dirty {
			q.nodes[i] = mergeCullNodes(a, b)
			q.nodes[i].dirty = true
			refits++
		} else {
			q.nodes[i].dirty = false
		}
	}
	q.snapshot = s
	return refits
}

// A 30-bit Morton code interleaves ten bits per axis. Original indices occupy
// the low 32 bits, making equal-position ordering deterministic without a map.
// This linear-bounds + integer-sort build is independent of input node order.
// Refit reuses topology; changing instance count rebuilds the spatial ordering.
func (q *Culler) sortSpatially(s *Snapshot) {
	n := len(s.instances)
	if cap(q.order) < n {
		q.order = make([]uint64, n)
	} else {
		q.order = q.order[:n]
	}
	if cap(q.centroids) < n {
		q.centroids = make([]Vec3, n)
	} else {
		q.centroids = q.centroids[:n]
	}
	lo := Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := range s.instances {
		in := &s.instances[i]
		center := Vec3{}
		if bound, ok := instanceCullBounds(in.mesh, in.world); ok {
			center = bound.lo.Mul(.5).Add(bound.hi.Mul(.5))
		}
		q.centroids[i] = center
		lo, hi = minCullVec(lo, center), maxCullVec(hi, center)
	}
	for i, center := range q.centroids {
		x, y, z := mortonQuantize(center.X, lo.X, hi.X), mortonQuantize(center.Y, lo.Y, hi.Y), mortonQuantize(center.Z, lo.Z, hi.Z)
		var code uint32
		for bit := uint(0); bit < 10; bit++ {
			code |= ((x >> bit) & 1) << (3 * bit)
			code |= ((y >> bit) & 1) << (3*bit + 1)
			code |= ((z >> bit) & 1) << (3*bit + 2)
		}
		q.order[i] = uint64(code)<<32 | uint64(i)
	}
	slices.Sort(q.order)
}

func mortonQuantize(v, lo, hi float64) uint32 {
	if hi <= lo {
		return 0
	}
	return uint32(max(0, min(1023, (v-lo)/(hi-lo)*1023)))
}

func makeCullLeaf(s *Snapshot, order []uint64, first, end int) cullNode {
	n := cullNode{first: first, end: end, usable: first != end}
	lo := Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for j := first; j < end; j++ {
		in := &s.instances[int(uint32(order[j]))]
		n.triangles += in.mesh.TriangleCount()
		bound, ok := instanceCullBounds(in.mesh, in.world)
		if !ok {
			n.usable = false
			continue
		}
		lo, hi = minCullVec(lo, bound.lo), maxCullVec(hi, bound.hi)
		n.magnitude = maxCullVec(n.magnitude, bound.magnitude)
	}
	n.lo, n.hi = lo, hi
	return n
}

func minCullVec(a, b Vec3) Vec3 { return Vec3{min(a.X, b.X), min(a.Y, b.Y), min(a.Z, b.Z)} }
func maxCullVec(a, b Vec3) Vec3 { return Vec3{max(a.X, b.X), max(a.Y, b.Y), max(a.Z, b.Z)} }

func mergeCullNodes(a, b cullNode) cullNode {
	if a.first == a.end {
		return b
	}
	if b.first == b.end {
		return a
	}
	return cullNode{
		lo: minCullVec(a.lo, b.lo), hi: maxCullVec(a.hi, b.hi),
		magnitude: maxCullVec(a.magnitude, b.magnitude),
		first:     a.first, end: b.end, triangles: a.triangles + b.triangles,
		usable: a.usable && b.usable,
	}
}

// Keep absolute pre-cancellation magnitudes, not just the small resulting AABB.
// They bound the float32 world/clip arithmetic used by the GPU path too.
func instanceCullBounds(mesh *Mesh, world Mat4) (cullNode, bool) {
	if mesh == nil || len(mesh.indices) == 0 || !world.affine() || !mesh.min.valid() || !mesh.max.valid() {
		return cullNode{}, false
	}
	for _, x := range world {
		if math.Abs(x) > math.MaxFloat32 {
			return cullNode{}, false
		}
	}
	lo, hi := [3]float64{mesh.min.X, mesh.min.Y, mesh.min.Z}, [3]float64{mesh.max.X, mesh.max.Y, mesh.max.Z}
	var lower, upper, magnitude [3]float64
	for axis := range lo {
		if lo[axis] > hi[axis] {
			return cullNode{}, false
		}
	}
	for row := 0; row < 3; row++ {
		lower[row], upper[row] = world[12+row], world[12+row]
		// Match the leaf test's summation order at the float32-range boundary.
		magnitude[row] = math.Abs(world[row])*max(math.Abs(lo[0]), math.Abs(hi[0])) + math.Abs(world[4+row])*max(math.Abs(lo[1]), math.Abs(hi[1])) + math.Abs(world[8+row])*max(math.Abs(lo[2]), math.Abs(hi[2])) + math.Abs(world[12+row])
		for axis := 0; axis < 3; axis++ {
			v := world[axis*4+row]
			a, b := v*lo[axis], v*hi[axis]
			lower[row] += min(a, b)
			upper[row] += max(a, b)
		}
		if !finite(lower[row]) || !finite(upper[row]) || !finite(magnitude[row]) || magnitude[row] > math.MaxFloat32 {
			return cullNode{}, false
		}
	}
	return cullNode{lo: Vec3{lower[0], lower[1], lower[2]}, hi: Vec3{upper[0], upper[1], upper[2]}, magnitude: Vec3{magnitude[0], magnitude[1], magnitude[2]}, usable: true}, true
}

const (
	cullIntersect = iota
	cullOutside
	cullInside
)

func (n *cullNode) classify(f *Frustum) int {
	if !n.usable || !f.usable {
		return cullIntersect
	}
	// Validate all clip-coordinate magnitudes before any rejection. A distant X
	// must not hide a Z overflow which the renderer must still diagnose.
	var clip [4]float64
	for row := range clip {
		v := f.absVP[row]*n.magnitude.X + f.absVP[4+row]*n.magnitude.Y + f.absVP[8+row]*n.magnitude.Z + f.absVP[12+row]
		if !finite(v) || v > math.MaxFloat32 {
			return cullIntersect
		}
		clip[row] = v
	}
	inside := true
	for i, p := range f.planes {
		lo, hi := n.lo, n.hi
		if p[0] < 0 {
			lo.X, hi.X = hi.X, lo.X
		}
		if p[1] < 0 {
			lo.Y, hi.Y = hi.Y, lo.Y
		}
		if p[2] < 0 {
			lo.Z, hi.Z = hi.Z, lo.Z
		}
		lower := p[0]*lo.X + p[1]*lo.Y + p[2]*lo.Z + p[3]
		upper := p[0]*hi.X + p[1]*hi.Y + p[2]*hi.Z + p[3]
		// Twice the leaf test's slack covers AABB construction/reassociation and
		// retains borderline geometry. Only fully certain subtrees are skipped.
		tolerance := (clip[3]+clip[i/2])/32768 + 2e-8
		if !finite(lower) || !finite(upper) || !finite(tolerance) {
			return cullIntersect
		}
		if upper < -tolerance {
			return cullOutside
		}
		if lower <= tolerance {
			inside = false
		}
	}
	if inside {
		return cullInside
	}
	return cullIntersect
}
