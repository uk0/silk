package scene3d

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
)

func rayIndexFor(t testing.TB, nodes ...Node) (*Scene, *RayIndex) {
	t.Helper()
	s := NewScene()
	for _, n := range nodes {
		if e := s.Add(n); e != nil {
			t.Fatal(e)
		}
	}
	snap, e := s.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	idx, e := NewRayIndex(context.Background(), snap, RayIndexLimits{})
	if e != nil {
		t.Fatal(e)
	}
	return s, idx
}
func rayAlmost(a, b float64) bool { return math.Abs(a-b) <= 1e-9*max(1, math.Abs(a), math.Abs(b)) }
func rayVecAlmost(a, b Vec3) bool {
	return rayAlmost(a.X, b.X) && rayAlmost(a.Y, b.Y) && rayAlmost(a.Z, b.Z)
}
func rayGrid(t testing.TB, n int) *Mesh {
	t.Helper()
	vertices := make([]Vec3, (n+1)*(n+1))
	indices := make([]uint32, 0, n*n*6)
	for y := 0; y <= n; y++ {
		for x := 0; x <= n; x++ {
			vertices[y*(n+1)+x] = Vec3{float64(x), float64(y), 0}
		}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			a := uint32(y*(n+1) + x)
			b := a + 1
			c := a + uint32(n+2)
			d := a + uint32(n+1)
			indices = append(indices, a, b, c, a, c, d)
		}
	}
	m, e := NewMesh(vertices, indices)
	if e != nil {
		t.Fatal(e)
	}
	return m
}

func TestRayIndexNearestOriginalIndicesAndBarycentric(t *testing.T) {
	box := Box()
	_, idx := rayIndexFor(t, Node{ID: "first", Mesh: box}, Node{ID: "same", Mesh: box}, Node{ID: "hidden", Mesh: box, Hidden: true})
	ray := Ray{Vec3{0, 0, 5}, Vec3{0, 0, -17}}
	hit, ok, stats, e := idx.Cast(t.Context(), ray, RayQuery{})
	if e != nil || !ok || hit.NodeID != "first" || hit.Instance != 0 || hit.Triangle != 2 || hit.Distance != 4 || hit.Position != (Vec3{0, 0, 1}) || hit.LocalPosition != hit.Position || hit.Normal != (Vec3{Z: 1}) || hit.Barycentric != (Vec3{X: .5, Z: .5}) || !hit.FrontFace {
		t.Fatal(hit, ok, e)
	}
	if stats.TriangleTests == 0 || idx.Stats().MeshBuilds != 1 || idx.Stats().Meshes != 1 || idx.Stats().Instances != 2 {
		t.Fatal(stats, idx.Stats())
	}
	for _, q := range []RayQuery{{MaxDistance: 3.999}, {MinDistance: 6.001}, {SkipNode: "first", MaxDistance: 3}} {
		_, ok, _, e := idx.Cast(t.Context(), ray, q)
		if e != nil || ok {
			t.Fatal(q, ok, e)
		}
	}
	hit, ok, _, e = idx.Cast(t.Context(), ray, RayQuery{SkipNode: "first", MinDistance: 4, MaxDistance: 4})
	if e != nil || !ok || hit.NodeID != "same" {
		t.Fatal(hit, ok, e)
	}
	hit, ok, _, e = idx.Cast(t.Context(), ray, RayQuery{MinDistance: 4.1})
	if e != nil || !ok || hit.Distance != 6 || hit.Triangle != 0 || hit.FrontFace {
		t.Fatal(hit, ok, e)
	}
	// Distance is independent of magnitude and accepts subnormal finite rays.
	for _, d := range []float64{-1e300, -1e-300, -math.SmallestNonzeroFloat64} {
		h, ok, _, e := idx.Cast(t.Context(), Ray{ray.Origin, Vec3{Z: d}}, RayQuery{})
		if e != nil || !ok || h.Distance != 4 {
			t.Fatal(d, h, e)
		}
	}
}

func TestRayIndexAffineReflectionSingularAndExtremeOrigins(t *testing.T) {
	mesh, e := NewMesh([]Vec3{{-1, -1, 0}, {1, -1, 0}, {0, 1, 0}}, []uint32{0, 1, 2})
	if e != nil {
		t.Fatal(e)
	}
	matrices := []Mat4{Identity(), Scaling(Vec3{-2, 3, 1}), Scaling(Vec3{2, 3, 0}), Scaling(Vec3{1e150, 1e150, 1e150}), Scaling(Vec3{1e-150, 1e-150, 1e-150}), Translation(Vec3{X: 1e300})}
	shear := Identity()
	shear[4] = 1.7
	shear[8] = .4
	shear[9] = -.2
	matrices = append(matrices, shear)
	for i, m := range matrices {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, idx := rayIndexFor(t, Node{ID: "parent", Transform: m}, Node{ID: "surface", Parent: "parent", Mesh: mesh})
			height := 1.0
			if i == 3 {
				height = 1e150
			}
			if i == 4 {
				height = 1e-150
			}
			ray := Ray{Origin: Vec3{X: m[12], Z: height}, Direction: Vec3{Z: -1}}
			h, ok, _, e := idx.Cast(t.Context(), ray, RayQuery{})
			if e != nil || !ok || math.Abs(h.Distance/height-1) > 1e-12 || !rayVecAlmost(h.LocalPosition, Vec3{}) {
				t.Fatal(h, ok, e)
			}
			if i == 1 {
				if h.FrontFace || h.Normal.Z != -1 {
					t.Fatal(h)
				}
				_, ok, _, e = idx.Cast(t.Context(), ray, RayQuery{CullBackfaces: true})
				if e != nil || ok {
					t.Fatal(ok, e)
				}
			}
			// Authored geometry and the scene revision are untouched by queries.
			if s.Revision() != 2 {
				t.Fatal(s.Revision())
			}
		})
	}
	_, flat := rayIndexFor(t, Node{ID: "line", Mesh: mesh, Transform: Scaling(Vec3{0, 1, 0})})
	_, ok, _, e := flat.Cast(t.Context(), Ray{Vec3{Z: 1}, Vec3{Z: -1}}, RayQuery{})
	if e != nil || ok {
		t.Fatal(ok, e)
	}
}

func TestRayIndexPositionRetainsSurfaceAtDistantRayOrigin(t *testing.T) {
	mesh, err := NewMesh([]Vec3{{-1, -1, 1}, {1, -1, 1}, {0, 1, 1}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, translation := range []Vec3{{}, {X: 1e300}, {Z: 7}} {
		_, idx := rayIndexFor(t, Node{ID: "surface", Mesh: mesh, Transform: Translation(translation)})
		h, ok, _, err := idx.Cast(t.Context(), Ray{Vec3{X: translation.X, Z: 1e300}, Vec3{Z: -1}}, RayQuery{})
		if err != nil || !ok || h.Position != (Vec3{X: translation.X, Z: 1 + translation.Z}) || h.LocalPosition != (Vec3{Z: 1}) {
			t.Fatal(translation, h, ok, err)
		}
	}
}

func TestRayIndexIndependentProjectionScales(t *testing.T) {
	mesh, err := NewMesh([]Vec3{{-1, -1, 0}, {1, -1, 0}, {0, 1, 0}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		scale    Vec3
		distance float64
	}{
		{"distant", Vec3{1, 1, 1}, 1e300},
		{"near_tiny", Vec3{1e-300, 1e-300, 1}, 1},
		{"anisotropic", Vec3{1e300, 1e-300, 1}, 1},
		{"anisotropic_swapped", Vec3{1e-300, 1e300, 1}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, idx := rayIndexFor(t, Node{ID: "triangle", Mesh: mesh, Transform: Scaling(tc.scale)})
			h, ok, _, err := idx.Cast(t.Context(), Ray{Vec3{Z: tc.distance}, Vec3{Z: -1}}, RayQuery{})
			if err != nil || !ok || math.Abs(h.Distance/tc.distance-1) > 1e-12 || h.Normal != (Vec3{Z: 1}) || h.Barycentric != (Vec3{.25, .25, .5}) {
				t.Fatal(h, ok, err)
			}
		})
	}
}

func TestRayIndexSharedEdgesVerticesParallelAndOnSurface(t *testing.T) {
	_, idx := rayIndexFor(t, Node{ID: "grid", Mesh: rayGrid(t, 12)})
	for x := 0; x <= 12; x++ {
		for y := 0; y <= 12; y++ {
			h, ok, _, e := idx.Cast(t.Context(), Ray{Vec3{float64(x), float64(y), 3}, Vec3{Z: -1}}, RayQuery{})
			if e != nil || !ok || h.Distance != 3 || !rayAlmost(h.Barycentric.X+h.Barycentric.Y+h.Barycentric.Z, 1) {
				t.Fatal(x, y, h, ok, e)
			}
		}
	}
	for x := .5; x < 12; x++ {
		h, ok, _, e := idx.Cast(t.Context(), Ray{Vec3{x, x, 3}, Vec3{Z: -1}}, RayQuery{})
		if e != nil || !ok || h.Triangle%2 != 0 {
			t.Fatal(x, h, e)
		}
	}
	for _, ray := range []Ray{{Vec3{X: 1, Y: 1}, Vec3{X: 1}}, {Vec3{X: 1, Y: 1, Z: 3}, Vec3{Z: 1}}, {Vec3{X: -.001, Y: 1, Z: 3}, Vec3{Z: -1}}} {
		_, ok, _, e := idx.Cast(t.Context(), ray, RayQuery{})
		if e != nil || ok {
			t.Fatal(ray, ok, e)
		}
	}
	h, ok, _, e := idx.Cast(t.Context(), Ray{Vec3{X: 1, Y: 1}, Vec3{Z: -1}}, RayQuery{})
	if e != nil || !ok || h.Distance != 0 {
		t.Fatal(h, ok, e)
	}
}

func TestRayIndexExtremeFiniteEdgesAndQueryCancellation(t *testing.T) {
	mesh, e := NewMesh([]Vec3{{-9e11, 0, 0}, {9e11, 0, 0}, {0, 9e11, 0}}, []uint32{0, 1, 2})
	if e != nil {
		t.Fatal(e)
	}
	_, idx := rayIndexFor(t, Node{ID: "wide", Mesh: mesh, Transform: Scaling(Vec3{1.5e296, 1.5e296, 1})})
	h, ok, _, e := idx.Cast(t.Context(), Ray{Vec3{Y: 5e307, Z: 1}, Vec3{Z: -1}}, RayQuery{})
	if e != nil || !ok || !rayAlmost(h.Distance, 1) || h.Normal != (Vec3{Z: 1}) {
		t.Fatal(h, ok, e)
	}
	_, grid := rayIndexFor(t, Node{ID: "grid", Mesh: rayGrid(t, 256)})
	stop := &rayCancelContext{Context: t.Context(), after: 3}
	h, ok, stats, e := grid.Cast(stop, Ray{Vec3{X: -1, Y: 128.3}, Vec3{X: 1}}, RayQuery{})
	if !errors.Is(e, context.Canceled) || ok || h != (RayHit{}) || stats.BoundsTests+stats.TriangleTests > 512 {
		t.Fatal(h, ok, stats, e, stop.calls)
	}
	h, ok, _, e = grid.Cast(t.Context(), Ray{Vec3{X: 1, Y: 1, Z: 5}, Vec3{Z: -1}}, RayQuery{})
	if e != nil || !ok || h.Distance != 5 {
		t.Fatal(h, ok, e)
	}
}

func TestRayIndexRefitCacheLifetimeAndConcurrentSnapshots(t *testing.T) {
	mesh := Box()
	nodes := make([]Node, 200)
	for i := range nodes {
		nodes[i] = Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: Translation(Vec3{X: float64(i) * 10})}
	}
	scene, old := rayIndexFor(t, nodes...)
	ray := Ray{Vec3{Z: 5}, Vec3{Z: -1}}
	same, e := old.WithSnapshot(t.Context(), old.Snapshot())
	if e != nil || same != old {
		t.Fatal(e)
	}
	if e = scene.SetColor("0", color.RGBA{255, 0, 0, 255}); e != nil {
		t.Fatal(e)
	}
	snapshot, e := scene.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	colored, e := old.WithSnapshot(t.Context(), snapshot)
	if e != nil || colored.top != old.top || colored.Stats().MeshBuilds != 0 || colored.Stats().MeshReuses != 1 || !colored.Stats().InstanceTreeReused {
		t.Fatal(e, colored.Stats())
	}
	if e = scene.SetTransform("0", Translation(Vec3{Z: -5})); e != nil {
		t.Fatal(e)
	}
	snapshot, e = scene.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	moved, e := colored.WithSnapshot(t.Context(), snapshot)
	if e != nil || moved.top == old.top || moved.meshes[mesh] != old.meshes[mesh] || moved.Stats().RefittedNodes == 0 {
		t.Fatal(e, moved.Stats())
	}
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				idx, want := old, 4.0
				if worker%2 == 1 {
					idx, want = moved, 9
				}
				h, ok, _, err := idx.Cast(context.Background(), ray, RayQuery{})
				if err != nil || !ok || h.Distance != want {
					t.Errorf("%v %v %v", h, ok, err)
					return
				}
				copy, err := idx.WithSnapshot(context.Background(), snapshot)
				if err != nil || copy.Snapshot() != snapshot {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if e = scene.SetVisible("0", false); e != nil {
		t.Fatal(e)
	}
	snapshot, e = scene.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	hidden, e := moved.WithSnapshot(t.Context(), snapshot)
	if e != nil {
		t.Fatal(e)
	}
	_, ok, _, e := hidden.Cast(t.Context(), ray, RayQuery{})
	if e != nil || ok {
		t.Fatal(ok, e)
	}
	empty := NewScene()
	snap, _ := empty.Snapshot()
	cleared, e := hidden.WithSnapshot(t.Context(), snap)
	if e != nil || len(cleared.meshes) != 0 || cleared.top != nil {
		t.Fatal(e)
	}
	// Updating the last owner never clears immutable resources held by old queries.
	h, ok, _, e := old.Cast(t.Context(), ray, RayQuery{})
	if e != nil || !ok || h.Distance != 4 {
		t.Fatal(h, ok, e)
	}
}

type rayCancelContext struct {
	context.Context
	calls, after int
}

func (c *rayCancelContext) Err() error {
	c.calls++
	if c.calls >= c.after {
		return context.Canceled
	}
	return nil
}

func TestRayIndexValidationBudgetsCancellationAndAtomicity(t *testing.T) {
	_, idx := rayIndexFor(t, Node{ID: "box", Mesh: Box()})
	snap := idx.Snapshot()
	for _, limits := range []RayIndexLimits{{MaxBytes: -1}, {MaxBytes: MaxRayIndexBytes + 1}, {MaxBytes: 1}} {
		if _, e := NewRayIndex(t.Context(), snap, limits); !errors.Is(e, ErrLimit) {
			t.Fatal(limits, e)
		}
	}
	if _, e := NewRayIndex(nil, snap, RayIndexLimits{}); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	if _, e := NewRayIndex(t.Context(), nil, RayIndexLimits{}); !errors.Is(e, ErrNode) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := idx.WithSnapshot(ctx, snap); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var nilIndex *RayIndex
	if _, _, _, e := nilIndex.Cast(t.Context(), Ray{}, RayQuery{}); !errors.Is(e, ErrNode) {
		t.Fatal(e)
	}
	valid := Ray{Vec3{Z: 5}, Vec3{Z: -1}}
	for _, ray := range []Ray{{}, {Origin: Vec3{X: math.Inf(1)}, Direction: Vec3{Z: 1}}, {Direction: Vec3{Z: math.NaN()}}} {
		_, ok, _, e := idx.Cast(t.Context(), ray, RayQuery{})
		if !errors.Is(e, ErrGeometry) || ok {
			t.Fatal(ok, e)
		}
	}
	for _, q := range []RayQuery{{MinDistance: -1}, {MaxDistance: -1}, {MinDistance: 2, MaxDistance: 1}, {MaxDistance: math.Inf(1)}, {MinDistance: math.NaN()}} {
		_, ok, _, e := idx.Cast(t.Context(), valid, q)
		if !errors.Is(e, ErrGeometry) || ok {
			t.Fatal(q, ok, e)
		}
	}
	if _, _, _, e := idx.Cast(nil, valid, RayQuery{}); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	if _, ok, _, e := idx.Cast(ctx, valid, RayQuery{}); !errors.Is(e, context.Canceled) || ok {
		t.Fatal(ok, e)
	}
	large := rayGrid(t, 180)
	scene, _ := rayIndexFor(t, Node{ID: "large", Mesh: large})
	largeSnap, _ := scene.Snapshot()
	stop := &rayCancelContext{Context: t.Context(), after: 12}
	if got, e := idx.WithSnapshot(stop, largeSnap); !errors.Is(e, context.Canceled) || got != nil {
		t.Fatal(got, e)
	}
	h, ok, _, e := idx.Cast(t.Context(), valid, RayQuery{})
	if e != nil || !ok || h.Distance != 4 {
		t.Fatal(h, ok, e)
	}
	bad := NewScene()
	_ = bad.Add(Node{ID: "empty", Mesh: &Mesh{}})
	bs, _ := bad.Snapshot()
	if _, e := NewRayIndex(t.Context(), bs, RayIndexLimits{}); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	overflow := Scaling(Vec3{math.MaxFloat64, math.MaxFloat64, math.MaxFloat64})
	overflow[12] = math.MaxFloat64
	_, over := rayIndexFor(t, Node{ID: "overflow", Mesh: Box(), Transform: overflow})
	// An origin difference that cannot be represented is an error, not a partial hit.
	_, ok, _, e = over.Cast(t.Context(), Ray{Vec3{X: -math.MaxFloat64}, Vec3{X: 1}}, RayQuery{})
	if !errors.Is(e, ErrGeometry) || ok {
		t.Fatal(ok, e)
	}
}

func TestRayIndexAccelerationAndZeroAllocation(t *testing.T) {
	_, idx := rayIndexFor(t, Node{ID: "grid", Mesh: rayGrid(t, 256)})
	ray := Ray{Vec3{123.25, 111.4, 5}, Vec3{Z: -1}}
	h, ok, stats, e := idx.Cast(t.Context(), ray, RayQuery{})
	if e != nil || !ok || stats.TriangleTests > 64 {
		t.Fatal(h, ok, stats, e)
	}
	allocations := testing.AllocsPerRun(100, func() { _, _, _, _ = idx.Cast(context.Background(), ray, RayQuery{}) })
	if allocations != 0 {
		t.Fatal(allocations)
	}
	nodes := make([]Node, MaxNodes)
	box := Box()
	for i := range nodes {
		nodes[i] = Node{ID: fmt.Sprint(i), Mesh: box, Transform: Translation(Vec3{X: float64(i) * 5})}
	}
	_, instances := rayIndexFor(t, nodes...)
	_, ok, stats, e = instances.Cast(t.Context(), Ray{Vec3{Z: 5}, Vec3{Z: -1}}, RayQuery{})
	if e != nil || !ok || stats.InstanceTests > 16 || stats.TriangleTests > 96 {
		t.Fatal(ok, stats, e)
	}
}

// Independent Moller-Trumbore reference for moderate, non-degenerate inputs;
// it does not use the production sheared-edge predicate or either BVH.
func rayOracle(s *Snapshot, ray Ray, q RayQuery) (RayHit, bool) {
	d, _ := projectionUnit(ray.Direction)
	best := q.MaxDistance
	if best == 0 {
		best = math.Inf(1)
	}
	var hit RayHit
	found := false
	for ii, in := range s.instances {
		if in.id == q.SkipNode {
			continue
		}
		m := in.mesh
		for k := 0; k < len(m.indices); k += 3 {
			a, b, c := in.world.Point(m.vertices[m.indices[k]]), in.world.Point(m.vertices[m.indices[k+1]]), in.world.Point(m.vertices[m.indices[k+2]])
			e1, e2 := b.Sub(a), c.Sub(a)
			p := d.Cross(e2)
			det := e1.Dot(p)
			if det == 0 {
				continue
			}
			offset := ray.Origin.Sub(a)
			u := offset.Dot(p) / det
			if u < 0 || u > 1 {
				continue
			}
			cross := offset.Cross(e1)
			v := d.Dot(cross) / det
			if v < 0 || u+v > 1 {
				continue
			}
			distance := e2.Dot(cross) / det
			if distance < q.MinDistance || distance > best || found && distance == best {
				continue
			}
			normal, _ := projectionUnit(e1.Cross(e2))
			front := normal.Dot(d) < 0
			if q.CullBackfaces && !front {
				continue
			}
			best = distance
			hit = RayHit{NodeID: in.id, Instance: ii, Triangle: k / 3, Distance: distance, Barycentric: Vec3{1 - u - v, u, v}, Position: ray.Origin.Add(d.Mul(distance)), Normal: normal, FrontFace: front}
			found = true
		}
	}
	return hit, found
}
func checkRayOracle(t testing.TB, idx *RayIndex, ray Ray, q RayQuery) {
	t.Helper()
	h, ok, _, e := idx.Cast(context.Background(), ray, q)
	want, found := rayOracle(idx.Snapshot(), ray, q)
	if e != nil || ok != found {
		t.Fatalf("found %v/%v error %v ray %+v", ok, found, e, ray)
	}
	if ok && (h.NodeID != want.NodeID || h.Triangle != want.Triangle || !rayAlmost(h.Distance, want.Distance) || !rayVecAlmost(h.Barycentric, want.Barycentric) || !rayVecAlmost(h.Normal, want.Normal) || h.FrontFace != want.FrontFace) {
		t.Fatalf("got %+v want %+v", h, want)
	}
}
func TestRayIndexDifferentialTransformedAndRefit(t *testing.T) {
	random := rand.New(rand.NewPCG(19, 31))
	nodes := make([]Node, 150)
	mesh := Box()
	for i := range nodes {
		m := RotationXYZ(Vec3{random.Float64(), random.Float64(), random.Float64()}).Mul(Scaling(Vec3{random.Float64()*2 + .1, random.Float64()*2 + .1, random.Float64()*2 + .1}))
		m[4] += .3
		m[0] *= -1
		m[12], m[13], m[14] = random.Float64()*30-15, random.Float64()*20-10, random.Float64()*20-10
		nodes[i] = Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: m}
	}
	scene, idx := rayIndexFor(t, nodes...)
	for i := range 1000 {
		ray := Ray{Vec3{random.Float64()*60 - 30, random.Float64()*60 - 30, 25}, Vec3{random.Float64() - .5, random.Float64() - .5, -1}}
		checkRayOracle(t, idx, ray, RayQuery{CullBackfaces: i%2 == 0})
	}
	old := idx
	for i := range nodes {
		m := nodes[i].Transform
		m[12] *= -1
		m[13] *= -1
		_ = scene.SetTransform(nodes[i].ID, m)
	}
	snap, _ := scene.Snapshot()
	var e error
	idx, e = idx.WithSnapshot(t.Context(), snap)
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(old.top.nodes, idx.top.nodes) {
		t.Fatal("refit did not change bounds")
	}
	for range 500 {
		checkRayOracle(t, idx, Ray{Vec3{random.Float64()*30 - 15, random.Float64()*20 - 10, 25}, Vec3{Z: -1}}, RayQuery{})
	}
}

func FuzzRayIndexIndependentOracle(f *testing.F) {
	for _, seed := range []uint64{0, 1, 89, 0xdeadbeef} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		nodes := make([]Node, 10)
		mesh := Box()
		for i := range nodes {
			nodes[i] = Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: Translation(Vec3{rng.Float64()*10 - 5, rng.Float64()*10 - 5, rng.Float64()*10 - 5}).Mul(RotationXYZ(Vec3{rng.Float64() * 3, rng.Float64() * 3, rng.Float64() * 3}))}
		}
		_, idx := rayIndexFor(t, nodes...)
		for range 6 {
			checkRayOracle(t, idx, Ray{Vec3{rng.Float64()*16 - 8, rng.Float64()*16 - 8, 10}, Vec3{rng.Float64() - .5, rng.Float64() - .5, -1}}, RayQuery{})
		}
	})
}
func BenchmarkRayIndexGrid131072(b *testing.B) {
	_, idx := rayIndexFor(b, Node{ID: "grid", Mesh: rayGrid(b, 256)})
	ray := Ray{Vec3{123.25, 111.4, 5}, Vec3{Z: -1}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h, ok, _, e := idx.Cast(context.Background(), ray, RayQuery{})
		if e != nil || !ok || h.NodeID != "grid" {
			b.Fatal(h, ok, e)
		}
	}
}

func BenchmarkRayIndexLinearGrid131072(b *testing.B) {
	_, idx := rayIndexFor(b, Node{ID: "grid", Mesh: rayGrid(b, 256)})
	ray := Ray{Vec3{123.25, 111.4, 5}, Vec3{Z: -1}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h, ok := rayOracle(idx.Snapshot(), ray, RayQuery{})
		if !ok || h.NodeID != "grid" {
			b.Fatal(h, ok)
		}
	}
}

func FuzzRayIndexScaleSeparation(f *testing.F) {
	for _, seed := range [][3]int16{{0, 0, 0}, {996, -996, 0}, {-996, -996, 996}, {996, 996, -996}} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, xe, ye, ze int16) {
		sx, sy, depth := math.Ldexp(1, int(xe)%1001), math.Ldexp(1, int(ye)%1001), math.Ldexp(1, int(ze)%1001)
		mesh, e := NewMesh([]Vec3{{-1, -1, 0}, {1, -1, 0}, {0, 1, 0}}, []uint32{0, 1, 2})
		if e != nil {
			t.Fatal(e)
		}
		_, idx := rayIndexFor(t, Node{ID: "scaled", Mesh: mesh, Transform: Scaling(Vec3{sx, sy, 1})})
		h, ok, _, e := idx.Cast(t.Context(), Ray{Vec3{X: sx * .25, Z: depth}, Vec3{Z: -1}}, RayQuery{})
		if e != nil || !ok || math.Abs(h.Distance/depth-1) > 1e-12 || h.Normal != (Vec3{Z: 1}) || h.Barycentric != (Vec3{.125, .375, .5}) || h.LocalPosition != (Vec3{X: .25}) {
			t.Fatal(xe, ye, ze, h, ok, e)
		}
	})
}

func ExampleRayIndex() {
	scene := NewScene()
	_ = scene.Add(Node{ID: "pump", Mesh: Box()})
	snapshot, _ := scene.Snapshot()                                           // Capture on the scene owner thread.
	index, _ := NewRayIndex(context.Background(), snapshot, RayIndexLimits{}) // May run on a worker.
	hit, found, _, err := index.Cast(context.Background(), Ray{Vec3{Z: 5}, Vec3{Z: -1}}, RayQuery{})
	fmt.Println(hit.NodeID, found, hit.Distance, hit.Position, hit.Normal, err)
	// Output: pump true 4 {0 0 1} {0 0 1} <nil>
}
