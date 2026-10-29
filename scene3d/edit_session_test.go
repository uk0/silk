package scene3d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func editFixture(t testing.TB) (*EditSession, *Document) {
	t.Helper()
	d, err := CaptureDocument(documentScene(t), []Binding{{Node: "a", Tag: "level", Property: PositionY, Gain: 1}, {Node: "b", Tag: "visible", Property: Visibility}})
	if err != nil {
		t.Fatal(err)
	}
	d.Warnings = []string{"inert metadata"}
	e, err := NewEditSession(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e, d
}
func mustEdit(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func assertEditUnchanged(t testing.TB, e *EditSession, run func() error) error {
	t.Helper()
	d, _ := e.Document()
	snap := e.scene.cached
	rev := e.Revision()
	count, size := e.HistoryUsage()
	meshMap := reflect.ValueOf(e.state.meshes).Pointer()
	err := run()
	if err == nil {
		t.Fatal("invalid edit accepted")
	}
	after, _ := e.Document()
	n, b := e.HistoryUsage()
	if !reflect.DeepEqual(d, after) || e.scene.cached != snap || rev != e.Revision() || count != n || size != b || meshMap != reflect.ValueOf(e.state.meshes).Pointer() {
		t.Fatal("failed edit changed published state/history/resources")
	}
	return err
}
func TestEditSessionOwnershipCachesHistoryAndNoops(t *testing.T) {
	ctx := context.Background()
	e, d := editFixture(t)
	original, _ := e.Document()
	mesh, _ := e.Mesh(d.Meshes[0].ID)
	snap := e.scene.cached
	d.Meshes[0].Vertices[0].X = 999
	d.Nodes[1].Transform[12] = 999
	d.Warnings[0] = "external"
	n := e.Nodes()[1]
	n.Name = "renamed"
	mustEdit(t, e.UpdateNode(ctx, n))
	if !e.Modified() || !e.CanUndo() || e.CanRedo() || e.Revision() != 1 || e.scene.cached != snap {
		t.Fatal("metadata edit invalidated geometry/render cache")
	}
	m, _ := e.Mesh(original.Meshes[0].ID)
	if m != mesh {
		t.Fatal("recompiled immutable geometry")
	}
	rev := e.Revision()
	mustEdit(t, e.UpdateNode(ctx, n))
	mustEdit(t, e.Reparent(ctx, n.ID, n.Parent, true))
	mustEdit(t, e.SetBinding(ctx, e.Bindings()[0]))
	removed, err := e.RemoveBinding(ctx, "missing", PositionX)
	mustEdit(t, err)
	if removed || rev != e.Revision() {
		t.Fatal("no-op edit produced history")
	}
	exported, _ := e.Document()
	exported.Nodes[1].Transform[12] = 700
	exported.Meshes[0].Indices[0] = 700
	exported.Warnings[0] = "changed"
	nodes := e.Nodes()
	nodes[1].Transform[12] = 700
	bindings := e.Bindings()
	bindings[0].Tag = "changed"
	ids := e.MeshIDs()
	ids[0] = "changed"
	sc, err := e.Scene()
	mustEdit(t, err)
	mustEdit(t, sc.SetTransform("a", Translation(Vec3{X: 88})))
	if e.scene.cached != snap || e.scene.nodes["a"].Transform[12] != 0 || e.Bindings()[0].Tag != "level" {
		t.Fatal("export exposed mutable owned state")
	}
	mustEdit(t, e.Undo(ctx))
	got, _ := e.Document()
	if e.Modified() || !reflect.DeepEqual(got, original) || e.CanUndo() || !e.CanRedo() {
		t.Fatal("undo did not restore exact source")
	}
	mustEdit(t, e.Redo(ctx))
	if !e.Modified() || e.Nodes()[1].Name != "renamed" {
		t.Fatal("redo")
	}
	mustEdit(t, e.Undo(ctx))
	mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "branch"}))
	if e.CanRedo() {
		t.Fatal("new branch retained redo")
	}
	e.Close()
	e.Close()
	if _, err = e.Document(); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	if e.Nodes() != nil || e.Bindings() != nil || e.MeshIDs() != nil || e.CanUndo() || e.CanRedo() || e.Modified() {
		t.Fatal("close retained state")
	}
	if err = e.AddNode(ctx, DocumentNode{ID: "closed"}); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	var nilSession *EditSession
	if err = nilSession.Undo(ctx); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
	nilSession.Close()
}
func TestEditSessionAtomicFailuresAndCancellation(t *testing.T) {
	e, _ := editFixture(t)
	ctx := context.Background()
	bad := e.Nodes()[1]
	tests := map[string]func() error{
		"duplicate":      func() error { return e.AddNode(ctx, DocumentNode{ID: "a"}) },
		"parent":         func() error { return e.AddNode(ctx, DocumentNode{ID: "x", Parent: "missing"}) },
		"missing-mesh":   func() error { return e.AddNode(ctx, DocumentNode{ID: "x", Mesh: "missing"}) },
		"long-transform": func() error { return e.AddNode(ctx, DocumentNode{ID: "x", Transform: make([]float64, 100000)}) },
		"nonfinite": func() error {
			n := bad
			n.Transform = append([]float64(nil), bad.Transform...)
			n.Transform[5] = math.Inf(1)
			return e.UpdateNode(ctx, n)
		},
		"alpha":          func() error { n := bad; n.Color = color.RGBA{A: 2}; return e.UpdateNode(ctx, n) },
		"binding":        func() error { return e.SetBinding(ctx, Binding{Node: "parent", Tag: "alarm", Property: AlarmColor}) },
		"mixed-tag":      func() error { return e.SetBinding(ctx, Binding{Node: "a", Tag: "level", Property: Visibility}) },
		"cycle":          func() error { return e.Reparent(ctx, "parent", "a", false) },
		"remove-missing": func() error { return e.RemoveSubtree(ctx, "missing") },
		"undo-empty":     func() error { return e.Undo(ctx) }, "redo-empty": func() error { return e.Redo(ctx) },
		"nil-context": func() error { return e.AddNode(nil, DocumentNode{ID: "x"}) },
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) { assertEditUnchanged(t, e, run) })
	}
	for _, at := range []int{0, 1, 3, 7, 12} {
		t.Run(fmt.Sprintf("cancel-%d", at), func(t *testing.T) {
			ctx := &editCancelContext{Context: context.Background(), remaining: at}
			err := assertEditUnchanged(t, e, func() error { return e.AddMeshNode(ctx, DocumentNode{ID: "fresh"}, "fresh-mesh", Box()) })
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
	badDoc, _ := e.Document()
	badDoc.Nodes[0].Transform = make([]float64, 100000)
	if _, err := NewEditSession(ctx, badDoc); !errors.Is(err, ErrNode) {
		t.Fatal(err)
	}
	if _, err := NewEditSession(ctx, nil); !errors.Is(err, ErrGeometry) {
		t.Fatal(err)
	}
	mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "history"}))
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	assertEditUnchanged(t, e, func() error { return e.Undo(canceled) })
}

type editCancelContext struct {
	context.Context
	remaining int
}

func (c *editCancelContext) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}
func TestEditSessionDuplicateDeleteRoundtripAndBindings(t *testing.T) {
	ctx := context.Background()
	e, d := editFixture(t)
	ids, err := e.DuplicateSubtree(ctx, "parent", DuplicateOptions{RootID: "copy", RootName: "复制组", Offset: Vec3{X: 4}})
	mustEdit(t, err)
	if len(ids) != 3 || ids["a"] != "copy/node-1" || ids["b"] != "copy/node-2" || len(e.Bindings()) != 2 || len(e.MeshIDs()) != 1 {
		t.Fatal(ids)
	}
	copied := e.scene.nodes[ids["a"]]
	if copied.Parent != "copy" || copied.Mesh != e.scene.nodes["a"].Mesh {
		t.Fatal("copy lost mesh sharing or hierarchy")
	}
	a, _ := e.WorldTransform("a")
	b, _ := e.WorldTransform(ids["a"])
	if math.Abs(b[12]-a[12]-4) > 1e-12 {
		t.Fatal("offset")
	}
	rev := e.Revision()
	mustEdit(t, e.Undo(ctx))
	if e.Modified() {
		t.Fatal("duplicate undo")
	}
	mustEdit(t, e.Redo(ctx))
	if e.Revision() != rev+2 {
		t.Fatal("monotonic revision")
	}
	mustEdit(t, e.Undo(ctx))
	ids, err = e.DuplicateSubtree(ctx, "parent", DuplicateOptions{RootID: "copy", CopyBindings: true, Tags: map[string]string{"level": "copy.level", "visible": "copy.visible"}})
	mustEdit(t, err)
	if len(e.Bindings()) != 4 || e.Bindings()[2].Node != ids["a"] || e.Bindings()[2].Tag != "copy.level" {
		t.Fatal(e.Bindings())
	}
	copiedDoc, _ := e.Document()
	mustEdit(t, e.RemoveSubtree(ctx, "parent"))
	if len(e.Nodes()) != 4 || len(e.Bindings()) != 2 || e.Bindings()[0].Tag != "copy.level" || len(e.MeshIDs()) != 1 {
		t.Fatal("delete did not clean subtree bindings")
	}
	mustEdit(t, e.Undo(ctx))
	restored, _ := e.Document()
	if !reflect.DeepEqual(restored, copiedDoc) {
		t.Fatal("delete undo order/metadata")
	}
	mustEdit(t, e.Undo(ctx))
	original, _ := e.Document()
	if !reflect.DeepEqual(original, d) || e.Modified() {
		t.Fatal("undo origin")
	}
	var out bytes.Buffer
	mustEdit(t, SaveDocument(ctx, &out, original))
	loaded, err := LoadDocument(ctx, &out)
	mustEdit(t, err)
	if !reflect.DeepEqual(loaded, d) {
		t.Fatal("save/reopen")
	}
	for name, o := range map[string]DuplicateOptions{
		"collision": {RootID: "a"}, "child-collision": {RootID: "collision"}, "unused-remap": {RootID: "copy", CopyBindings: true, Tags: map[string]string{"missing": "tag"}},
		"disabled-remap": {RootID: "copy", Tags: map[string]string{"level": "tag"}}, "bad-tag": {RootID: "copy", CopyBindings: true, Tags: map[string]string{"level": ""}},
		"tag-type": {RootID: "copy", CopyBindings: true, Tags: map[string]string{"level": "visible"}}, "long-child": {RootID: strings.Repeat("x", 256)}, "infinite-offset": {RootID: "copy", Offset: Vec3{X: math.Inf(1)}},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "child-collision" {
				mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "collision/node-1"}))
			}
			assertEditUnchanged(t, e, func() error { _, err := e.DuplicateSubtree(ctx, "parent", o); return err })
		})
	}
}
func TestEditSessionMeshResourceTransactionAndRebranch(t *testing.T) {
	ctx := context.Background()
	e, _ := editFixture(t)
	mesh := Box()
	mustEdit(t, e.AddMeshNode(ctx, DocumentNode{ID: "new-node"}, "new-mesh", mesh))
	mustEdit(t, e.AddMeshNode(ctx, DocumentNode{ID: "second"}, "new-mesh", mesh))
	if e.scene.nodes["new-node"].Mesh != mesh || e.scene.nodes["second"].Mesh != mesh || len(e.MeshIDs()) != 2 {
		t.Fatal("resource not shared")
	}
	assertEditUnchanged(t, e, func() error { return e.AddMeshNode(ctx, DocumentNode{ID: "fail"}, "new-mesh", Box()) })
	mustEdit(t, e.Undo(ctx))
	mustEdit(t, e.Undo(ctx))
	if _, ok := e.Mesh("new-mesh"); ok {
		t.Fatal("undo retained added resource")
	}
	different, err := Cylinder(1, 2, 8)
	mustEdit(t, err)
	mustEdit(t, e.AddMeshNode(ctx, DocumentNode{ID: "different"}, "new-mesh", different))
	m, _ := e.Mesh("new-mesh")
	if m != different || e.CanRedo() {
		t.Fatal("resource identity on branch")
	}
}
func TestEditSessionHistoryBoundsAndDeepUnorderedHierarchy(t *testing.T) {
	ctx := context.Background()
	e, _ := editFixture(t)
	for i := 0; i < MaxEditHistory+10; i++ {
		n := e.Nodes()[1]
		n.Name = fmt.Sprint(i)
		mustEdit(t, e.UpdateNode(ctx, n))
	}
	count, b := e.HistoryUsage()
	if count != MaxEditHistory || b > MaxEditHistoryBytes {
		t.Fatal(count, b)
	}
	for e.CanUndo() {
		mustEdit(t, e.Undo(ctx))
	}
	if !e.Modified() {
		t.Fatal("discarded oldest history pretended to be source")
	}
	for e.CanRedo() {
		mustEdit(t, e.Redo(ctx))
	}
	// Also exercise the byte cap independently of count; tiny documents normally
	// hit the state cap first. Costs are private accounting, not caller supplied.
	e.undo = []editState{{cost: MaxEditHistoryBytes}, {cost: 1}}
	e.redo = nil
	e.trimHistory()
	if n, b := e.HistoryUsage(); n != 1 || b != 1 {
		t.Fatal(n, b)
	}
	d := &Document{Version: 1}
	for i := MaxNodes - 1; i >= 0; i-- {
		n := DocumentNode{ID: fmt.Sprint(i), Hidden: true}
		if i > 0 {
			n.Parent = fmt.Sprint(i - 1)
		}
		d.Nodes = append(d.Nodes, n)
	}
	deep, err := NewEditSession(ctx, d)
	mustEdit(t, err)
	defer deep.Close()
	m, err := deep.WorldTransform(fmt.Sprint(MaxNodes - 1))
	mustEdit(t, err)
	if m != Identity() {
		t.Fatal(m)
	}
	assertEditUnchanged(t, deep, func() error { _, err := deep.DuplicateSubtree(ctx, "0", DuplicateOptions{RootID: "copy"}); return err })
	mustEdit(t, deep.RemoveSubtree(ctx, "0"))
	if len(deep.Nodes()) != 0 {
		t.Fatal("deep removal")
	}
	mustEdit(t, deep.Undo(ctx))
	if deep.Modified() {
		t.Fatal("deep undo")
	}
}
func TestEditSessionReparentPreservesHiddenWorldAndExactUndo(t *testing.T) {
	ctx := context.Background()
	e, _ := editFixture(t)
	a := e.Nodes()[0]
	m := Translation(Vec3{3, 2, -1}).Mul(RotationXYZ(Vec3{.2, .3, -.7})).Mul(Scaling(Vec3{-2, 3, .5}))
	m[4] += .3
	a.Transform = m[:]
	mustEdit(t, e.UpdateNode(ctx, a))
	other := e.Nodes()[2]
	m = Translation(Vec3{-3, 1, 2}).Mul(RotationXYZ(Vec3{.5, -.2, .7}))
	other.Transform = m[:]
	mustEdit(t, e.UpdateNode(ctx, other))
	before, _ := e.Document()
	world, err := e.WorldTransform("b")
	mustEdit(t, err)
	mustEdit(t, e.Reparent(ctx, "b", "other", true))
	after, err := e.WorldTransform("b")
	mustEdit(t, err)
	for i := range world {
		if math.Abs(world[i]-after[i]) > 1e-9*math.Max(1, math.Abs(world[i])) {
			t.Fatal("hidden node jumped")
		}
	}
	mustEdit(t, e.Undo(ctx))
	d, _ := e.Document()
	if !reflect.DeepEqual(before, d) {
		t.Fatal("reparent undo changed authored matrix")
	}
	mustEdit(t, e.Reparent(ctx, "b", "", true))
	root, err := e.WorldTransform("b")
	mustEdit(t, err)
	for i := range world {
		if math.Abs(world[i]-root[i]) > 1e-9 {
			t.Fatal("root move")
		}
	}
	zero := Scaling(Vec3{0, 1, 1})
	mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "singular", Transform: zero[:]}))
	assertEditUnchanged(t, e, func() error { return e.Reparent(ctx, "b", "singular", true) })
	local := append([]float64(nil), e.Nodes()[3].Transform...)
	mustEdit(t, e.Reparent(ctx, "b", "singular", false))
	if !reflect.DeepEqual(local, e.Nodes()[3].Transform) {
		t.Fatal("keep-local changed transform")
	}
}
func TestInverseAffineAndRandomReparent(t *testing.T) {
	rng := rand.New(rand.NewSource(9124))
	for i := 0; i < 500; i++ {
		m := Translation(Vec3{rng.NormFloat64() * 10, rng.NormFloat64() * 10, rng.NormFloat64() * 10}).Mul(RotationXYZ(Vec3{rng.Float64(), rng.Float64(), rng.Float64()})).Mul(Scaling(Vec3{-.1 - rng.Float64()*4, .1 + rng.Float64()*4, .1 + rng.Float64()*4}))
		m[4] += .05
		inv, err := m.InverseAffine()
		mustEdit(t, err)
		for _, v := range []Mat4{m.Mul(inv), inv.Mul(m)} {
			for j, want := range Identity() {
				if math.Abs(v[j]-want) > 1e-10 {
					t.Fatal(i, j, v[j])
				}
			}
		}
		e, err := NewEditSession(context.Background(), &Document{Version: 1, Nodes: []DocumentNode{{ID: "a", Transform: m[:]}, {ID: "b", Transform: inv[:]}, {ID: "c", Parent: "a"}}})
		mustEdit(t, err)
		mustEdit(t, e.Reparent(context.Background(), "c", "b", true))
		actual, err := e.WorldTransform("c")
		mustEdit(t, err)
		e.Close()
		for j := range m {
			if math.Abs(m[j]-actual[j]) > 1e-9*math.Max(1, math.Abs(m[j])) {
				t.Fatal("world changed")
			}
		}
	}
	nearly := Identity()
	nearly[0] = 1
	nearly[4] = 1
	nearly[1] = 1
	nearly[5] = 1 + 1e-14
	nan := Identity()
	nan[0] = math.NaN()
	proj := Identity()
	proj[3] = 1
	for _, m := range []Mat4{{}, Scaling(Vec3{0, 1, 1}), nearly, nan, proj, Scaling(Vec3{1e-320, 1, 1})} {
		if _, err := m.InverseAffine(); err == nil {
			t.Fatal("unsafe inverse", m)
		}
	}
	scaled := Scaling(Vec3{1e-150, 1e150, -2})
	if _, err := scaled.InverseAffine(); err != nil {
		t.Fatal("valid nonuniform diagonal", err)
	}
	if n := testing.AllocsPerRun(100, func() { _, _ = scaled.InverseAffine() }); n != 0 {
		t.Fatal(n)
	}
}
func BenchmarkEditSessionRename1000(b *testing.B) {
	d := &Document{Version: 1}
	mesh := Box()
	d.Meshes = []DocumentMesh{{ID: "box", Vertices: mesh.vertices, Indices: mesh.indices}}
	for i := 0; i < 1000; i++ {
		d.Nodes = append(d.Nodes, DocumentNode{ID: fmt.Sprint(i), Mesh: "box"})
	}
	e, err := NewEditSession(context.Background(), d)
	mustEdit(b, err)
	defer e.Close()
	snap := e.scene.cached
	n := d.Nodes[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.Name = fmt.Sprint(i)
		mustEdit(b, e.UpdateNode(context.Background(), n))
	}
	b.StopTimer()
	if e.scene.cached != snap {
		b.Fatal("renaming invalidated render cache")
	}
}

func TestEditSessionActualHistoryMetadataBudget(t *testing.T) {
	d := &Document{Version: 1}
	for i := 0; i < 5000; i++ {
		d.Nodes = append(d.Nodes, DocumentNode{ID: fmt.Sprint(i), Name: strings.Repeat("x", 600)})
	}
	e, err := NewEditSession(context.Background(), d)
	mustEdit(t, err)
	defer e.Close()
	for i := 0; i < 20; i++ {
		n := d.Nodes[0]
		n.Name = fmt.Sprint(i)
		mustEdit(t, e.UpdateNode(context.Background(), n))
	}
	n, b := e.HistoryUsage()
	if n == 0 || n >= 20 || b > MaxEditHistoryBytes {
		t.Fatal("actual metadata budget not enforced", n, b)
	}
	for e.CanUndo() {
		mustEdit(t, e.Undo(context.Background()))
		_, b = e.HistoryUsage()
		if b > MaxEditHistoryBytes {
			t.Fatal("undo exceeded budget", b)
		}
	}
}

func FuzzEditSessionAtomicOperations(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	f.Add([]byte{2, 2, 3, 4, 9, 1, 5, 8, 9, 10, 0, 0})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 128 {
			ops = ops[:128]
		}
		ctx := context.Background()
		e, err := NewEditSession(ctx, &Document{Version: 1, Nodes: []DocumentNode{{ID: "root"}, {ID: "child", Parent: "root"}}})
		mustEdit(t, err)
		defer e.Close()
		for i, op := range ops {
			before, _ := e.Document()
			rev := e.Revision()
			snap := e.scene.cached
			states, size := e.HistoryUsage()
			switch op % 12 {
			case 0:
				err = e.AddNode(ctx, DocumentNode{ID: fmt.Sprintf("n%d", i), Parent: "root"})
			case 1:
				err = e.RemoveSubtree(ctx, "child")
			case 2:
				_, err = e.DuplicateSubtree(ctx, "root", DuplicateOptions{RootID: fmt.Sprintf("copy%d", i)})
			case 3:
				err = e.Reparent(ctx, "root", "child", true)
			case 4:
				err = e.Reparent(ctx, "child", "", true)
			case 5:
				err = e.SetBinding(ctx, Binding{Node: "child", Tag: "value", Property: PositionY, Gain: 1})
			case 6:
				_, err = e.RemoveBinding(ctx, "child", PositionY)
			case 7:
				err = e.Undo(ctx)
			case 8:
				err = e.Redo(ctx)
			case 9:
				err = e.AddNode(ctx, DocumentNode{ID: "root"})
			case 10:
				err = e.UpdateNode(ctx, DocumentNode{ID: "child", Parent: "root", Name: fmt.Sprint(i)})
			case 11:
				err = e.RemoveSubtree(ctx, "root")
			}
			after, _ := e.Document()
			n, b := e.HistoryUsage()
			if err != nil && (!reflect.DeepEqual(before, after) || rev != e.Revision() || snap != e.scene.cached || states != n || size != b) {
				t.Fatal("failed transaction changed state")
			}
			if n > MaxEditHistory || b > MaxEditHistoryBytes {
				t.Fatal("history budget")
			}
			built, err := after.Build(ctx)
			mustEdit(t, err)
			ref, err := built.Snapshot()
			mustEdit(t, err)
			if len(ref.instances) != len(e.scene.cached.instances) {
				t.Fatal("invalid publication")
			}
		}
	})
}
