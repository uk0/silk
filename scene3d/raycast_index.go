package scene3d

import (
	"context"
	"math"
	"slices"
)

const MaxRayIndexBytes = 64 << 20
const rayLeafSize = 8

// RayIndexLimits bounds estimated retained acceleration storage, not source mesh
// storage, process RSS or caller-retained older indexes. Zero uses the hard cap.
type RayIndexLimits struct{ MaxBytes int64 }

// RayIndexStats describes construction of this immutable index. Shared meshes
// are counted once. EstimatedBytes is conservative on supported Go architectures.
type RayIndexStats struct {
	Instances, Meshes, Triangles, MeshBuilds, MeshReuses int
	HierarchyNodes, RefittedNodes                        int
	InstanceTreeReused                                   bool
	EstimatedBytes                                       int64
}

// RayIndex owns an immutable two-level BVH for geometric surface queries. It is
// independent of render targets and native contexts. Construct/WithSnapshot can
// run on background workers; Cast and WithSnapshot may be called concurrently.
// No mesh, snapshot, old index or caller data is mutated. Drop the last reference
// to release storage; there are no timers, goroutines or native resources.
//
// Mesh BVHs use deterministic Morton ordering. Same-count snapshot updates refit
// the instance hierarchy, reusing mesh BVHs. Large movements can degrade that
// topology; NewRayIndex explicitly rebuilds it. This is not a SAH/occlusion/LOD
// accelerator, a physics engine or a replacement for raster-pixel picking.
type RayIndex struct {
	snapshot *Snapshot
	meshes   map[*Mesh]*rayTree
	top      *rayTree
	limits   RayIndexLimits
	stats    RayIndexStats
}

type rayBox struct {
	lo, hi Vec3
	valid  bool
}
type rayNode struct {
	box        rayBox
	first, end int
}
type rayTree struct {
	nodes  []rayNode
	order  []uint64
	leaves int
}

func (r *RayIndex) Stats() RayIndexStats {
	if r == nil {
		return RayIndexStats{}
	}
	return r.stats
}
func (r *RayIndex) Snapshot() *Snapshot {
	if r == nil {
		return nil
	}
	return r.snapshot
}

func NewRayIndex(ctx context.Context, snapshot *Snapshot, limits RayIndexLimits) (*RayIndex, error) {
	return buildRayIndex(ctx, snapshot, limits, nil)
}

// WithSnapshot publishes a separate immutable index only on success. Geometry
// removed from the new snapshot is not retained in its mesh cache. The exact
// same snapshot returns r. A nil receiver behaves like NewRayIndex with defaults.
func (r *RayIndex) WithSnapshot(ctx context.Context, snapshot *Snapshot) (*RayIndex, error) {
	limits := RayIndexLimits{}
	if r != nil {
		limits = r.limits
	}
	return buildRayIndex(ctx, snapshot, limits, r)
}

func rayLeaves(n int) int {
	k := 1
	for k*rayLeafSize < n {
		k *= 2
	}
	return k
}
func rayTreeBytes(n int) int64 {
	if n == 0 {
		return 0
	}
	// A rayNode occupies at most 72 bytes on 64-bit Go; include slice metadata
	// and padding conservatively, without unsafe or architecture-dependent casts.
	return int64(rayLeaves(n))*2*80 + int64(n)*8 + 128
}

func buildRayIndex(ctx context.Context, snapshot *Snapshot, limits RayIndexLimits, old *RayIndex) (*RayIndex, error) {
	if ctx == nil {
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrNode
	}
	if limits.MaxBytes == 0 {
		limits.MaxBytes = MaxRayIndexBytes
	}
	if limits.MaxBytes < 0 || limits.MaxBytes > MaxRayIndexBytes {
		return nil, ErrLimit
	}
	if old != nil && snapshot == old.snapshot {
		return old, nil
	}
	n := snapshot.InstanceCount()
	if n > MaxNodes {
		return nil, ErrLimit
	}
	cost := int64(256+n*32) + rayTreeBytes(n)
	if cost > limits.MaxBytes {
		return nil, ErrLimit
	}
	r := &RayIndex{snapshot: snapshot, limits: limits, meshes: make(map[*Mesh]*rayTree)}
	vertices, triangles := 0, 0
	same := old != nil && old.snapshot.InstanceCount() == n
	for i, in := range snapshot.instances {
		if i&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if in.mesh == nil || in.mesh.VertexCount() == 0 || in.mesh.TriangleCount() == 0 || !in.world.affine() {
			return nil, ErrGeometry
		}
		vertices += in.mesh.VertexCount()
		triangles += in.mesh.TriangleCount()
		if vertices > MaxSnapshotVertices || triangles > MaxSnapshotTriangles {
			return nil, ErrLimit
		}
		if _, exists := r.meshes[in.mesh]; !exists {
			cost += rayTreeBytes(in.mesh.TriangleCount()) + 128
			if cost > limits.MaxBytes {
				return nil, ErrLimit
			}
			r.meshes[in.mesh] = nil
			r.stats.Triangles += in.mesh.TriangleCount()
		}
		if same && (in.mesh != old.snapshot.instances[i].mesh || in.world != old.snapshot.instances[i].world) {
			same = false
		}
	}
	// Build in snapshot order, not map order, for deterministic cancellation and
	// statistics. Temporary centroid storage is reused and bounded by one mesh.
	var scratch []Vec3
	for _, in := range snapshot.instances {
		if r.meshes[in.mesh] != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if old != nil && old.meshes[in.mesh] != nil {
			r.meshes[in.mesh] = old.meshes[in.mesh]
			r.stats.MeshReuses++
			continue
		}
		mesh := in.mesh
		tree, err := buildRayTree(ctx, mesh.TriangleCount(), func(i int) rayBox { return rayTriangleBox(mesh, i) }, &scratch)
		if err != nil {
			return nil, err
		}
		r.meshes[mesh] = tree
		r.stats.MeshBuilds++
	}
	bounds := func(i int) rayBox {
		in := snapshot.instances[i]
		return rayTransformBox(rayBox{in.mesh.min, in.mesh.max, true}, in.world)
	}
	var err error
	if same {
		r.top = old.top
		r.stats.InstanceTreeReused = true
	} else if old != nil && old.snapshot.InstanceCount() == n && n != 0 {
		r.top = &rayTree{order: old.top.order, leaves: old.top.leaves, nodes: make([]rayNode, len(old.top.nodes))}
		err = r.top.refit(ctx, bounds)
		r.stats.RefittedNodes = len(r.top.nodes) - 1
	} else {
		r.top, err = buildRayTree(ctx, n, bounds, &scratch)
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.stats.Instances, r.stats.Meshes, r.stats.EstimatedBytes = n, len(r.meshes), cost
	if r.top != nil {
		r.stats.HierarchyNodes = len(r.top.nodes) - 1
	}
	for _, tree := range r.meshes {
		if tree != nil {
			r.stats.HierarchyNodes += len(tree.nodes) - 1
		}
	}
	return r, nil
}

func buildRayTree(ctx context.Context, n int, bounds func(int) rayBox, scratch *[]Vec3) (*rayTree, error) {
	if n == 0 {
		return nil, nil
	}
	tree := &rayTree{leaves: rayLeaves(n), order: make([]uint64, n)}
	tree.nodes = make([]rayNode, tree.leaves*2)
	if cap(*scratch) < n {
		*scratch = make([]Vec3, n)
	} else {
		*scratch = (*scratch)[:n]
	}
	centers := *scratch
	lo, hi := Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}, Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := range n {
		if i&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		box := bounds(i)
		c := Vec3{}
		if box.valid {
			c = box.lo.Mul(.5).Add(box.hi.Mul(.5))
		}
		centers[i] = c
		lo, hi = minCullVec(lo, c), maxCullVec(hi, c)
	}
	for i, c := range centers {
		x, y, z := rayQuantize(c.X, lo.X, hi.X), rayQuantize(c.Y, lo.Y, hi.Y), rayQuantize(c.Z, lo.Z, hi.Z)
		var code uint32
		for bit := uint(0); bit < 10; bit++ {
			code |= ((x>>bit)&1)<<(bit*3) | ((y>>bit)&1)<<(bit*3+1) | ((z>>bit)&1)<<(bit*3+2)
		}
		tree.order[i] = uint64(code)<<32 | uint64(i)
	}
	// One bounded integer sort; cancellation is checked before publication.
	slices.Sort(tree.order)
	if err := tree.refit(ctx, bounds); err != nil {
		return nil, err
	}
	return tree, nil
}
func rayQuantize(v, lo, hi float64) uint32 {
	if hi <= lo {
		return 0
	}
	scale := max(math.Abs(lo), math.Abs(hi))
	lo, hi, v = lo/scale, hi/scale, v/scale
	return uint32(max(0, min(1023, (v-lo)/(hi-lo)*1023)))
}
func (tree *rayTree) refit(ctx context.Context, bounds func(int) rayBox) error {
	n := len(tree.order)
	for leaf := range tree.leaves {
		if leaf&127 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		first, end := min(n, leaf*rayLeafSize), min(n, (leaf+1)*rayLeafSize)
		node := rayNode{first: first, end: end}
		for j := first; j < end; j++ {
			b := bounds(int(uint32(tree.order[j])))
			if j == first {
				node.box = b
			} else {
				node.box = rayUnion(node.box, b)
			}
		}
		tree.nodes[tree.leaves+leaf] = node
	}
	for i := tree.leaves - 1; i > 0; i-- {
		a, b := tree.nodes[i*2], tree.nodes[i*2+1]
		switch {
		case a.first == a.end:
			tree.nodes[i] = b
		case b.first == b.end:
			tree.nodes[i] = a
		default:
			tree.nodes[i] = rayNode{rayUnion(a.box, b.box), a.first, b.end}
		}
	}
	return ctx.Err()
}
func rayUnion(a, b rayBox) rayBox {
	if !a.valid || !b.valid {
		return rayBox{}
	}
	return rayBox{minCullVec(a.lo, b.lo), maxCullVec(a.hi, b.hi), true}
}
func rayTriangleBox(m *Mesh, triangle int) rayBox {
	i := triangle * 3
	a, b, c := m.vertices[m.indices[i]], m.vertices[m.indices[i+1]], m.vertices[m.indices[i+2]]
	return rayBox{minCullVec(a, minCullVec(b, c)), maxCullVec(a, maxCullVec(b, c)), true}
}

// Bounds are conservative, including rounding during affine evaluation. Invalid
// or overflowing arithmetic fails open; only exact triangle evaluation decides
// a hit/error. Relative translation is used in mesh traversal, so a large world
// origin does not erase small local details. No inverse matrix is required:
// reflections, shear and singular transforms retain their geometric meaning.
func rayTransformBox(box rayBox, m Mat4) rayBox {
	if !box.valid {
		return rayBox{}
	}
	var lo, hi [3]float64
	for row := range 3 {
		a, b := m[row]*box.lo.X, m[row]*box.hi.X
		c, d := m[4+row]*box.lo.Y, m[4+row]*box.hi.Y
		e, f := m[8+row]*box.lo.Z, m[8+row]*box.hi.Z
		l := m[12+row] + min(a, b) + min(c, d) + min(e, f)
		h := m[12+row] + max(a, b) + max(c, d) + max(e, f)
		pad := (math.Abs(m[12+row])+max(math.Abs(a), math.Abs(b))+max(math.Abs(c), math.Abs(d))+max(math.Abs(e), math.Abs(f)))*(64*0x1p-52) + 16*math.SmallestNonzeroFloat64
		lo[row], hi[row] = math.Nextafter(l-pad, math.Inf(-1)), math.Nextafter(h+pad, math.Inf(1))
		if !finite(lo[row]) || !finite(hi[row]) {
			return rayBox{}
		}
	}
	return rayBox{Vec3{lo[0], lo[1], lo[2]}, Vec3{hi[0], hi[1], hi[2]}, true}
}
