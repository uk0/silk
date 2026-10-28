package scene3d

import (
	"context"
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"
)

func testCamera() Camera {
	return Camera{Eye: Vec3{0, 0, 5}, Up: Vec3{0, 1, 0}, FOVY: math.Pi / 2, Near: .1, Far: 100}
}
func triangle(t *testing.T, z float64) *Mesh {
	t.Helper()
	m, e := NewMesh([]Vec3{{-1, -1, z}, {1, -1, z}, {0, 1, z}}, []uint32{0, 1, 2})
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func snapshot(t *testing.T, s *Scene) *Snapshot {
	t.Helper()
	p, e := s.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestTransformsHierarchyAndSnapshotIsolation(t *testing.T) {
	s := NewScene()
	if err := s.Add(Node{ID: "root", Transform: Translation(Vec3{1, 2, 3})}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Node{ID: "mesh", Parent: "root", Mesh: Box(), Transform: Scaling(Vec3{2, 2, 2})}); err != nil {
		t.Fatal(err)
	}
	old := snapshot(t, s)
	if got := old.instances[0].world.Point(Vec3{1, 0, 0}); got != (Vec3{3, 2, 3}) {
		t.Fatal(got)
	}
	rev := s.Revision()
	_ = s.SetTransform("root", Translation(Vec3{1, 2, 3}))
	if s.Revision() != rev || snapshot(t, s) != old {
		t.Fatal("unchanged scene invalidated cache")
	}
	_ = s.SetVisible("root", false)
	if snapshot(t, s).InstanceCount() != 0 || old.InstanceCount() != 1 {
		t.Fatal("visibility or snapshot isolation")
	}
	if !s.Remove("root") || len(s.nodes) != 0 || len(s.order) != 0 || s.Remove("root") {
		t.Fatal("subtree removal")
	}
	if err := s.Add(Node{ID: "orphan", Parent: "gone"}); !errors.Is(err, ErrNode) {
		t.Fatal(err)
	}
}
func TestMeshCopiesAndRejectsInvalidInputs(t *testing.T) {
	v := []Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}
	ix := []uint32{0, 1, 2}
	m, e := NewMesh(v, ix)
	if e != nil {
		t.Fatal(e)
	}
	v[0].X = 20
	ix[0] = 99
	if m.vertices[0].X != 0 || m.indices[0] != 0 {
		t.Fatal("mutable input escaped")
	}
	for _, c := range []struct {
		v  []Vec3
		ix []uint32
	}{{nil, nil}, {v, []uint32{0, 1}}, {v, []uint32{0, 1, 4}}, {[]Vec3{{math.NaN(), 0, 0}}, []uint32{0, 0, 0}}} {
		if _, e := NewMesh(c.v, c.ix); e == nil {
			t.Fatal("accepted invalid geometry")
		}
	}
}
func TestDepthPickingCacheAndVisibility(t *testing.T) {
	s := NewScene()
	_ = s.Add(Node{ID: "near", Mesh: triangle(t, 1), Color: color.RGBA{255, 0, 0, 255}})
	_ = s.Add(Node{ID: "far", Mesh: triangle(t, 0), Color: color.RGBA{0, 255, 0, 255}})
	var r Renderer
	f, e := r.Render(snapshot(t, s), testCamera(), 128, 128, color.RGBA{})
	if e != nil {
		t.Fatal(e)
	}
	if f.Pick(64, 64) != "near" || f.Pick(-1, 0) != "" || f.Pick(0, 0) != "" {
		t.Fatal("z-buffer/pick", f.Pick(64, 64))
	}
	if f.Image.RGBAAt(64, 64).R == 0 || f.Stats.Pixels == 0 {
		t.Fatal("missing visible pixels")
	}
	f, e = r.Render(snapshot(t, s), testCamera(), 128, 128, color.RGBA{})
	if e != nil || !f.Stats.Cached {
		t.Fatal("frame cache", e)
	}
	_ = s.SetVisible("near", false)
	f, e = r.Render(snapshot(t, s), testCamera(), 128, 128, color.RGBA{})
	if e != nil || f.Stats.Cached || f.Pick(64, 64) != "far" {
		t.Fatal("stale frame/pick", e)
	}
	allocations := testing.AllocsPerRun(100, func() { _, _ = r.Render(snapshot(t, s), testCamera(), 128, 128, color.RGBA{}) })
	if allocations != 0 {
		t.Fatal("cached frame allocated", allocations)
	}
}
func TestClipNearPlaneAndRejectBehindCamera(t *testing.T) {
	c := testCamera()
	c.Eye = Vec3{}
	c.Target = Vec3{0, 0, -1}
	c.Near = 1
	m, e := NewMesh([]Vec3{{-.8, -.8, -2}, {.8, -.8, -2}, {0, .8, -.5}}, []uint32{0, 1, 2})
	if e != nil {
		t.Fatal(e)
	}
	s := NewScene()
	_ = s.Add(Node{ID: "clipped", Mesh: m})
	var r Renderer
	f, e := r.Render(snapshot(t, s), c, 128, 128, color.RGBA{})
	if e != nil || f.Stats.Clipped != 1 || f.Stats.Pixels == 0 {
		t.Fatal(f.Stats, e)
	}
	_ = s.SetTransform("clipped", Translation(Vec3{0, 0, 5}))
	f, e = r.Render(snapshot(t, s), c, 128, 128, color.RGBA{})
	if e != nil || f.Stats.Pixels != 0 {
		t.Fatal("geometry behind eye rendered", e)
	}
}
func TestRenderLimitsAndCameraValidation(t *testing.T) {
	var r Renderer
	s := snapshot(t, NewScene())
	c := testCamera()
	for _, size := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {MaxDimension + 1, 1}, {8192, 8192}} {
		if _, e := r.Render(s, c, size[0], size[1], color.RGBA{}); !errors.Is(e, ErrLimit) {
			t.Fatal(size, e)
		}
	}
	for _, bad := range []Camera{{}, {Eye: Vec3{0, 0, 1}, Target: Vec3{}, Up: Vec3{0, 0, 1}, FOVY: 1, Near: 1, Far: 5}, {Eye: c.Eye, Up: c.Up, FOVY: math.NaN(), Near: .1, Far: 1}} {
		if _, e := r.Render(s, bad, 16, 16, color.RGBA{}); !errors.Is(e, ErrCamera) {
			t.Fatal(e)
		}
	}
}

const objQuad = "v -1 -1 0\nv 1 -1 0\nv 1 1 0\nv -1 1 0\nf -4 -3 -2 -1\n"

func TestOBJLimitsCancellationAndFeatures(t *testing.T) {
	m, e := LoadOBJ(context.Background(), strings.NewReader(objQuad), LoadLimits{})
	if e != nil || m.Mesh.TriangleCount() != 2 || m.Mesh.VertexCount() != 4 {
		t.Fatal(m, e)
	}
	for _, s := range []string{"v NaN 0 0\nf 1 1 1", "v 0 0 0\nf 0 1 1", "v 0 0 0\nf 1/1 1/1 1/1", objQuad + "curv 1 2\n", "v -1 -1 0\nv 1 1 0\nv 1 -1 0\nv -1 1 0\nf 1 2 3 4\n"} {
		if _, e := LoadOBJ(context.Background(), strings.NewReader(s), LoadLimits{}); e == nil {
			t.Fatal("bad model accepted", s)
		}
	}
	for _, l := range []LoadLimits{{MaxBytes: 8}, {MaxVertices: 3}, {MaxTriangles: 1}, {MaxLineBytes: 3}} {
		if _, e := LoadOBJ(context.Background(), strings.NewReader(objQuad), l); e == nil {
			t.Fatal("budget ignored", l)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := LoadOBJ(ctx, strings.NewReader(objQuad), LoadLimits{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	m, e = LoadOBJ(context.Background(), strings.NewReader("mtllib ../../not-opened.mtl\n"+objQuad), LoadLimits{})
	if e != nil || len(m.Warnings) != 1 {
		t.Fatal(m, e)
	}
}
func FuzzOBJBounded(f *testing.F) {
	f.Add(objQuad)
	f.Add("v 0 0 0\nf -1 -1 -1")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 65536 {
			t.Skip()
		}
		_, _ = LoadOBJ(context.Background(), strings.NewReader(s), LoadLimits{MaxBytes: 65536, MaxLineBytes: 8192, MaxVertices: 512, MaxTriangles: 512})
	})
}
func BenchmarkCachedFrame(b *testing.B) {
	s := NewScene()
	_ = s.Add(Node{ID: "box", Mesh: Box()})
	snap, _ := s.Snapshot()
	var r Renderer
	_, _ = r.Render(snap, DefaultCamera(), 640, 480, color.RGBA{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Render(snap, DefaultCamera(), 640, 480, color.RGBA{})
	}
}
func BenchmarkRenderBoxes(b *testing.B) {
	s := NewScene()
	mesh := Box()
	for i := 0; i < 100; i++ {
		_ = s.Add(Node{ID: fmtInt(i), Mesh: mesh, Transform: Translation(Vec3{float64(i%10) - 5, 0, float64(i/10) - 5}).Mul(Scaling(Vec3{.3, .3, .3}))})
	}
	snap, _ := s.Snapshot()
	var r Renderer
	c := DefaultCamera()
	c.Eye = Vec3{12, 12, 18}
	_, _ = r.Render(snap, c, 640, 480, color.RGBA{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Eye.X += 1e-6
		_, _ = r.Render(snap, c, 640, 480, color.RGBA{})
	}
}
func fmtInt(i int) string { return string(rune(i + 1)) }

func TestCylinderTopologyBoundsAndLimits(t *testing.T) {
	m, e := Cylinder(2, 6, 32)
	if e != nil {
		t.Fatal(e)
	}
	lo, hi := m.Bounds()
	if lo != (Vec3{-2, -3, -2}) || hi != (Vec3{2, 3, 2}) || m.VertexCount() != 66 || m.TriangleCount() != 128 {
		t.Fatal(lo, hi, m.VertexCount(), m.TriangleCount())
	}
	edges := map[[2]uint32]int{}
	for i := 0; i < len(m.indices); i += 3 {
		a, b, c := m.indices[i], m.indices[i+1], m.indices[i+2]
		for _, edge := range [][2]uint32{{a, b}, {b, c}, {c, a}} {
			if edge[0] > edge[1] {
				edge[0], edge[1] = edge[1], edge[0]
			}
			edges[edge]++
		}
	}
	for edge, n := range edges {
		if n != 2 {
			t.Fatal("non-manifold cylinder edge", edge, n)
		}
	}
	if _, e := Cylinder(-1, 2, 32); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	if _, e := Cylinder(1, 2, 100000); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}

func TestSnapshotBudgetsCountInstancesAndRecover(t *testing.T) {
	// A valid mesh may repeat indices. Sharing must not bypass CPU work limits.
	indices := make([]uint32, MaxTriangles*3)
	for i := range indices {
		indices[i] = uint32(i % 3)
	}
	mesh, err := NewMesh([]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, indices)
	if err != nil {
		t.Fatal(err)
	}
	s := NewScene()
	for _, id := range []string{"a", "b", "c"} {
		if err := s.Add(Node{ID: id, Mesh: mesh}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Snapshot(); !errors.Is(err, ErrLimit) {
		t.Fatal("instance budget bypass", err)
	}
	_ = s.SetVisible("c", false)
	if snap, err := s.Snapshot(); err != nil || snap.InstanceCount() != 2 {
		t.Fatal("budget recovery", err)
	}
}
