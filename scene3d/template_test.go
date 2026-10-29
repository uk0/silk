package scene3d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func deviceTemplateFixture(t testing.TB) (*Template, *Document) {
	t.Helper()
	s := documentScene(t)
	d, err := CaptureDocument(s, []Binding{
		{Node: "parent", Tag: "height", Property: PositionY, Gain: .001},
		{Node: "a", Tag: "angle", Property: RotationZ, Gain: 1},
		{Node: "a", Tag: "alarm", Property: AlarmColor, FalseColor: color.RGBA{30, 180, 80, 255}, TrueColor: color.RGBA{240, 40, 30, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d.Warnings = []string{"flat geometry preview"}
	tmpl, err := NewTemplate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl, d
}

func TestTemplatePlacementSharesGeometryAndIsolatesState(t *testing.T) {
	tmpl, source := deviceTemplateFixture(t)
	s := NewScene()
	if err := s.Add(Node{ID: "line", Transform: Translation(Vec3{Z: 4})}); err != nil {
		t.Fatal(err)
	}
	old, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	revision := s.Revision()
	options := []InstanceOptions{
		{ID: "pump-1", Parent: "line", Transform: Translation(Vec3{X: -2}), TagPrefix: "pump1."},
		{ID: "pump-2", Parent: "line", Transform: Translation(Vec3{X: 2}), TagPrefix: "pump2.", TagMap: map[string]string{"alarm": "line.sharedAlarm"}},
	}
	placed, err := tmpl.InstantiateBatch(context.Background(), s, options)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.NodeCount() != 4 || len(placed) != 2 || s.Revision() != revision+1 || old.InstanceCount() != 0 || s.cached == old {
		t.Fatal("placement did not commit exactly once", tmpl.NodeCount(), len(placed), s.Revision())
	}
	firstID, ok := placed[0].NodeID("a")
	if !ok || firstID != "pump-1/a" || placed[0].RootID() != "pump-1" {
		t.Fatal(firstID, ok)
	}
	if _, ok := placed[0].NodeID("unknown"); ok {
		t.Fatal("accepted unknown local ID")
	}
	first, _ := s.Node(firstID)
	second, _ := s.Node("pump-2/a")
	other, _ := s.Node("pump-1/other")
	if first.Mesh != second.Mesh || first.Mesh != other.Mesh || first.Mesh != tmpl.nodes[1].Mesh {
		t.Fatal("geometry copied instead of shared")
	}
	// The authored topological / coplanar draw order is preserved in every copy.
	wantOrder := []string{"line", "pump-1", "pump-1/parent", "pump-1/a", "pump-1/other", "pump-1/b", "pump-2", "pump-2/parent", "pump-2/a", "pump-2/other", "pump-2/b"}
	if !reflect.DeepEqual(s.order, wantOrder) {
		t.Fatal(s.order)
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.InstanceCount() != 4 {
		t.Fatal(snapshot.InstanceCount())
	}
	instance, _ := snapshot.Instance(0)
	if instance.Transform[12] != -1.8 || instance.Transform[14] != 4 {
		t.Fatal(instance.Transform)
	}
	var bindings []Binding
	for _, item := range placed {
		bindings = append(bindings, item.Bindings()...)
	}
	plan, err := NewBindingSet(s, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("pump1.height", 1250.0); err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("pump1.angle", math.Pi/2); err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("line.sharedAlarm", true); err != nil {
		t.Fatal(err)
	}
	n, _ := s.Node("pump-1/parent")
	otherParent, _ := s.Node("pump-2/parent")
	changedFirst, _ := s.Node(firstID)
	changedSecond, _ := s.Node("pump-2/a")
	if n.Transform[13] != 1.25 || otherParent.Transform[13] != 0 || changedFirst.Transform == first.Transform || changedSecond.Transform != second.Transform || changedSecond.Color.R != 240 {
		t.Fatal("instance state leaked", n, otherParent, changedFirst, changedSecond)
	}
	if original, _ := snapshot.Instance(0); original.Transform != instance.Transform {
		t.Fatal("live update mutated old snapshot")
	}
	// Mutating all caller-owned data / returned slices cannot mutate the template.
	source.Meshes[0].Vertices[0].X = 900
	source.Nodes[0].Transform[12] = 900
	source.Bindings[0].Tag = "changed"
	source.Warnings[0] = "changed"
	options[1].TagMap["alarm"] = "changed"
	returned := placed[0].Bindings()
	returned[0].Tag = "changed"
	warnings := tmpl.Warnings()
	warnings[0] = "changed"
	if placed[0].Bindings()[0].Tag != "pump1.height" || placed[1].Bindings()[2].Tag != "line.sharedAlarm" || tmpl.Warnings()[0] != "flat geometry preview" {
		t.Fatal("mutable metadata alias")
	}
	fresh := NewScene()
	_, err = tmpl.Instantiate(context.Background(), fresh, InstanceOptions{ID: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	freshNode, _ := fresh.Node("fresh/a")
	vertex, _ := freshNode.Mesh.Vertex(0)
	freshParent, _ := fresh.Node("fresh/parent")
	if vertex.X == 900 || freshParent.Transform[12] != .2 || freshNode.Mesh != first.Mesh {
		t.Fatal("source document or instance mutated template")
	}
	if !s.Remove(placed[0].RootID()) {
		t.Fatal("cannot remove instance")
	}
	if _, ok := s.Node("pump-1/a"); ok {
		t.Fatal("removed instance left child")
	}
	if _, ok := s.Node("pump-2/a"); !ok {
		t.Fatal("removed another instance")
	}
}

func TestTemplatePlacementFailureIsAtomic(t *testing.T) {
	tmpl, _ := deviceTemplateFixture(t)
	for _, tc := range []struct {
		name    string
		options []InstanceOptions
		want    error
	}{
		{"empty-id", []InstanceOptions{{}}, ErrNode},
		{"duplicate-root", []InstanceOptions{{ID: "existing"}}, ErrNode},
		{"missing-parent", []InstanceOptions{{ID: "new", Parent: "missing"}}, ErrNode},
		{"self-parent", []InstanceOptions{{ID: "new", Parent: "new"}}, ErrNode},
		{"bad-name", []InstanceOptions{{ID: "new", Name: "\x00"}}, ErrNode},
		{"bad-id-utf8", []InstanceOptions{{ID: string([]byte{255})}}, ErrNode},
		{"long-qualified-id", []InstanceOptions{{ID: strings.Repeat("x", 253)}}, ErrNode},
		{"non-affine", []InstanceOptions{{ID: "new", Transform: Mat4{15: 2}}}, ErrNode},
		{"nan", []InstanceOptions{{ID: "new", Transform: Translation(Vec3{X: math.NaN()})}}, ErrNode},
		{"unknown-tag", []InstanceOptions{{ID: "new", TagMap: map[string]string{"typo": "value"}}}, ErrBinding},
		{"empty-tag", []InstanceOptions{{ID: "new", TagMap: map[string]string{"height": ""}}}, ErrBinding},
		{"long-tag", []InstanceOptions{{ID: "new", TagPrefix: strings.Repeat("x", 512)}}, ErrBinding},
		{"nul-prefix", []InstanceOptions{{ID: "new", TagPrefix: "\x00"}}, ErrBinding},
		{"mixed-type-alias", []InstanceOptions{{ID: "new", TagMap: map[string]string{"height": "same", "alarm": "same"}}}, ErrBinding},
		{"late-root-collision", []InstanceOptions{{ID: "new"}, {ID: "new"}}, ErrNode},
		{"late-child-collision", []InstanceOptions{{ID: "new"}, {ID: "new/a"}}, ErrNode},
		{"late-parent", []InstanceOptions{{ID: "new", Parent: "later"}, {ID: "later"}}, ErrNode},
		{"cross-instance-mixed-tag", []InstanceOptions{{ID: "one", TagPrefix: "one.", TagMap: map[string]string{"height": "shared"}}, {ID: "two", TagPrefix: "two.", TagMap: map[string]string{"alarm": "shared"}}}, ErrBinding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScene()
			if err := s.Add(Node{ID: "existing", Mesh: Box()}); err != nil {
				t.Fatal(err)
			}
			before := s.Nodes()
			revision := s.Revision()
			cached, err := s.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			placed, err := tmpl.InstantiateBatch(context.Background(), s, tc.options)
			if !errors.Is(err, tc.want) || placed != nil {
				t.Fatalf("got %v, %v; want %v", placed, err, tc.want)
			}
			if !reflect.DeepEqual(before, s.Nodes()) || s.Revision() != revision || s.cached != cached {
				t.Fatal("failed transaction changed destination")
			}
		})
	}
}

// Cancellation occurs during work, not merely on entry, without sleeps/races.
type templateCancelContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *templateCancelContext) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestTemplateCancellationAndLimits(t *testing.T) {
	tmpl, _ := deviceTemplateFixture(t)
	options := make([]InstanceOptions, 100)
	for i := range options {
		options[i] = InstanceOptions{ID: fmt.Sprintf("pump-%d", i), TagPrefix: fmt.Sprintf("pump%d.", i)}
	}
	for _, at := range []int{1, 3, 20, 400, 802} {
		t.Run(fmt.Sprintf("cancel-%d", at), func(t *testing.T) {
			s := NewScene()
			cached, _ := s.Snapshot()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			probe := &templateCancelContext{Context: ctx, cancel: cancel, remaining: at}
			if _, err := tmpl.InstantiateBatch(probe, s, options); !errors.Is(err, context.Canceled) {
				t.Fatal(err, probe.remaining)
			}
			if len(s.nodes) != 0 || s.Revision() != 0 || s.cached != cached {
				t.Fatal("cancelled batch partially committed")
			}
		})
	}
	s := NewScene()
	tooMany := make([]InstanceOptions, MaxNodes/(tmpl.NodeCount()+1)+1)
	if _, err := tmpl.InstantiateBatch(context.Background(), s, tooMany); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	// The unique geometry is legal; three visible copies exceed the instance
	// triangle budget. Hiding them is explicit, not a silent render-time omission.
	d := &Document{Version: DocumentVersion, Meshes: []DocumentMesh{{ID: "mesh", Vertices: []Vec3{{}, {X: 1}, {Y: 1}}, Indices: make([]uint32, 400000*3)}}, Nodes: []DocumentNode{{ID: "mesh-node", Mesh: "mesh"}}}
	for i := 0; i < len(d.Meshes[0].Indices); i += 3 {
		d.Meshes[0].Indices[i+1] = 1
		d.Meshes[0].Indices[i+2] = 2
	}
	large, err := NewTemplate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	batch := []InstanceOptions{{ID: "one"}, {ID: "two"}, {ID: "three"}}
	cached, _ := s.Snapshot()
	if _, err = large.InstantiateBatch(context.Background(), s, batch); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if len(s.nodes) != 0 || s.cached != cached || s.Revision() != 0 {
		t.Fatal("budget failure changed destination")
	}
	batch[2].Hidden = true
	if _, err = large.InstantiateBatch(context.Background(), s, batch); err != nil {
		t.Fatal(err)
	}
	if snap, err := s.Snapshot(); err != nil || snap.InstanceCount() != 2 {
		t.Fatal(err, snap.InstanceCount())
	}
	// Finite local matrices may still overflow when composed with a parent.
	world := NewScene()
	if err = world.Add(Node{ID: "huge", Transform: Scaling(Vec3{1e308, 1, 1})}); err != nil {
		t.Fatal(err)
	}
	cached, _ = world.Snapshot()
	revision := world.Revision()
	if _, err = tmpl.Instantiate(context.Background(), world, InstanceOptions{ID: "new", Parent: "huge", Transform: Scaling(Vec3{2, 1, 1})}); !errors.Is(err, ErrNode) {
		t.Fatal(err)
	}
	if len(world.nodes) != 1 || world.cached != cached || world.Revision() != revision {
		t.Fatal("world overflow changed scene")
	}
}

func TestTemplateNilEmptyAndNestedPlacement(t *testing.T) {
	ctx := context.Background()
	if _, err := NewTemplate(ctx, nil); !errors.Is(err, ErrGeometry) {
		t.Fatal(err)
	}
	if _, err := NewTemplate(nil, &Document{Version: DocumentVersion}); err == nil {
		t.Fatal("nil context")
	}
	if _, err := NewTemplate(ctx, &Document{Version: DocumentVersion, Nodes: []DocumentNode{{ID: "a", Parent: "a"}}}); err == nil {
		t.Fatal("invalid document")
	}
	var absent *Template
	if absent.NodeCount() != 0 || absent.Warnings() != nil {
		t.Fatal("nil template metadata")
	}
	if _, err := absent.Instantiate(ctx, NewScene(), InstanceOptions{ID: "a"}); err == nil {
		t.Fatal("nil template")
	}
	if _, err := (&Template{}).Instantiate(ctx, NewScene(), InstanceOptions{ID: "a"}); err == nil {
		t.Fatal("uncompiled template")
	}
	var missing *TemplateInstance
	if missing.RootID() != "" || missing.Bindings() != nil {
		t.Fatal("nil instance metadata")
	}
	if _, ok := missing.NodeID("a"); ok {
		t.Fatal("nil instance ID")
	}
	empty, err := NewTemplate(ctx, &Document{Version: DocumentVersion})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = empty.Instantiate(ctx, nil, InstanceOptions{ID: "a"}); err == nil {
		t.Fatal("nil destination")
	}
	if _, err = empty.Instantiate(nil, NewScene(), InstanceOptions{ID: "a"}); err == nil {
		t.Fatal("nil context")
	}
	var s Scene
	cached, _ := s.Snapshot()
	if result, err := empty.InstantiateBatch(ctx, &s, nil); err != nil || len(result) != 0 || s.Revision() != 0 || s.cached != cached {
		t.Fatal("empty batch mutated scene", err)
	}
	items, err := empty.InstantiateBatch(ctx, &s, []InstanceOptions{{ID: "line"}, {ID: "station", Parent: "line"}})
	if err != nil || len(items) != 2 || s.Revision() != 1 {
		t.Fatal(err, len(items), s.Revision())
	}
	n, _ := s.Node("station")
	if n.Parent != "line" || n.Transform != Identity() {
		t.Fatal(n)
	}
}

func TestTemplateConcurrentDestinationsAndMetadata(t *testing.T) {
	tmpl, _ := deviceTemplateFixture(t)
	var wg sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 10; iteration++ {
				s := NewScene()
				item, err := tmpl.Instantiate(context.Background(), s, InstanceOptions{ID: "device", TagPrefix: "local."})
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := s.Snapshot(); err != nil {
					t.Error(err)
					return
				}
				b := item.Bindings()
				b[0].Tag = "changed"
				w := tmpl.Warnings()
				w[0] = "changed"
				if id, ok := item.NodeID("a"); !ok || id != "device/a" {
					t.Error(id, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestTemplateModelLoadBindSaveReloadPixelsAndPicking(t *testing.T) {
	ctx := context.Background()
	loaded, err := LoadModelFile(ctx, filepath.Join("..", "examples", "scene3d", "assembly.silkscene"), LoadLimits{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := CaptureDocument(loaded.Scene, loaded.Bindings)
	if err != nil {
		t.Fatal(err)
	}
	d.Warnings = loaded.Warnings
	tmpl, err := NewTemplate(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	s := NewScene()
	placements := []InstanceOptions{{ID: "left", Transform: Translation(Vec3{X: -2.2}), TagPrefix: "left."}, {ID: "right", Transform: Translation(Vec3{X: 2.2}), TagPrefix: "right."}}
	items, err := tmpl.InstantiateBatch(ctx, s, placements)
	if err != nil {
		t.Fatal(err)
	}
	var bindings []Binding
	for _, item := range items {
		bindings = append(bindings, item.Bindings()...)
	}
	plan, err := NewBindingSet(s, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("left.tank.rotation", .3); err != nil {
		t.Fatal(err, plan.Tags())
	}
	// Export authored values; binding replay after reload must reproduce both
	// the exact geometry and stable picking IDs, not compound the rotation.
	out, err := plan.CaptureDocument()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Meshes) != len(d.Meshes) || len(out.Nodes) != 2*(len(d.Nodes)+1) {
		t.Fatal("duplicated mesh data", len(out.Meshes), len(out.Nodes))
	}
	var raw bytes.Buffer
	if err = SaveDocument(ctx, &raw, out); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := LoadDocument(ctx, bytes.NewReader(raw.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := roundtrip.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restoredPlan, err := NewBindingSet(restored, roundtrip.Bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restoredPlan.Apply("left.tank.rotation", .3); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Snapshot()
	second, _ := restored.Snapshot()
	c := DefaultCamera()
	c.Eye = Vec3{8, 7, 14}
	var r1, r2 Renderer
	f1, err := r1.Render(first, c, 240, 160, color.RGBA{A: 255})
	if err != nil {
		t.Fatal(err)
	}
	f2, err := r2.Render(second, c, 240, 160, color.RGBA{A: 255})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f1.Image.Pix, f2.Image.Pix) {
		t.Fatal("placement roundtrip changed pixels")
	}
	seen := map[string]bool{}
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			a, b := f1.Pick(x, y), f2.Pick(x, y)
			if a != b {
				t.Fatal("picking changed", x, y, a, b)
			}
			if a != "" {
				seen[strings.SplitN(a, "/", 2)[0]] = true
			}
		}
	}
	if !seen["left"] || !seen["right"] {
		t.Fatal("both placed models must really render", seen)
	}
}

func BenchmarkTemplateBatchPlacement(b *testing.B) {
	tmpl, _ := deviceTemplateFixture(b)
	for _, count := range []int{1, 100, 1000} {
		options := make([]InstanceOptions, count)
		for i := range options {
			options[i] = InstanceOptions{ID: fmt.Sprintf("pump-%d", i), TagPrefix: fmt.Sprintf("pump%d.", i)}
		}
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tmpl.InstantiateBatch(context.Background(), NewScene(), options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestTemplateMappedNumericFanoutAndBindingTextBudget(t *testing.T) {
	tmpl, _ := deviceTemplateFixture(t)
	s := NewScene()
	item, err := tmpl.Instantiate(context.Background(), s, InstanceOptions{ID: "shared", TagMap: map[string]string{"height": "value", "angle": "value"}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewBindingSet(s, item.Bindings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("value", 1.0); err != nil {
		t.Fatal(err)
	}
	parent, _ := s.Node("shared/parent")
	rotated, _ := s.Node("shared/a")
	if parent.Transform[13] != .001 || math.Abs(rotated.Transform[0]-math.Cos(1)) > 1e-12 {
		t.Fatal("same-type tag fanout lost", parent, rotated)
	}
	// Replication must not evade the aggregate text budget accepted by a single
	// BindingSet. The source itself and each individual placement are legal.
	base := NewScene()
	id := strings.Repeat("n", 230)
	if err = base.Add(Node{ID: id, Mesh: Box()}); err != nil {
		t.Fatal(err)
	}
	var specs []Binding
	for channel, property := range bindingProperties {
		b := Binding{Node: id, Tag: strings.Repeat("n", 512), Property: property, Gain: 1}
		if channel >= 9 {
			b.Tag = strings.Repeat("b", 512)
			b.Gain = 0
		}
		if property == AlarmColor {
			b.FalseColor = color.RGBA{A: 255}
			b.TrueColor = color.RGBA{R: 255, A: 255}
		}
		specs = append(specs, b)
	}
	d, err := CaptureDocument(base, specs)
	if err != nil {
		t.Fatal(err)
	}
	large, err := NewTemplate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	options := make([]InstanceOptions, 800)
	for i := range options {
		options[i].ID = fmt.Sprint(i)
	}
	before := s.Nodes()
	cached, _ := s.Snapshot()
	revision := s.Revision()
	if _, err = large.InstantiateBatch(context.Background(), s, options); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, s.Nodes()) || cached != s.cached || revision != s.Revision() {
		t.Fatal("text limit partially committed")
	}
}

func FuzzTemplatePlacementTransactions(f *testing.F) {
	tmpl, _ := deviceTemplateFixture(f)
	f.Add("one", "two", "device.", "level", "alarm", false)
	f.Add("one", "one/a", "", "same", "same", true)
	f.Fuzz(func(t *testing.T, firstID, secondID, prefix, numeric, boolean string, hidden bool) {
		if len(firstID) > 300 || len(secondID) > 300 || len(prefix) > 600 || len(numeric) > 600 || len(boolean) > 600 {
			t.Skip()
		}
		s := NewScene()
		if err := s.Add(Node{ID: "existing", Mesh: Box()}); err != nil {
			t.Fatal(err)
		}
		cached, _ := s.Snapshot()
		before := s.Nodes()
		revision := s.Revision()
		items, err := tmpl.InstantiateBatch(context.Background(), s, []InstanceOptions{
			{ID: firstID, TagPrefix: prefix, Hidden: hidden, TagMap: map[string]string{"height": numeric, "alarm": boolean}},
			{ID: secondID, TagPrefix: prefix},
		})
		if err != nil {
			if items != nil || s.Revision() != revision || s.cached != cached || !reflect.DeepEqual(before, s.Nodes()) {
				t.Fatal("failed placement changed destination")
			}
			return
		}
		var bindings []Binding
		for _, item := range items {
			bindings = append(bindings, item.Bindings()...)
		}
		if _, err = NewBindingSet(s, bindings); err != nil {
			t.Fatal("accepted invalid remapped binding plan", err)
		}
		if _, err = s.Snapshot(); err != nil {
			t.Fatal("accepted invalid snapshot", err)
		}
		if s.Revision() != revision+1 {
			t.Fatal("not one atomic revision")
		}
		if len(s.Nodes()) != len(before)+2*(tmpl.NodeCount()+1) {
			t.Fatal("lost or overwritten source nodes")
		}
	})
}
