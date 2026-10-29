package scene3d

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func placementNear(a, b Vec3) bool { return a.Sub(b).Length() <= 2e-11*math.Max(1, b.Length()) }
func checkSurfaceFrame(t testing.TB, point, normal Vec3, options SurfacePlacementOptions) Mat4 {
	t.Helper()
	m, err := SurfacePlacementTransform(point, normal, options)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := projectionUnit(normal)
	if !placementNear(m.Point(options.Anchor), point.Add(n.Mul(options.Offset))) {
		t.Fatal("anchor drift", m.Point(options.Anchor), point)
	}
	x, y, z := Vec3{m[0], m[1], m[2]}, Vec3{m[4], m[5], m[6]}, Vec3{m[8], m[9], m[10]}
	if options.AlignNormal && !placementNear(y, n) {
		t.Fatal("normal", y, n)
	}
	if math.Abs(x.Dot(y)) > 2e-12 || math.Abs(y.Dot(z)) > 2e-12 || math.Abs(x.Dot(z)) > 2e-12 || math.Abs(x.Cross(y).Dot(z)-1) > 2e-12 {
		t.Fatal("not a proper rotation", m)
	}
	return m
}
func TestSurfacePlacementAlignmentAnchorsAndExtremes(t *testing.T) {
	for _, n := range []Vec3{{Y: 1}, {Y: -1}, {X: 1}, {Z: -1}, {1, 2, 3}, {1e300, 1e300, -1e300}, {1e-300, -1e-300, 1e-300}, {math.SmallestNonzeroFloat64, 0, 0}, {1e-300, -1, 1e-300}} {
		for _, heading := range []float64{0, math.Pi / 2, -3.2} {
			for _, align := range []bool{false, true} {
				checkSurfaceFrame(t, Vec3{2, 3, 4}, n, SurfacePlacementOptions{Anchor: Vec3{2, -1, .5}, Offset: .3, Heading: heading, AlignNormal: align})
			}
		}
	}
	m, err := SurfacePlacementTransform(Vec3{1, 2, 3}, Vec3{Y: 1}, SurfacePlacementOptions{})
	if err != nil || m != Translation(Vec3{1, 2, 3}) {
		t.Fatal(m, err)
	}
	m, err = SurfacePlacementTransform(Vec3{}, Vec3{Y: -1}, SurfacePlacementOptions{AlignNormal: true})
	if err != nil || m != Scaling(Vec3{1, -1, -1}) {
		t.Fatal("antipodal must be exact", m, err)
	}
	r := rand.New(rand.NewSource(20261029))
	for i := 0; i < 1000; i++ {
		checkSurfaceFrame(t, Vec3{r.NormFloat64(), r.NormFloat64(), r.NormFloat64()}, Vec3{r.NormFloat64(), r.NormFloat64(), r.NormFloat64()}, SurfacePlacementOptions{Anchor: Vec3{r.NormFloat64(), r.NormFloat64(), r.NormFloat64()}, Offset: r.NormFloat64(), Heading: r.NormFloat64(), AlignNormal: true})
	}
	if n := testing.AllocsPerRun(100, func() {
		_, _ = SurfacePlacementTransform(Vec3{1, 2, 3}, Vec3{1, 3, 2}, SurfacePlacementOptions{AlignNormal: true})
	}); n != 0 {
		t.Fatal("allocated", n)
	}
}
func TestSurfacePlacementInvalidAndBounds(t *testing.T) {
	for _, tc := range []struct {
		point, normal Vec3
		options       SurfacePlacementOptions
	}{
		{normal: Vec3{}}, {point: Vec3{X: math.NaN()}, normal: Vec3{Y: 1}}, {normal: Vec3{Y: math.Inf(1)}},
		{normal: Vec3{Y: 1}, options: SurfacePlacementOptions{Anchor: Vec3{Z: math.NaN()}}},
		{normal: Vec3{Y: 1}, options: SurfacePlacementOptions{Heading: math.Inf(1)}},
		{normal: Vec3{Y: 1}, options: SurfacePlacementOptions{Offset: math.NaN()}},
		{point: Vec3{Y: math.MaxFloat64}, normal: Vec3{Y: 1}, options: SurfacePlacementOptions{Offset: math.MaxFloat64}},
		{point: Vec3{Y: math.MaxFloat64}, normal: Vec3{Y: 1}, options: SurfacePlacementOptions{Anchor: Vec3{Y: -math.MaxFloat64}}},
	} {
		m, err := SurfacePlacementTransform(tc.point, tc.normal, tc.options)
		if !errors.Is(err, ErrGeometry) || m != (Mat4{}) {
			t.Fatal(m, err)
		}
	}
	s := NewScene()
	mustEdit(t, s.Add(Node{ID: "root", Transform: Translation(Vec3{2, 3, 4})}))
	mustEdit(t, s.Add(Node{ID: "box", Parent: "root", Mesh: Box(), Transform: Scaling(Vec3{2, 3, 4})}))
	mustEdit(t, s.Add(Node{ID: "hidden", Mesh: Box(), Hidden: true, Transform: Translation(Vec3{Y: -100})}))
	d, err := CaptureDocument(s, nil)
	mustEdit(t, err)
	tmpl, err := NewTemplate(t.Context(), d)
	mustEdit(t, err)
	lo, hi, err := tmpl.Bounds()
	mustEdit(t, err)
	if lo != (Vec3{0, 0, 0}) || hi != (Vec3{4, 6, 8}) {
		t.Fatal(lo, hi)
	}
	anchor, err := tmpl.BaseAnchor()
	mustEdit(t, err)
	if anchor != (Vec3{2, 0, 4}) {
		t.Fatal(anchor)
	}
	d.Nodes[1].Transform[13] = -50
	again, _ := tmpl.BaseAnchor()
	if again != anchor {
		t.Fatal("mutable template bounds")
	}
	empty, err := NewTemplate(t.Context(), &Document{Version: DocumentVersion})
	mustEdit(t, err)
	for _, invalid := range []*Template{nil, {}, empty} {
		if _, err := invalid.BaseAnchor(); !errors.Is(err, ErrGeometry) {
			t.Fatal(err)
		}
	}
}
func TestSurfacePlacementTemplateHistoryAndLiveBinding(t *testing.T) {
	ctx := context.Background()
	e, original, tmpl := templateEditFixture(t)
	anchor, err := tmpl.BaseAnchor()
	mustEdit(t, err)
	matrix := checkSurfaceFrame(t, Vec3{3, 4, 5}, Vec3{X: 1}, SurfacePlacementOptions{Anchor: anchor, AlignNormal: true})
	instance, err := e.InstantiateTemplate(ctx, tmpl, InstanceOptions{ID: "mounted", TagPrefix: "sensor.", Transform: matrix})
	mustEdit(t, err)
	root, _ := e.scene.Node(instance.RootID())
	if root.Transform != matrix {
		t.Fatal("lost transform")
	}
	if count, _ := e.HistoryUsage(); count != 1 {
		t.Fatal("not atomic", count)
	}
	after, err := e.Document()
	mustEdit(t, err)
	mustEdit(t, e.Undo(ctx))
	back, _ := e.Document()
	if !reflect.DeepEqual(back, original) {
		t.Fatal("undo not exact")
	}
	mustEdit(t, e.Redo(ctx))
	redo, _ := e.Document()
	if !reflect.DeepEqual(redo, after) {
		t.Fatal("redo not exact")
	}
	live, err := redo.Build(ctx)
	mustEdit(t, err)
	program, err := NewBindingSet(live, redo.Bindings)
	mustEdit(t, err)
	_, err = program.Apply("sensor.height", 1000.0)
	mustEdit(t, err)
	unchanged, _ := live.Node("mounted")
	moved, _ := live.Node("mounted/parent")
	authored, _ := e.scene.Node("mounted/parent")
	if unchanged.Transform != matrix || moved.Transform[13] != 1 || authored.Transform[13] != 0 {
		t.Fatal("binding lost placement or mutated authored data")
	}
}
func FuzzSurfacePlacementFrames(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, 4.0, -2.0)
	f.Add(0.0, -1.0, 0.0, 0.0, 1.0)
	f.Fuzz(func(t *testing.T, x, y, z, heading, offset float64) {
		if !finite(x) || !finite(y) || !finite(z) || !finite(heading) || !finite(offset) {
			t.Skip()
		}
		if _, ok := projectionUnit(Vec3{x, y, z}); !ok {
			t.Skip()
		}
		checkSurfaceFrame(t, Vec3{1, 2, 3}, Vec3{x, y, z}, SurfacePlacementOptions{Anchor: Vec3{.5, -.25, 2}, Heading: math.Remainder(heading, 2*math.Pi), Offset: math.Remainder(offset, 100), AlignNormal: true})
	})
}
