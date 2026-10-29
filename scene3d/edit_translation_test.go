package scene3d

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestTranslationEditPreviewCommitAndExactHistory(t *testing.T) {
	ctx := context.Background()
	parent := Translation(Vec3{X: 11, Y: -4, Z: 2}).Mul(RotationXYZ(Vec3{X: .2, Y: .5, Z: -.3})).Mul(Scaling(Vec3{X: 2, Y: -3, Z: .7}))
	parent[4] += .3 // shear, not just TRS
	local := Translation(Vec3{X: 1, Y: 2, Z: 3}).Mul(RotationXYZ(Vec3{Y: .7})).Mul(Scaling(Vec3{X: -1, Y: .4, Z: 3}))
	s := NewScene()
	mustEdit(t, s.Add(Node{ID: "parent", Transform: parent}))
	mustEdit(t, s.Add(Node{ID: "child", Parent: "parent", Mesh: Box(), Transform: local}))
	d, err := CaptureDocument(s, []Binding{{Node: "child", Tag: "level", Property: PositionY, Gain: 2}})
	mustEdit(t, err)
	e, err := NewEditSession(ctx, d)
	mustEdit(t, err)
	defer e.Close()
	original, _ := e.Document()
	originalSnap, _ := e.scene.Snapshot()
	oldWorld, _ := e.WorldTransform("child")
	g, err := e.BeginTranslation(ctx, "child")
	mustEdit(t, err)
	defer g.Cancel()
	preview, _ := e.Scene()
	for i := 0; i < 100; i++ {
		m, err := g.Transform(Vec3{X: float64(i) / 10})
		mustEdit(t, err)
		mustEdit(t, preview.SetTransform("child", m))
	}
	unchanged, _ := e.Document()
	if !reflect.DeepEqual(original, unchanged) || e.Modified() || e.Revision() != 0 || e.CanUndo() || e.scene.cached != originalSnap {
		t.Fatal("preview changed authored state/history/cache")
	}
	if allocs := testing.AllocsPerRun(100, func() { _, _ = g.Transform(Vec3{X: .1}) }); allocs != 0 {
		t.Fatal("preview allocates", allocs)
	}
	delta := Vec3{X: 2.5, Y: -1.75, Z: .6}
	m, err := g.Transform(delta)
	mustEdit(t, err)
	world := parent.Mul(m)
	for i := range world {
		want := oldWorld[i]
		if i >= 12 && i < 15 {
			want += []float64{delta.X, delta.Y, delta.Z}[i-12]
		}
		if math.Abs(world[i]-want) > 1e-10 {
			t.Fatal("world displacement changed basis or used local axes", i, world[i], want)
		}
	}
	mustEdit(t, g.Commit(ctx, delta))
	if count, _ := e.HistoryUsage(); count != 1 || e.Revision() != 1 {
		t.Fatal("gesture created more than one history entry", count, e.Revision())
	}
	after, _ := e.Document()
	if !reflect.DeepEqual(after.Bindings, original.Bindings) || !reflect.DeepEqual(after.Meshes, original.Meshes) {
		t.Fatal("gesture changed resources or inert binding declarations")
	}
	if _, err := g.Transform(Vec3{}); !errors.Is(err, ErrEditClosed) {
		t.Fatal("committed gesture not retired", err)
	}
	mustEdit(t, e.Undo(ctx))
	restored, _ := e.Document()
	if !reflect.DeepEqual(original, restored) || e.Modified() {
		t.Fatal("undo is not exact")
	}
	mustEdit(t, e.Redo(ctx))
	redone, _ := e.Document()
	if !reflect.DeepEqual(after, redone) {
		t.Fatal("redo is not exact")
	}
}
func TestTranslationEditNoopCancelStaleAndInvalid(t *testing.T) {
	ctx := context.Background()
	e, original := editFixture(t)
	id := original.Nodes[1].ID
	g, err := e.BeginTranslation(ctx, id)
	mustEdit(t, err)
	g.Cancel()
	g.Cancel()
	if err := g.Commit(ctx, Vec3{X: 1}); !errors.Is(err, ErrEditClosed) {
		t.Fatal("canceled gesture accepted", err)
	}
	n := e.Nodes()[1]
	n.Name = "rename"
	mustEdit(t, e.UpdateNode(ctx, n))
	mustEdit(t, e.Undo(ctx))
	rev := e.Revision()
	g, err = e.BeginTranslation(ctx, id)
	mustEdit(t, err)
	mustEdit(t, g.Commit(ctx, Vec3{}))
	d, _ := e.Document()
	if e.Revision() != rev || !e.CanRedo() || !reflect.DeepEqual(d, original) {
		t.Fatal("zero gesture changed encoding/history/redo")
	}
	g, err = e.BeginTranslation(ctx, id)
	mustEdit(t, err)
	defer g.Cancel()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, badContext := range []context.Context{nil, canceled} {
		assertEditUnchanged(t, e, func() error { return g.Commit(badContext, Vec3{X: 1}) })
		if _, err := e.BeginTranslation(badContext, id); err == nil {
			t.Fatal("bad begin context accepted")
		}
	}
	for _, delta := range []Vec3{{X: math.NaN()}, {Y: math.Inf(1)}, {Z: math.Inf(-1)}} {
		assertEditUnchanged(t, e, func() error { return g.Commit(ctx, delta) })
	}
	mustEdit(t, e.Redo(ctx))
	mustEdit(t, e.Undo(ctx)) // identical contents, different revision
	err = assertEditUnchanged(t, e, func() error { return g.Commit(ctx, Vec3{X: 1}) })
	if !errors.Is(err, ErrEditConflict) {
		t.Fatal("stale gesture not rejected", err)
	}
	if _, err := g.Transform(Vec3{}); !errors.Is(err, ErrEditConflict) {
		t.Fatal("stale preview not rejected", err)
	}
	if _, err := e.BeginTranslation(ctx, "missing"); !errors.Is(err, ErrNode) {
		t.Fatal(err)
	}
	e.Close()
	if _, err := e.BeginTranslation(ctx, id); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	if _, err := g.Transform(Vec3{}); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	var nilGesture *TranslationEdit
	nilGesture.Cancel()
	if err := nilGesture.Commit(ctx, Vec3{}); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
}
func TestTranslationEditHiddenGroupsSingularAndHugeParent(t *testing.T) {
	ctx := context.Background()
	for _, scale := range []float64{0, 1e-200, -2} {
		s := NewScene()
		parent := Translation(Vec3{X: 1e200}).Mul(Scaling(Vec3{X: scale, Y: scale, Z: scale}))
		mustEdit(t, s.Add(Node{ID: "parent", Hidden: true, Transform: parent}))
		mustEdit(t, s.Add(Node{ID: "group", Parent: "parent"}))
		d, err := CaptureDocument(s, nil)
		mustEdit(t, err)
		e, err := NewEditSession(ctx, d)
		mustEdit(t, err)
		defer e.Close()
		g, err := e.BeginTranslation(ctx, "group")
		if scale == 0 {
			if err == nil || g != nil || e.Modified() {
				t.Fatal("singular parent accepted")
			}
			continue
		}
		mustEdit(t, err)
		m, err := g.Transform(Vec3{Y: 1})
		mustEdit(t, err)
		if m[13] != 1/scale || m[12] != 0 {
			t.Fatal("parent translation contaminated delta", m)
		}
		mustEdit(t, g.Commit(ctx, Vec3{Y: 1}))
		mustEdit(t, e.Undo(ctx))
		after, _ := e.Document()
		if !reflect.DeepEqual(d, after) {
			t.Fatal("hidden group transform encoding changed after undo")
		}
	}
}
func BenchmarkTranslationEditPreview10000(b *testing.B) {
	s := NewScene()
	mesh := Box()
	for i := 0; i < 10000; i++ {
		mustEdit(b, s.Add(Node{ID: fmt.Sprint(i), Mesh: mesh}))
	}
	d, err := CaptureDocument(s, nil)
	mustEdit(b, err)
	e, err := NewEditSession(context.Background(), d)
	mustEdit(b, err)
	defer e.Close()
	g, err := e.BeginTranslation(context.Background(), "5000")
	mustEdit(b, err)
	defer g.Cancel()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := g.Transform(Vec3{X: float64(i % 100)})
		if err != nil {
			b.Fatal(err)
		}
	}
}
