package scene3d

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"testing"
)

func linearVisible(dst []int, s *Snapshot, f Frustum) []int {
	dst = dst[:0]
	for i, in := range s.instances {
		if f.Intersects(in.mesh, in.world) {
			dst = append(dst, i)
		}
	}
	return dst
}

func cullingGrid(t testing.TB, count int) (*Scene, *Snapshot) {
	t.Helper()
	s := NewScene()
	box := Box()
	for i := 0; i < count; i++ {
		pose := Translation(Vec3{X: 1000 + float64(i%100)*3, Y: float64(i/100) * 3})
		if i == count/2 {
			pose = Identity()
		}
		if err := s.Add(Node{ID: fmt.Sprint(i), Mesh: box, Transform: pose}); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return s, snap
}

func assertVisibleMatches(t testing.TB, q *Culler, s *Snapshot, f Frustum) CullingStats {
	t.Helper()
	want := linearVisible(nil, s, f)
	got, stats := q.Cull(nil, s, f)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered culling mismatch: got=%v want=%v stats=%+v", got, want, stats)
	}
	triangles := 0
	for _, in := range s.instances {
		triangles += in.mesh.TriangleCount()
	}
	for _, i := range got {
		triangles -= s.instances[i].mesh.TriangleCount()
	}
	if stats.CulledInstances != s.InstanceCount()-len(got) || stats.CulledTriangles != triangles {
		t.Fatal("incorrect skipped-work statistics", stats, triangles)
	}
	return stats
}

func TestCullerOrderedHierarchyRefitAndOwnership(t *testing.T) {
	s, snap := cullingGrid(t, MaxNodes)
	f := NewFrustum(Identity())
	var q Culler
	first := assertVisibleMatches(t, &q, snap, f)
	if first.HierarchyNodes == 0 || first.RefittedNodes == 0 || first.InstanceTests > 16 || first.BoundsTests > 32 || first.CulledInstances != MaxNodes-1 {
		t.Fatal("sparse camera query did not prune subtrees", first)
	}
	buffer := make([]int, 0, MaxNodes)
	if allocations := testing.AllocsPerRun(100, func() { buffer, _ = q.Cull(buffer, snap, f) }); allocations != 0 {
		t.Fatal("camera-only query allocated", allocations)
	}
	second := assertVisibleMatches(t, &q, snap, f)
	if second.RefittedNodes != 0 {
		t.Fatal("immutable snapshot rebuilt", second)
	}
	if err := s.SetColor("5000", color.RGBA{R: 250, A: 255}); err != nil {
		t.Fatal(err)
	}
	colored := snapshot(t, s)
	if stats := assertVisibleMatches(t, &q, colored, f); stats.RefittedNodes != 0 {
		t.Fatal("color-only edit rebuilt spatial bounds", stats)
	}
	if err := s.SetTransform("5000", Translation(Vec3{X: 3000})); err != nil {
		t.Fatal(err)
	}
	moved := snapshot(t, s)
	stats := assertVisibleMatches(t, &q, moved, f)
	if stats.RefittedNodes <= 0 || stats.RefittedNodes > 13 || stats.InstanceTests != 0 || stats.BoundsTests != 1 {
		t.Fatal("single leaf did not refit only its ancestor chain", stats)
	}
	if in, _ := snap.Instance(5000); in.Transform != Identity() {
		t.Fatal("old snapshot changed")
	}
	assertVisibleMatches(t, &q, snap, f) // old snapshots remain independently usable
	if err := s.SetVisible("100", false); err != nil {
		t.Fatal(err)
	}
	if stats := assertVisibleMatches(t, &q, snapshot(t, s), f); stats.RefittedNodes == 0 {
		t.Fatal("count change failed to rebuild", stats)
	}
	q.Reset()
	if q.snapshot != nil || q.nodes != nil || q.leaves != 0 || q.order != nil || q.centroids != nil || q.visible != nil {
		t.Fatal("Reset retained snapshot/index")
	}
	if got, stats := q.Cull([]int{42}, nil, f); len(got) != 0 || stats != (CullingStats{}) {
		t.Fatal("nil query retained output", got, stats)
	}
	_, small := cullingGrid(t, 7)
	if stats := assertVisibleMatches(t, &q, small, f); stats.HierarchyNodes != 0 || stats.InstanceTests != 7 {
		t.Fatal("small scene did not use linear fallback", stats)
	}
}

func TestCullerAllInsideOrderAndMeshReplacement(t *testing.T) {
	s := NewScene()
	mesh := Box()
	for i := 0; i < 257; i++ {
		if err := s.Add(Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: Scaling(Vec3{X: .1, Y: .1, Z: .1})}); err != nil {
			t.Fatal(err)
		}
	}
	f := NewFrustum(Identity())
	var q Culler
	old := snapshot(t, s)
	stats := assertVisibleMatches(t, &q, old, f)
	if stats.BoundsTests != 1 || stats.InstanceTests != 0 || stats.CulledInstances != 0 {
		t.Fatal("fully inside tree was not accepted as one range", stats)
	}
	// Changing mesh identity must refit even if transforms and count match.
	changed := &Snapshot{instances: append([]instance(nil), old.instances...)}
	copy := *mesh
	copy.vertices = append([]Vec3(nil), mesh.vertices...)
	copy.min.X, copy.max.X = 100, 110
	changed.instances[256].mesh = &copy
	stats = assertVisibleMatches(t, &q, changed, f)
	if stats.RefittedNodes == 0 || stats.CulledInstances != 1 {
		t.Fatal("mesh replacement kept stale bounds", stats)
	}
	// Metadata-only replacement preserves index geometry but returns new IDs.
	metadata := &Snapshot{instances: append([]instance(nil), changed.instances...)}
	metadata.instances[0].id = "new-id"
	if stats := assertVisibleMatches(t, &q, metadata, f); stats.RefittedNodes != 0 {
		t.Fatal("ID edit refitted geometry", stats)
	}
}

func TestCullerRandomizedExactLinearAndRasterParity(t *testing.T) {
	rng := rand.New(rand.NewSource(51293))
	mesh := Box()
	var q Culler
	for run := 0; run < 240; run++ {
		s := NewScene()
		for i := 0; i < 64+run%193; i++ {
			pose := Translation(Vec3{rng.Float64()*100 - 50, rng.Float64()*60 - 30, rng.Float64()*80 - 40}).Mul(RotationXYZ(Vec3{rng.Float64() * 6, rng.Float64() * 6, rng.Float64() * 6})).Mul(Scaling(Vec3{rng.Float64()*6 - 3, rng.Float64()*4 - 2, rng.Float64() * 5}))
			pose[4] += .7
			if err := s.Add(Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: pose, Color: color.RGBA{byte(i * 5), byte(i * 11), byte(i * 23), 255}}); err != nil {
				t.Fatal(err)
			}
		}
		snap := snapshot(t, s)
		for camera := 0; camera < 5; camera++ {
			c := testCamera()
			c.Eye = Vec3{rng.Float64()*6 - 3, rng.Float64()*6 - 3, rng.Float64()*5 + 3}
			c.Near, c.Far = .3, 80
			vp, err := c.ViewProjection(.5 + rng.Float64()*3)
			if err != nil {
				t.Fatal(err)
			}
			assertVisibleMatches(t, &q, snap, NewFrustum(vp))
			if camera == 0 && run < 32 {
				checkCullingParity(t, snap, c) // complete RGBA, depth and stable IDs
			}
		}
	}
}

func TestCullerUncertainAndCancellationBoundsFailOpen(t *testing.T) {
	mesh := Box()
	badBounds := *mesh
	badBounds.min.X = math.NaN()
	poses := []Mat4{Identity(), Translation(Vec3{X: 1e100}), Scaling(Vec3{X: 0, Y: 0, Z: 0}), Translation(Vec3{X: 2 + 1e-7})}
	cancel := Identity()
	cancel[0], cancel[4], cancel[12] = 1e30, -1e30, 4
	poses = append(poses, cancel)
	vps := []Mat4{Identity(), {}, Scaling(Vec3{X: 1e38, Y: 1, Z: 1}), Scaling(Vec3{X: 1, Y: 1, Z: 1e38})}
	for _, pose := range poses {
		for _, vp := range vps {
			s := &Snapshot{instances: make([]instance, 129)}
			for i := range s.instances {
				s.instances[i] = instance{id: fmt.Sprint(i), mesh: mesh, world: pose}
			}
			s.instances[0].mesh = &badBounds
			s.instances[127].mesh = nil
			var q Culler
			assertVisibleMatches(t, &q, s, NewFrustum(vp))
		}
	}
	// Expose cancellation in model vertices as well as transforms. Bounds may
	// collapse to a small world box but tolerance must retain their large inputs.
	m, err := NewMesh([]Vec3{{1e10, 1e10, 0}, {1e10 + .001, 1e10, 0}, {1e10, 1e10 + .001, 0}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	world := Identity()
	world[0], world[4], world[12] = 1e8, -1e8, 3
	s := &Snapshot{instances: make([]instance, 64)}
	for i := range s.instances {
		s.instances[i] = instance{mesh: m, world: world}
	}
	var q Culler
	assertVisibleMatches(t, &q, s, NewFrustum(Identity()))
}

func TestCullerSharedSnapshotIndependentOwners(t *testing.T) {
	_, s := cullingGrid(t, 1000)
	f := NewFrustum(Identity())
	var wg sync.WaitGroup
	for owner := 0; owner < 16; owner++ {
		wg.Go(func() {
			var q Culler
			for i := 0; i < 30; i++ {
				got, stats := q.Cull(nil, s, f)
				if len(got) != 1 || got[0] != 500 || stats.CulledInstances != 999 {
					t.Error("owner interference", got, stats)
					return
				}
			}
		})
	}
	wg.Wait()
}

func FuzzCullerConservativeOrder(f *testing.F) {
	for _, v := range []float64{0, 1, -1, 1e-12, 1e12, 1e30, 1e100, math.MaxFloat64, math.NaN()} {
		f.Add(v, v, 0., 1., -1., 1.)
	}
	f.Fuzz(func(t *testing.T, x, y, z, sx, shear, projection float64) {
		world := Identity()
		world[0], world[4], world[12], world[13], world[14] = sx, shear, x, y, z
		vp := Identity()
		vp[0], vp[8] = projection, shear
		mesh := Box()
		s := &Snapshot{instances: make([]instance, 80)}
		for i := range s.instances {
			s.instances[i] = instance{id: fmt.Sprint(i), mesh: mesh, world: world}
			if i%7 == 0 {
				s.instances[i].world = Translation(Vec3{X: float64(i)})
			}
		}
		var q Culler
		assertVisibleMatches(t, &q, s, NewFrustum(vp))
	})
}

func TestCullerInterleavedOrderBoundsOverheadAndOverflow(t *testing.T) {
	_, s := cullingGrid(t, MaxNodes)
	for i := range s.instances {
		if i%2 == 0 {
			s.instances[i].world = Translation(Vec3{X: -10000})
		}
	}
	var q Culler
	stats := assertVisibleMatches(t, &q, s, NewFrustum(Identity()))
	if stats.LinearFallbackInstances != 0 || stats.InstanceTests > 16 || stats.BoundsTests > 32 {
		t.Fatal("interleaved input was not spatially grouped", stats)
	}
	overlap := &Snapshot{instances: append([]instance(nil), s.instances...)}
	for i := range overlap.instances {
		overlap.instances[i].world = Scaling(Vec3{X: 1e4, Y: 1e4, Z: 1e4})
	}
	stats = assertVisibleMatches(t, &q, overlap, NewFrustum(Identity()))
	if stats.LinearFallbackInstances == 0 || stats.BoundsTests > MaxNodes/64 {
		t.Fatal("ineffective overlapping traversal was not bounded", stats)
	}
	mesh, err := NewMesh([]Vec3{{0, 2, 0}, {1, 3, 0}, {0, 3, 1}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	bad := &Snapshot{instances: append([]instance(nil), s.instances...)}
	bad.instances[5000].mesh = mesh
	bad.instances[5000].world = Scaling(Vec3{X: 1, Y: math.MaxFloat64 / 2, Z: 1})
	bad.instances[5000].world[12] = 1e20
	var renderer Renderer
	if _, err := renderer.Render(bad, testCamera(), 32, 32, color.RGBA{}); !errors.Is(err, ErrGeometry) {
		t.Fatal("BVH concealed overflowing offscreen geometry", err)
	}
}

func BenchmarkCullerOrderedQueries(b *testing.B) {
	_, sparse := cullingGrid(b, MaxNodes)
	dense := &Snapshot{instances: append([]instance(nil), sparse.instances...)}
	for i := range dense.instances {
		dense.instances[i].world = Scaling(Vec3{X: .1, Y: .1, Z: .1})
	}
	interleaved := &Snapshot{instances: append([]instance(nil), sparse.instances...)}
	for i := range interleaved.instances {
		if i%2 == 0 && i != MaxNodes/2 {
			interleaved.instances[i].world = Translation(Vec3{X: -10000})
		}
	}
	for _, tc := range []struct {
		name  string
		scene *Snapshot
	}{{"sparse", sparse}, {"dense", dense}, {"interleaved", interleaved}} {
		for _, hierarchy := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/hierarchy=%v", tc.name, hierarchy), func(b *testing.B) {
				var q Culler
				dst := make([]int, 0, MaxNodes)
				f := NewFrustum(Identity())
				q.Cull(dst, tc.scene, f)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if hierarchy {
						dst, _ = q.Cull(dst, tc.scene, f)
					} else {
						dst = linearVisible(dst, tc.scene, f)
					}
				}
				if len(dst) == 0 {
					b.Fatal("empty fixture")
				}
			})
		}
	}
}

func TestCullerResizeBitsetAndCallerStorage(t *testing.T) {
	var q Culler
	buf := make([]int, 0, MaxNodes)
	for _, n := range []int{0, 1, 63, 64, 65, 4096, 257, 1024, 128, 7, 129} {
		_, s := cullingGrid(t, n)
		want := linearVisible(nil, s, NewFrustum(Identity()))
		got, _ := q.Cull(buf, s, NewFrustum(Identity()))
		if len(got) != len(want) {
			t.Fatal("stale trailing bitmap words", n, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatal("wrong ordered ID", n, got, want)
			}
			got[i] = -1
		}
		got, _ = q.Cull(got, s, NewFrustum(Identity()))
		for i := range got {
			if got[i] != want[i] {
				t.Fatal("caller storage poisoned internal index", n)
			}
		}
		buf = got
	}
}

func BenchmarkCullerSnapshotUpdates(b *testing.B) {
	_, original := cullingGrid(b, MaxNodes)
	colored := &Snapshot{instances: append([]instance(nil), original.instances...)}
	colored.instances[5000].color.R = 233
	moved := &Snapshot{instances: append([]instance(nil), original.instances...)}
	moved.instances[9999].world = Translation(Vec3{X: 99999})
	for _, tc := range []struct {
		name    string
		changed *Snapshot
		cold    bool
	}{{"color-only", colored, false}, {"one-transform", moved, false}, {"cold-build", original, true}} {
		b.Run(tc.name, func(b *testing.B) {
			var q Culler
			dst := make([]int, 0, MaxNodes)
			f := NewFrustum(Identity())
			q.Cull(dst, original, f)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s := original
				if i%2 == 0 {
					s = tc.changed
				}
				if tc.cold {
					q.Reset()
				}
				dst, _ = q.Cull(dst, s, f)
			}
			if len(dst) != 1 {
				b.Fatal("fixture changed")
			}
		})
	}
}
