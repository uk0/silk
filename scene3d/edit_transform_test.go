package scene3d

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestTransformEditWorldBasisOriginHistoryAndBindings(t *testing.T) {
	ctx := context.Background()
	parent := Translation(Vec3{11, -4, 2}).Mul(RotationXYZ(Vec3{.2, .5, -.3})).Mul(Scaling(Vec3{2, -3, .7}))
	parent[4] += .3
	local := Translation(Vec3{1, 2, 3}).Mul(RotationXYZ(Vec3{0, .7, 0})).Mul(Scaling(Vec3{-1, .4, 3}))
	rotation, err := RotationAxis(Vec3{1, 2, -3}, .7)
	mustEdit(t, err)
	for name, delta := range map[string]Mat4{"rotation": rotation, "scale": Scaling(Vec3{2, .7, 1.4})} {
		t.Run(name, func(t *testing.T) {
			s := NewScene()
			mustEdit(t, s.Add(Node{ID: "parent", Transform: parent}))
			mustEdit(t, s.Add(Node{ID: "child", Parent: "parent", Mesh: Box(), Transform: local}))
			mustEdit(t, s.Add(Node{ID: "leaf", Parent: "child", Mesh: Box(), Transform: Translation(Vec3{1, 0, 0})}))
			d, err := CaptureDocument(s, []Binding{{Node: "child", Tag: "level", Property: ScaleY, Gain: 1}})
			mustEdit(t, err)
			e, err := NewEditSession(ctx, d)
			mustEdit(t, err)
			defer e.Close()
			before, _ := e.Document()
			snap, _ := e.scene.Snapshot()
			old, _ := e.WorldTransform("child")
			g, err := e.BeginTransform(ctx, "child")
			mustEdit(t, err)
			defer g.Cancel()
			m, err := g.Transform(delta)
			mustEdit(t, err)
			world := parent.Mul(m)
			basis := old
			basis[12], basis[13], basis[14] = 0, 0, 0
			want := delta.Mul(basis)
			for i := range world {
				if i >= 12 && i < 15 {
					want[i] = old[i]
				}
				if math.Abs(world[i]-want[i]) > 1e-10 {
					t.Fatalf("world[%d]=%g want %g", i, world[i], want[i])
				}
			}
			if m[12] != local[12] || m[13] != local[13] || m[14] != local[14] {
				t.Fatal("local origin drift")
			}
			preview, _ := e.Scene()
			mustEdit(t, preview.SetTransform("child", m))
			afterPreview, _ := e.Document()
			if !reflect.DeepEqual(before, afterPreview) || e.Revision() != 0 || e.scene.cached != snap {
				t.Fatal("preview mutated authoring")
			}
			if n := testing.AllocsPerRun(100, func() { _, _ = g.Transform(delta) }); n != 0 {
				t.Fatal("preview allocations", n)
			}
			mustEdit(t, g.Commit(ctx, delta))
			after, _ := e.Document()
			if n, _ := e.HistoryUsage(); n != 1 || e.Revision() != 1 {
				t.Fatal("not one transaction")
			}
			if !reflect.DeepEqual(before.Meshes, after.Meshes) || !reflect.DeepEqual(before.Bindings, after.Bindings) || !reflect.DeepEqual(before.Nodes[2], after.Nodes[2]) {
				t.Fatal("resources/bindings/child local changed")
			}
			mustEdit(t, e.Undo(ctx))
			undo, _ := e.Document()
			if !reflect.DeepEqual(before, undo) {
				t.Fatal("undo not exact")
			}
			mustEdit(t, e.Redo(ctx))
			redo, _ := e.Document()
			if !reflect.DeepEqual(after, redo) {
				t.Fatal("redo not exact")
			}
			if _, err := g.Transform(delta); !errors.Is(err, ErrEditClosed) {
				t.Fatal(err)
			}
		})
	}
}

func TestTransformEditFailuresNoopAndLargeParent(t *testing.T) {
	ctx := context.Background()
	e, original := editFixture(t)
	id := original.Nodes[1].ID
	node := e.Nodes()[1]
	node.Name = "changed"
	mustEdit(t, e.UpdateNode(ctx, node))
	mustEdit(t, e.Undo(ctx))
	rev := e.Revision()
	g, err := e.BeginTransform(ctx, id)
	mustEdit(t, err)
	mustEdit(t, g.Commit(ctx, Identity()))
	d, _ := e.Document()
	if !reflect.DeepEqual(d, original) || e.Revision() != rev || !e.CanRedo() {
		t.Fatal("identity changed history/encoding")
	}
	g, err = e.BeginTransform(ctx, id)
	mustEdit(t, err)
	defer g.Cancel()
	bad := Identity()
	bad[0] = math.NaN()
	projective := Identity()
	projective[3] = .1
	for _, delta := range []Mat4{bad, projective, Translation(Vec3{1, 0, 0})} {
		assertEditUnchanged(t, e, func() error { return g.Commit(ctx, delta) })
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, c := range []context.Context{nil, canceled} {
		assertEditUnchanged(t, e, func() error { return g.Commit(c, Scaling(Vec3{2, 1, 1})) })
		if _, err := e.BeginTransform(c, id); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
	mustEdit(t, e.Redo(ctx))
	mustEdit(t, e.Undo(ctx))
	err = assertEditUnchanged(t, e, func() error { return g.Commit(ctx, Identity()) })
	if !errors.Is(err, ErrEditConflict) {
		t.Fatal(err)
	}
	g.Cancel()
	g.Cancel()
	if _, err := g.Transform(Identity()); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	var nilEdit *TransformEdit
	nilEdit.Cancel()
	if err := nilEdit.Commit(ctx, Identity()); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	if _, err := e.BeginTransform(ctx, "missing"); !errors.Is(err, ErrNode) {
		t.Fatal(err)
	}
	e.Close()
	if _, err := e.BeginTransform(ctx, id); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	for _, scale := range []float64{0, 1e-200, -2} {
		s := NewScene()
		mustEdit(t, s.Add(Node{ID: "parent", Hidden: true, Transform: Translation(Vec3{1e200, 0, 0}).Mul(Scaling(Vec3{scale, scale, scale}))}))
		mustEdit(t, s.Add(Node{ID: "group", Parent: "parent"}))
		d, err := CaptureDocument(s, nil)
		mustEdit(t, err)
		e, err := NewEditSession(ctx, d)
		mustEdit(t, err)
		g, err := e.BeginTransform(ctx, "group")
		if scale == 0 {
			if err == nil || g != nil {
				t.Fatal("singular parent accepted")
			}
			e.Close()
			continue
		}
		mustEdit(t, err)
		rotation, _ := RotationAxis(Vec3{0, 1, 0}, .4)
		m, err := g.Transform(rotation)
		mustEdit(t, err)
		for i := range m {
			if math.Abs(m[i]-rotation[i]) > 1e-12 {
				t.Fatal("huge translation contaminated basis", m)
			}
		}
		mustEdit(t, g.Commit(ctx, rotation))
		mustEdit(t, e.Undo(ctx))
		undo, _ := e.Document()
		if !reflect.DeepEqual(d, undo) {
			t.Fatal("hidden group encoding changed")
		}
		e.Close()
	}
}

func BenchmarkTransformEditPreview10000(b *testing.B) {
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
	g, err := e.BeginTransform(context.Background(), "5000")
	mustEdit(b, err)
	defer g.Cancel()
	op, _ := RotationAxis(Vec3{1, 2, 3}, .4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := g.Transform(op); err != nil {
			b.Fatal(err)
		}
	}
}
