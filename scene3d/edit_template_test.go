package scene3d

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func templateEditFixture(t testing.TB) (*EditSession, *Document, *Template) {
	t.Helper()
	s := NewScene()
	mustEdit(t, s.Add(Node{ID: "line", Transform: Translation(Vec3{Z: 4}).Mul(RotationXYZ(Vec3{Y: .4}))}))
	d, err := CaptureDocument(s, nil)
	mustEdit(t, err)
	// An unused resource must survive imports, with no ID overwrite.
	box := Box()
	d.Meshes = []DocumentMesh{{ID: "template-mesh/1", Vertices: append([]Vec3(nil), box.vertices...), Indices: box.CopyIndices(nil)}}
	d.Warnings = []string{"existing diagnostic"}
	e, err := NewEditSession(context.Background(), d)
	mustEdit(t, err)
	t.Cleanup(e.Close)
	tmpl, _ := deviceTemplateFixture(t)
	return e, d, tmpl
}

func TestEditTemplateBatchHistoryResourcesAndPersistence(t *testing.T) {
	ctx := context.Background()
	e, original, tmpl := templateEditFixture(t)
	before := e.scene.cached
	options := []InstanceOptions{{ID: "pump1", Parent: "line", TagPrefix: "p1.", Transform: Translation(Vec3{X: 2})}, {ID: "pump2", Parent: "line", TagPrefix: "p2.", TagMap: map[string]string{"alarm": "p1.alarm"}}}
	instances, err := e.InstantiateTemplates(ctx, tmpl, options)
	mustEdit(t, err)
	if len(instances) != 2 || e.Revision() != 1 {
		t.Fatal("not one transaction", len(instances), e.Revision())
	}
	if n, _ := e.HistoryUsage(); n != 1 {
		t.Fatal("not one undo state", n)
	}
	after, err := e.Document()
	mustEdit(t, err)
	if len(after.Nodes) != 11 || len(after.Bindings) != 6 || len(after.Meshes) != 2 || !reflect.DeepEqual(after.Nodes[:1], original.Nodes) || !reflect.DeepEqual(after.Meshes[0], original.Meshes[0]) {
		t.Fatal("lost original order/resources", after)
	}
	if !reflect.DeepEqual(after.Warnings, []string{"existing diagnostic", "flat geometry preview"}) {
		t.Fatal(after.Warnings)
	}
	a, _ := e.scene.Node("pump1/a")
	b, _ := e.scene.Node("pump2/a")
	if a.Mesh != b.Mesh || a.Mesh != tmpl.nodes[1].Mesh || before.InstanceCount() != 0 {
		t.Fatal("copied geometry or changed old snapshot")
	}
	if instances[1].Bindings()[2].Tag != "p1.alarm" {
		t.Fatal(instances[1].Bindings())
	}
	// Export cannot mutate immutable template resources or undo history.
	after.Meshes[1].Vertices[0].X = 12345
	after.Bindings[0].Tag = "changed"
	after.Warnings[0] = "changed"
	after, _ = e.Document()
	if after.Meshes[1].Vertices[0].X == 12345 || after.Bindings[0].Tag == "changed" || after.Warnings[0] == "changed" {
		t.Fatal("export aliases state")
	}
	mustEdit(t, e.Undo(ctx))
	undone, _ := e.Document()
	if !reflect.DeepEqual(undone, original) || e.Modified() {
		t.Fatal("undo not exact")
	}
	mustEdit(t, e.Redo(ctx))
	redone, _ := e.Document()
	if !reflect.DeepEqual(redone, after) {
		t.Fatal("redo not exact")
	}
	_, err = e.InstantiateTemplate(ctx, tmpl, InstanceOptions{ID: "pump3", TagPrefix: "p3."})
	mustEdit(t, err)
	if len(e.MeshIDs()) != 2 || len(e.Warnings()) != 2 {
		t.Fatal("repeated placement duplicates resources/diagnostics")
	}
	// The merged program is inert but usable when explicitly activated elsewhere.
	scene, err := e.Scene()
	mustEdit(t, err)
	program, err := NewBindingSet(scene, e.Bindings())
	mustEdit(t, err)
	_, err = program.Apply("p1.height", 1000.0)
	mustEdit(t, err)
	n, _ := scene.Node("pump1/parent")
	other, _ := scene.Node("pump2/parent")
	if n.Transform[13] != 1 || other.Transform[13] != 0 {
		t.Fatal("mapped instance leaked")
	}
	authored, _ := e.scene.Node("pump1/parent")
	if authored.Transform[13] != 0 {
		t.Fatal("external live scene changed authoring")
	}
	data, err := e.Document()
	mustEdit(t, err)
	path := filepath.Join(t.TempDir(), "line.silkscene")
	f, err := os.Create(path)
	mustEdit(t, err)
	err = SaveDocument(ctx, f, data)
	f.Close()
	mustEdit(t, err)
	loaded, err := LoadModelFile(ctx, path, LoadLimits{})
	mustEdit(t, err)
	reloaded, err := CaptureDocument(loaded.Scene, loaded.Bindings)
	mustEdit(t, err)
	if len(reloaded.Nodes) != len(data.Nodes) || !reflect.DeepEqual(loaded.Bindings, data.Bindings) || !reflect.DeepEqual(loaded.Warnings, data.Warnings) {
		t.Fatal("persistence lost placement")
	}
}

type templateEditCancelContext struct {
	context.Context
	remaining int
}

func (c *templateEditCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func TestEditTemplateAtomicFailuresNoopAndCancellation(t *testing.T) {
	for _, mode := range []string{"nil-template", "bad-id", "collision", "parent", "bad-tag", "combined-tag-type", "node-budget", "warning-budget", "cancel-before", "cancel-during", "nil-context"} {
		t.Run(mode, func(t *testing.T) {
			e, _, tmpl := templateEditFixture(t)
			ctx := context.Context(context.Background())
			// Keep a redo branch: failures and no-op must not discard it.
			mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "future"}))
			mustEdit(t, e.Undo(ctx))
			opts := []InstanceOptions{{ID: "placed", TagPrefix: "device."}}
			want := ErrNode
			switch mode {
			case "nil-template":
				tmpl = nil
			case "bad-id":
				opts[0].ID = "\x00"
			case "collision":
				opts[0].ID = "line"
			case "parent":
				opts[0].Parent = "missing"
			case "bad-tag":
				opts[0].TagMap = map[string]string{"typo": "new"}
				want = ErrBinding
			case "combined-tag-type":
				mustEdit(t, e.SetBinding(ctx, Binding{Node: "line", Tag: "existing.bool", Property: Visibility}))
				mustEdit(t, e.AddNode(ctx, DocumentNode{ID: "future2"}))
				mustEdit(t, e.Undo(ctx))
				opts[0].TagMap = map[string]string{"height": "existing.bool"}
				want = ErrBinding
			case "node-budget":
				opts = make([]InstanceOptions, MaxNodes)
				want = ErrLimit
			case "warning-budget":
				_, d := deviceTemplateFixture(t)
				d.Warnings = make([]string, 256)
				for i := range d.Warnings {
					d.Warnings[i] = fmt.Sprint("warning-", i)
				}
				var err error
				tmpl, err = NewTemplate(ctx, d)
				mustEdit(t, err)
				want = ErrLimit
			case "cancel-before":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = context.Canceled
			case "cancel-during":
				ctx = &templateEditCancelContext{Context: ctx, remaining: 12}
				want = context.Canceled
			case "nil-context":
				ctx = nil
				want = ErrGeometry
			}
			before, _ := e.Document()
			revision := e.Revision()
			snapshot := e.scene.cached
			history, cost := e.HistoryUsage()
			_, err := e.InstantiateTemplates(ctx, tmpl, opts)
			if !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			after, _ := e.Document()
			h, c := e.HistoryUsage()
			if !reflect.DeepEqual(before, after) || e.Revision() != revision || e.scene.cached != snapshot || h != history || c != cost || !e.CanRedo() {
				t.Fatal("failed edit changed state")
			}
		})
	}
	e, _, tmpl := templateEditFixture(t)
	mustEdit(t, e.AddNode(context.Background(), DocumentNode{ID: "future"}))
	mustEdit(t, e.Undo(context.Background()))
	before, _ := e.Document()
	revision := e.Revision()
	got, err := e.InstantiateTemplates(context.Background(), tmpl, nil)
	mustEdit(t, err)
	after, _ := e.Document()
	if len(got) != 0 || !reflect.DeepEqual(before, after) || e.Revision() != revision || !e.CanRedo() {
		t.Fatal("empty batch mutated")
	}
	e.Close()
	if _, err = e.InstantiateTemplate(context.Background(), tmpl, InstanceOptions{ID: "closed"}); !errors.Is(err, ErrEditClosed) {
		t.Fatal(err)
	}
}

func TestTemplateFileLoaderAndStableTagMetadata(t *testing.T) {
	tmpl, _ := deviceTemplateFixture(t)
	want := []TemplateTag{{"alarm", true}, {"angle", false}, {"height", false}}
	if !reflect.DeepEqual(tmpl.Tags(), want) {
		t.Fatal(tmpl.Tags())
	}
	tags := tmpl.Tags()
	tags[0].Name = "changed"
	if !reflect.DeepEqual(tmpl.Tags(), want) {
		t.Fatal("tag alias")
	}
	path := filepath.Join(t.TempDir(), "model.obj")
	mustEdit(t, os.WriteFile(path, []byte("v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n"), 0600))
	loaded, err := LoadTemplateFile(context.Background(), path, LoadLimits{})
	mustEdit(t, err)
	if loaded.NodeCount() != 1 || len(loaded.Tags()) != 0 {
		t.Fatal("bad loaded template")
	}
	e, err := NewEditSession(context.Background(), &Document{Version: DocumentVersion})
	mustEdit(t, err)
	defer e.Close()
	_, err = e.InstantiateTemplates(context.Background(), loaded, []InstanceOptions{{ID: "one"}, {ID: "two", Transform: Translation(Vec3{X: 2})}})
	mustEdit(t, err)
	a, _ := e.scene.Node("one/model")
	b, _ := e.scene.Node("two/model")
	if a.Mesh != b.Mesh || a.Mesh != loaded.nodes[0].Mesh || len(e.MeshIDs()) != 1 {
		t.Fatal("model geometry not shared")
	}
	if partial, loadErr := LoadTemplateFile(context.Background(), path, LoadLimits{MaxBytes: 1}); loadErr == nil || partial != nil {
		t.Fatal("limited load published a template", loadErr)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = LoadTemplateFile(canceled, path, LoadLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
