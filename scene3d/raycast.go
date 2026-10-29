package scene3d

import (
	"context"
	"math"
)

// RayQuery distances are world units, irrespective of the input ray's length.
// MinDistance is inclusive and nonnegative. MaxDistance=0 means unbounded;
// otherwise it must be finite and >= MinDistance. Surfaces are two-sided unless
// CullBackfaces is set. SkipNode excludes one node (not its descendants).
type RayQuery struct {
	MinDistance, MaxDistance float64
	CullBackfaces            bool
	SkipNode                 string
}

// RayHit identifies original snapshot/mesh indices, not BVH ordering. Normal
// follows transformed vertex winding (including reflections), not an automatic
// face-forward convention. Barycentric holds weights for vertices 0/1/2;
// LocalPosition is suitable for a mesh-local attachment point. Geometric hits
// do not promise exact raster-pixel coverage, MSAA or float32 GPU depth ties.
type RayHit struct {
	NodeID                                       string
	Instance, Triangle                           int
	Distance                                     float64
	Position, LocalPosition, Normal, Barycentric Vec3
	FrontFace                                    bool
}

type RaycastStats struct{ BoundsTests, InstanceTests, TriangleTests int }
type rayVisit struct {
	index int
	entry float64
}

// Cast returns the nearest geometric hit without constructing a framebuffer or
// allocating per query. Exact distance ties choose earlier snapshot instance,
// then earlier original triangle. Degenerate triangles and coplanar/parallel
// rays do not hit. Hidden nodes are already excluded by Snapshot.
//
// Cancellation and unrepresentable numeric operations return an error and no
// partial hit; neither the index nor any other concurrent query is changed.
func (r *RayIndex) Cast(ctx context.Context, ray Ray, options RayQuery) (hit RayHit, found bool, stats RaycastStats, err error) {
	if r == nil || r.snapshot == nil {
		return hit, false, stats, ErrNode
	}
	if ctx == nil {
		return hit, false, stats, ErrGeometry
	}
	if err = ctx.Err(); err != nil {
		return hit, false, stats, err
	}
	direction, ok := projectionUnit(ray.Direction)
	if !ok || !ray.Origin.valid() || !finite(options.MinDistance) || options.MinDistance < 0 || !finite(options.MaxDistance) || options.MaxDistance < 0 || options.MaxDistance != 0 && options.MaxDistance < options.MinDistance {
		return hit, false, stats, ErrGeometry
	}
	ray.Direction = direction
	limit := options.MaxDistance
	if limit == 0 {
		limit = math.Inf(1)
	}
	if r.top == nil {
		return hit, false, stats, nil
	}
	var stack [32]rayVisit
	stats.BoundsTests++
	entry, ok := raySlab(r.top.nodes[1].box, ray, options.MinDistance, limit)
	if !ok {
		return hit, false, stats, nil
	}
	stack[0] = rayVisit{1, entry}
	top := 1
	for top > 0 {
		if err = ctx.Err(); err != nil {
			return RayHit{}, false, stats, err
		}
		top--
		visit := stack[top]
		if visit.entry > limit {
			continue
		}
		node := r.top.nodes[visit.index]
		if visit.index < r.top.leaves {
			r.top.pushChildren(&stack, &top, visit.index, ray, options.MinDistance, limit, Identity(), false, &stats)
			continue
		}
		for j := node.first; j < node.end; j++ {
			ii := int(uint32(r.top.order[j]))
			in := r.snapshot.instances[ii]
			if in.id == options.SkipNode {
				continue
			}
			stats.InstanceTests++
			local := in.world
			local[12] -= ray.Origin.X
			local[13] -= ray.Origin.Y
			local[14] -= ray.Origin.Z
			if !local.valid() {
				return RayHit{}, false, stats, ErrGeometry
			}
			err = r.castMesh(ctx, ii, local, ray, options, &limit, &hit, &found, &stats)
			if err != nil {
				return RayHit{}, false, stats, err
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return RayHit{}, false, stats, err
	}
	return hit, found, stats, nil
}

func (r *RayIndex) castMesh(ctx context.Context, ii int, relative Mat4, ray Ray, options RayQuery, limit *float64, hit *RayHit, found *bool, stats *RaycastStats) error {
	in := r.snapshot.instances[ii]
	mesh := in.mesh
	tree := r.meshes[mesh]
	localRay := Ray{Direction: ray.Direction}
	var stack [32]rayVisit
	stats.BoundsTests++
	entry, ok := raySlab(rayTransformBox(tree.nodes[1].box, relative), localRay, options.MinDistance, *limit)
	if !ok {
		return nil
	}
	stack[0] = rayVisit{1, entry}
	top := 1
	nextCheck := stats.BoundsTests + stats.TriangleTests + 256
	for top > 0 {
		if stats.BoundsTests+stats.TriangleTests >= nextCheck {
			nextCheck = stats.BoundsTests + stats.TriangleTests + 256
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		top--
		visit := stack[top]
		if visit.entry > *limit {
			continue
		}
		node := tree.nodes[visit.index]
		if visit.index < tree.leaves {
			tree.pushChildren(&stack, &top, visit.index, localRay, options.MinDistance, *limit, relative, true, stats)
			continue
		}
		for j := node.first; j < node.end; j++ {
			ti := int(uint32(tree.order[j]))
			offset := ti * 3
			a, b, c := mesh.vertices[mesh.indices[offset]], mesh.vertices[mesh.indices[offset+1]], mesh.vertices[mesh.indices[offset+2]]
			stats.TriangleTests++
			distance, weights, normal, ok, err := rayTriangle(relative, a, b, c, ray.Direction, options.MinDistance, *limit)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			front := normal.Dot(ray.Direction) < 0
			if options.CullBackfaces && !front {
				continue
			}
			if *found && distance == *limit && (ii > hit.Instance || ii == hit.Instance && ti >= hit.Triangle) {
				continue
			}
			local := a.Mul(weights.X).Add(b.Mul(weights.Y)).Add(c.Mul(weights.Z))
			// Reconstruct on the surface, not origin + direction*t: distant rays
			// otherwise cancel away small, representable world coordinates.
			position := in.world.Point(local)
			if !position.valid() || !local.valid() {
				return ErrGeometry
			}
			*hit = RayHit{in.id, ii, ti, distance, position, local, normal, weights, front}
			*limit, *found = distance, true
		}
	}
	return ctx.Err()
}

func (tree *rayTree) pushChildren(stack *[32]rayVisit, top *int, parent int, ray Ray, minDistance, maxDistance float64, transform Mat4, transformed bool, stats *RaycastStats) {
	var visits [2]rayVisit
	count := 0
	for _, i := range [2]int{parent * 2, parent*2 + 1} {
		node := tree.nodes[i]
		if node.first == node.end {
			continue
		}
		b := node.box
		if transformed {
			b = rayTransformBox(b, transform)
		}
		stats.BoundsTests++
		if entry, ok := raySlab(b, ray, minDistance, maxDistance); ok {
			visits[count] = rayVisit{i, entry}
			count++
		}
	}
	if count == 2 && visits[0].entry < visits[1].entry {
		visits[0], visits[1] = visits[1], visits[0]
	}
	for i := range count {
		stack[*top] = visits[i]
		*top++
	}
}

// Slabs conservatively expand division endpoints by one representable value.
// Invalid arithmetic fails open instead of hiding a potentially intersected leaf.
func raySlab(box rayBox, ray Ray, near, far float64) (float64, bool) {
	if !box.valid {
		return near, true
	}
	origins, directions := [3]float64{ray.Origin.X, ray.Origin.Y, ray.Origin.Z}, [3]float64{ray.Direction.X, ray.Direction.Y, ray.Direction.Z}
	lo, hi := [3]float64{box.lo.X, box.lo.Y, box.lo.Z}, [3]float64{box.hi.X, box.hi.Y, box.hi.Z}
	for i := range 3 {
		if directions[i] == 0 {
			if origins[i] < lo[i] || origins[i] > hi[i] {
				return 0, false
			}
			continue
		}
		a, b := (lo[i]-origins[i])/directions[i], (hi[i]-origins[i])/directions[i]
		if !finite(a) || !finite(b) {
			return near, true
		}
		if a > b {
			a, b = b, a
		}
		near, far = max(near, math.Nextafter(a, math.Inf(-1))), min(far, math.Nextafter(b, math.Inf(1)))
		if near > far {
			return 0, false
		}
	}
	return near, true
}

func rayVector(m Mat4, v Vec3) Vec3 {
	return Vec3{m[0]*v.X + m[4]*v.Y + m[8]*v.Z, m[1]*v.X + m[5]*v.Y + m[9]*v.Z, m[2]*v.X + m[6]*v.Y + m[10]*v.Z}
}

// rayNormal retains mixed-scale components: normalizing both edges first can
// erase a tiny component even when their cross product is representable.
func rayNormal(m Mat4, a, b Vec3) (Vec3, bool) {
	u, v := rayVector(m, a), rayVector(m, b)
	if !u.valid() {
		u = rayVector(m, a.Mul(.25))
	}
	if !v.valid() {
		v = rayVector(m, b.Mul(.25))
	}
	if !u.valid() || !v.valid() {
		return Vec3{}, false
	}
	n := u.Cross(v)
	if n.valid() && n != (Vec3{}) {
		return projectionUnit(n)
	}
	// Products may overflow/underflow while the direction remains meaningful.
	// Store each component with its own binary exponent before common scaling.
	pairs := [3][4]float64{{u.Y, v.Z, u.Z, v.Y}, {u.Z, v.X, u.X, v.Z}, {u.X, v.Y, u.Y, v.X}}
	var mantissa [3]float64
	var exponent [3]int
	largest := -10000
	for i, p := range pairs {
		a, ae := math.Frexp(p[0])
		b, be := math.Frexp(p[1])
		c, ce := math.Frexp(p[2])
		d, de := math.Frexp(p[3])
		pe, qe := ae+be, ce+de
		if a == 0 || b == 0 {
			pe = -10000
		}
		if c == 0 || d == 0 {
			qe = -10000
		}
		e := max(pe, qe)
		mantissa[i] = rayDifferenceOfProducts(math.Ldexp(a, pe-e), b, math.Ldexp(c, qe-e), d)
		exponent[i] = e
		if mantissa[i] != 0 {
			largest = max(largest, e)
		}
	}
	return projectionUnit(Vec3{math.Ldexp(mantissa[0], exponent[0]-largest), math.Ldexp(mantissa[1], exponent[1]-largest), math.Ldexp(mantissa[2], exponent[2]-largest)})
}

func rayComponent(v Vec3, i int) float64 {
	switch i {
	case 0:
		return v.X
	case 1:
		return v.Y
	default:
		return v.Z
	}
}
func rayDifferenceOfProducts(a, b, c, d float64) float64 {
	p, q := a*b, c*d
	return (p - q) + (math.FMA(a, b, -p) - math.FMA(c, d, -q))
}

// Dominant-axis shearing gives shared edges the same sign test. Power-of-two
// scaling keeps projected products representable without an arbitrary geometric
// epsilon. This is a float64 geometric query, not an exact-arithmetic predicate.
func rayTriangle(m Mat4, a, b, c, direction Vec3, near, far float64) (distance float64, weights, normal Vec3, hit bool, err error) {
	p := [3]Vec3{m.Point(a), m.Point(b), m.Point(c)}
	for _, v := range p {
		if !v.valid() {
			err = ErrGeometry
			return
		}
	}
	kz := 0
	if math.Abs(direction.Y) > math.Abs(direction.X) {
		kz = 1
	}
	if math.Abs(direction.Z) > math.Abs(rayComponent(direction, kz)) {
		kz = 2
	}
	kx, ky := (kz+1)%3, (kz+2)%3
	dz := rayComponent(direction, kz)
	sx, sy := rayComponent(direction, kx)/dz, rayComponent(direction, ky)/dz
	// Scale the two sheared axes independently from depth. A single global
	// scale loses unit-size surfaces at distant origins and extreme anisotropy.
	var x, y, z [3]float64
	var xmax, ymax, zmax float64
	for _, v := range p {
		vz := rayComponent(v, kz)
		xmax = max(xmax, math.Abs(rayComponent(v, kx)), math.Abs(sx*vz))
		ymax = max(ymax, math.Abs(rayComponent(v, ky)), math.Abs(sy*vz))
		zmax = max(zmax, math.Abs(vz))
	}
	_, ex := math.Frexp(xmax)
	_, ey := math.Frexp(ymax)
	_, ez := math.Frexp(zmax)
	for i, v := range p {
		vz := rayComponent(v, kz)
		x[i] = math.Ldexp(rayComponent(v, kx), -ex) - math.Ldexp(sx*vz, -ex)
		y[i] = math.Ldexp(rayComponent(v, ky), -ey) - math.Ldexp(sy*vz, -ey)
		z[i] = math.Ldexp(vz, -ez)
	}
	e0, e1, e2 := rayDifferenceOfProducts(x[1], y[2], y[1], x[2]), rayDifferenceOfProducts(x[2], y[0], y[2], x[0]), rayDifferenceOfProducts(x[0], y[1], y[0], x[1])
	if (e0 < 0 || e1 < 0 || e2 < 0) && (e0 > 0 || e1 > 0 || e2 > 0) {
		return
	}
	det := e0 + e1 + e2
	if det == 0 {
		return
	}
	weights = Vec3{e0 / det, e1 / det, e2 / det}
	distance = math.Ldexp((weights.X*z[0]+weights.Y*z[1]+weights.Z*z[2])/dz, ez)
	if !finite(distance) {
		err = ErrGeometry
		return
	}
	if distance < near || distance > far {
		return
	}
	var ok bool
	normal, ok = rayNormal(m, b.Sub(a), c.Sub(a))
	if !ok {
		return
	}
	hit = true
	return
}
