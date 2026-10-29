package scene3d

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"testing"
)

func TestFrustumClipPlanesAndConservativeBoundaries(t *testing.T) {
	m, err := NewMesh([]Vec3{{-.25, -.25, -.25}, {.25, .25, .25}, {-.25, .25, .25}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	f := NewFrustum(Identity())
	for _, tc := range []struct {
		name      string
		transform Mat4
		visible   bool
	}{
		{"inside", Identity(), true},
		{"left", Translation(Vec3{X: -2}), false}, {"right", Translation(Vec3{X: 2}), false},
		{"bottom", Translation(Vec3{Y: -2}), false}, {"top", Translation(Vec3{Y: 2}), false},
		{"near", Translation(Vec3{Z: -2}), false}, {"far", Translation(Vec3{Z: 2}), false},
		{"touching", Translation(Vec3{X: 1.25}), true},
		{"float32-edge", Translation(Vec3{X: 1.25 + 1e-7}), true},
		{"clearly-outside", Translation(Vec3{X: 1.251}), false},
		{"reflected", Scaling(Vec3{X: -1, Y: 1, Z: 1}), true},
		{"singular-inside", Scaling(Vec3{}), true},
		{"encloses-camera", Scaling(Vec3{X: 1e4, Y: 1e4, Z: 1e4}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.Intersects(m, tc.transform); got != tc.visible {
				t.Fatal(got, tc.visible)
			}
		})
	}
	shear := Identity()
	shear[4] = 3
	shear[12] = 2
	if !f.Intersects(m, shear) {
		t.Fatal("sheared box touching the plane rejected")
	}
	c := testCamera()
	c.Eye = Vec3{}
	c.Target = Vec3{Z: -1}
	vp, err := c.ViewProjection(1)
	if err != nil {
		t.Fatal(err)
	}
	f = NewFrustum(vp)
	if f.Intersects(m, Translation(Vec3{Z: 2})) {
		t.Fatal("behind eye retained")
	}
	if !f.Intersects(m, Translation(Vec3{Z: -.1})) {
		t.Fatal("near-plane crossing rejected")
	}
	if f.Intersects(nil, Identity()) || f.Intersects(&Mesh{}, Identity()) {
		t.Fatal("empty geometry reported")
	}
}

func TestFrustumUncertainInputFailsOpen(t *testing.T) {
	m := Box()
	f := NewFrustum(Identity())
	outside := Translation(Vec3{X: 1e8})
	if f.Intersects(m, outside) {
		t.Fatal("fixture not outside")
	}
	invalid := Identity()
	invalid[0] = math.NaN()
	for _, matrix := range []Mat4{invalid, {}, {0: math.Inf(1)}} {
		unknown := NewFrustum(matrix)
		if !unknown.Intersects(m, outside) {
			t.Fatal("invalid/zero projection hid geometry")
		}
	}
	if !((*Frustum)(nil)).Intersects(m, outside) {
		t.Fatal("nil frustum is not fail-open")
	}
	if !f.Intersects(m, invalid) {
		t.Fatal("invalid transform hid geometry")
	}
	broad := *m
	broad.min.X = math.NaN()
	if !f.Intersects(&broad, outside) {
		t.Fatal("invalid mesh bounds hid geometry")
	}
	// X is definitely outside, but another component overflows. Do not allow an
	// early X rejection to mask the renderer's existing geometry validation.
	extreme := outside
	extreme[5] = math.MaxFloat64
	huge := *m
	huge.min.Y = -2
	huge.max.Y = 2
	if !f.Intersects(&huge, extreme) {
		t.Fatal("world overflow concealed by culling")
	}
	vp := Identity()
	vp[10] = math.MaxFloat64 / 2
	projection := NewFrustum(vp)
	extreme = outside
	extreme[14] = 4
	if !projection.Intersects(m, extreme) {
		t.Fatal("clip overflow concealed by culling")
	}
	if allocs := testing.AllocsPerRun(1000, func() { f.Intersects(m, outside) }); allocs != 0 {
		t.Fatal("culling allocated", allocs)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if f.Intersects(m, outside) {
					t.Error("shared frustum mutated")
				}
			}
		}()
	}
	wg.Wait()
}

// Bounds are the only acceleration input. Test-only unknown bounds keep the
// original vertex/index data and transforms but force the unculled triangle
// path. No production switch, shader or geometry is altered for this reference.
func unculledSnapshot(s *Snapshot) *Snapshot {
	out := &Snapshot{instances: append([]instance(nil), s.instances...)}
	meshes := map[*Mesh]*Mesh{}
	for i := range out.instances {
		m := out.instances[i].mesh
		if meshes[m] == nil {
			copy := *m
			copy.min.X = math.NaN()
			meshes[m] = &copy
		}
		out.instances[i].mesh = meshes[m]
	}
	return out
}

func checkCullingParity(t *testing.T, s *Snapshot, c Camera) {
	t.Helper()
	var accelerated, reference Renderer
	a, err := accelerated.Render(s, c, 80, 60, color.RGBA{9, 18, 27, 255})
	if err != nil {
		t.Fatal(err)
	}
	b, err := reference.Render(unculledSnapshot(s), c, 80, 60, color.RGBA{9, 18, 27, 255})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) || !reflect.DeepEqual(a.ids, b.ids) || !reflect.DeepEqual(a.depth, b.depth) {
		t.Fatal("culling changed exact pixels, depth or pick IDs", a.Stats, b.Stats)
	}
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			if a.Pick(x, y) != b.Pick(x, y) {
				t.Fatal("picking changed", x, y)
			}
		}
	}
}

func TestFrustumRendererPixelDepthPickParityAndCache(t *testing.T) {
	s := NewScene()
	mesh := Box()
	for _, n := range []Node{{ID: "culled-first", Mesh: mesh, Transform: Translation(Vec3{X: -1e4})}, {ID: "visible", Mesh: mesh}, {ID: "culled-last", Mesh: mesh, Transform: Translation(Vec3{Y: 1e4})}} {
		if err := s.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	snap := snapshot(t, s)
	checkCullingParity(t, snap, testCamera())
	var r Renderer
	f, err := r.Render(snap, testCamera(), 80, 60, color.RGBA{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Stats.CulledInstances != 2 || f.Stats.CulledTriangles != 24 || f.Pick(40, 30) != "visible" {
		t.Fatal("culling or stable IDs", f.Stats, f.Pick(40, 30))
	}
	seq, stats := f.Sequence, f.Stats
	f, err = r.Render(snap, testCamera(), 80, 60, color.RGBA{})
	if err != nil {
		t.Fatal(err)
	}
	if !f.Stats.Cached || f.Sequence != seq || f.Stats.CulledInstances != stats.CulledInstances {
		t.Fatal("cached culling result changed")
	}
	if err = s.SetTransform("culled-first", Translation(Vec3{Z: 1})); err != nil {
		t.Fatal(err)
	}
	f, err = r.Render(snapshot(t, s), testCamera(), 80, 60, color.RGBA{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Stats.Cached || f.Stats.CulledInstances != 1 || f.Pick(40, 30) != "culled-first" {
		t.Fatal("moving a culled mesh did not restore it", f.Stats)
	}
	checkCullingParity(t, snapshot(t, s), testCamera())
	random := rand.New(rand.NewSource(90817))
	for run := 0; run < 180; run++ {
		scene := NewScene()
		c := testCamera()
		c.Eye = Vec3{X: random.Float64()*4 - 2, Y: random.Float64()*3 - 1.5, Z: random.Float64()*3 + 2}
		c.Near = .5
		c.Far = 30
		for i := 0; i < 18; i++ {
			pose := Translation(Vec3{X: random.Float64()*60 - 30, Y: random.Float64()*40 - 20, Z: random.Float64()*60 - 30}).Mul(RotationXYZ(Vec3{X: random.Float64() * 6, Y: random.Float64() * 6})).Mul(Scaling(Vec3{X: random.Float64()*5 - 2.5, Y: random.Float64() * 4, Z: random.Float64() * 4}))
			pose[4] += .3 // include an affine shear, not just TRS
			if err = scene.Add(Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: pose, Color: color.RGBA{byte(i * 13), byte(i * 7), byte(i * 19), 255}}); err != nil {
				t.Fatal(err)
			}
		}
		checkCullingParity(t, snapshot(t, scene), c)
	}
}

func TestFrustumDoesNotHideRendererErrors(t *testing.T) {
	mesh, err := NewMesh([]Vec3{{0, 2, 0}, {1, 3, 0}, {0, 3, 1}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	s := NewScene()
	pose := Scaling(Vec3{X: 1, Y: math.MaxFloat64 / 2, Z: 1})
	pose[12] = 1e20
	if err = s.Add(Node{ID: "invalid-world", Mesh: mesh, Transform: pose}); err != nil {
		t.Fatal(err)
	}
	snap := snapshot(t, s)
	for _, candidate := range []*Snapshot{snap, unculledSnapshot(snap)} {
		var r Renderer
		if _, err = r.Render(candidate, testCamera(), 32, 32, color.RGBA{}); !errors.Is(err, ErrGeometry) {
			t.Fatal("geometry error hidden", err)
		}
	}
}

func float32Clip(v Vec3, world, vp Mat4) vec4 {
	input := [4]float32{float32(v.X), float32(v.Y), float32(v.Z), 1}
	var w, p [4]float32
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			w[row] = float32(w[row] + float32(float32(world[col*4+row])*input[col]))
		}
	}
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			p[row] = float32(p[row] + float32(float32(vp[col*4+row])*w[col]))
		}
	}
	return vec4{float64(p[0]), float64(p[1]), float64(p[2]), float64(p[3])}
}

func FuzzFrustumKeepsFloat32ClipCoverage(f *testing.F) {
	f.Add([]byte("perspective-boundary-check-000000000000000000000000000000000000000000"))
	f.Add(make([]byte, 64))
	m := Box()
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 32 {
			return
		}
		world := Identity()
		vp := Mat4{}
		for j, index := range []int{0, 1, 2, 4, 5, 6, 8, 9, 10, 12, 13, 14} {
			world[index] = float64(int(data[j])-128) / 16
		}
		for j := range vp {
			vp[j] = float64(int(data[j+12])-128) / 16
		}
		frustum := NewFrustum(vp)
		if frustum.Intersects(m, world) {
			return
		}
		var common uint8 = 63
		for _, v := range m.vertices {
			p := float32Clip(v, world, vp)
			common &= outcode(p)
		}
		if common == 0 {
			t.Fatal("rejected box has no common outside float32 clip plane", world, vp)
		}
	})
}

func BenchmarkFrustumInstanceCulling(b *testing.B) {
	mesh, err := Cylinder(.5, 1, 64)
	if err != nil {
		b.Fatal(err)
	}
	for _, visible := range []bool{false, true} {
		s := NewScene()
		for i := 0; i < 1000; i++ {
			p := Vec3{X: 1000 + float64(i%40)*3, Y: float64(i/40) * 3}
			scale := Identity()
			if visible {
				p = Vec3{X: float64(i%40-20) * .08, Y: float64(i/40-12) * .08}
				scale = Scaling(Vec3{X: .03, Y: .03, Z: .03})
			} else if i == 500 {
				p = Vec3{}
			}
			if err = s.Add(Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: Translation(p).Mul(scale)}); err != nil {
				b.Fatal(err)
			}
		}
		snap, err := s.Snapshot()
		if err != nil {
			b.Fatal(err)
		}
		for _, enabled := range []bool{false, true} {
			candidate := snap
			if !enabled {
				candidate = unculledSnapshot(snap)
			}
			b.Run(fmt.Sprintf("visible=%v/culling=%v", visible, enabled), func(b *testing.B) {
				var r Renderer
				c := testCamera()
				c.Eye.X = -.001
				if _, err := r.Render(candidate, c, 160, 120, color.RGBA{}); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					c.Eye.X = float64(i%2) * .001
					if _, err := r.Render(candidate, c, 160, 120, color.RGBA{}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
