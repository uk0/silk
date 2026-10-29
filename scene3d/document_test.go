package scene3d

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func documentScene(t testing.TB) *Scene {
	t.Helper()
	s := NewScene()
	box := Box()
	// Preserve interleaved roots/children, including coplanar tie order.
	for _, n := range []Node{
		{ID: "parent", Name: "设备组", Transform: Translation(Vec3{X: .2})},
		{ID: "a", Parent: "parent", Mesh: box, Color: color.RGBA{210, 80, 30, 255}},
		{ID: "other", Mesh: box, Color: color.RGBA{30, 180, 80, 255}},
		{ID: "b", Parent: "parent", Mesh: box, Hidden: true, Transform: Translation(Vec3{Z: 2})},
	} {
		if e := s.Add(n); e != nil {
			t.Fatal(e)
		}
	}
	return s
}
func TestSceneDocumentRoundtripPreservesPixelsPickingAndSharedMesh(t *testing.T) {
	ctx := context.Background()
	s := documentScene(t)
	d, e := CaptureDocument(s, []Binding{{Node: "a", Tag: "设备.level", Property: PositionY, Gain: .01}})
	if e != nil {
		t.Fatal(e)
	}
	d.Warnings = []string{"flat geometry preview"}
	if len(d.Meshes) != 1 {
		t.Fatal("lost mesh deduplication")
	}
	var out bytes.Buffer
	if e = SaveDocument(ctx, &out, d); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "falseColor") {
		t.Fatal("zero optional colors inflated document")
	}
	loaded, e := LoadDocument(ctx, bytes.NewReader(out.Bytes()))
	if e != nil {
		t.Fatal(e)
	}
	restored, e := loaded.Build(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(loaded, d) {
		t.Fatal("document metadata changed")
	}
	if !reflect.DeepEqual(s.order, restored.order) {
		t.Fatal("coplanar draw order changed", s.order, restored.order)
	}
	a, _ := restored.Node("a")
	b, _ := restored.Node("b")
	if a.Mesh != b.Mesh {
		t.Fatal("shared mesh copied per instance")
	}
	snap, _ := s.Snapshot()
	other, _ := restored.Snapshot()
	var r1, r2 Renderer
	first, e := r1.Render(snap, DefaultCamera(), 160, 120, color.RGBA{A: 255})
	if e != nil {
		t.Fatal(e)
	}
	second, e := r2.Render(other, DefaultCamera(), 160, 120, color.RGBA{A: 255})
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(first.Image.Pix, second.Image.Pix) {
		t.Fatal("render changed on save/load")
	}
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			if first.Pick(x, y) != second.Pick(x, y) {
				t.Fatal("picking order changed")
			}
		}
	}
	loaded.Meshes[0].Vertices[0].X = 900
	loaded.Nodes[1].Transform[12] = 900
	nodes := restored.Nodes()
	nodes[1].Transform[12] = 700
	actual, _ := restored.Node("a")
	if actual.Transform[12] != 0 {
		t.Fatal("document or Nodes exposes mutable scene")
	}
	lo, _ := a.Mesh.Bounds()
	if lo.X != -1 {
		t.Fatal("document mutation changed built mesh")
	}
	if e = s.SetTransform("a", Translation(Vec3{X: 4})); e != nil {
		t.Fatal(e)
	}
	if d.Nodes[1].Transform[12] != 0 {
		t.Fatal("capture aliased live scene")
	}
}

func TestSceneDocumentUnorderedHierarchyAndInvalidInputs(t *testing.T) {
	ctx := context.Background()
	d := &Document{Version: 1, Nodes: []DocumentNode{{ID: "child", Parent: "root"}, {ID: "root"}}}
	s, e := d.Build(ctx)
	if e != nil || !reflect.DeepEqual(s.order, []string{"root", "child"}) {
		t.Fatal(s, e)
	}
	tests := map[string]string{
		"version": `{"version":2}`, "missing-version": `{}`,
		"unknown": `{"version":1,"code":"exec"}`, "duplicate": `{"version":1,"version":1}`,
		"case-duplicate": `{"version":1,"Version":1}`, "trailing": `{"version":1} {}`,
		"cycle":        `{"version":1,"nodes":[{"id":"a","parent":"b"},{"id":"b","parent":"a"}]}`,
		"parent":       `{"version":1,"nodes":[{"id":"a","parent":"missing"}]}`,
		"mesh":         `{"version":1,"nodes":[{"id":"a","mesh":"missing"}]}`,
		"ids":          `{"version":1,"nodes":[{"id":"a"},{"id":"a"}]}`,
		"matrix-short": `{"version":1,"nodes":[{"id":"a","transform":[1,0]}]}`,
		"matrix-long":  `{"version":1,"nodes":[{"id":"a","transform":[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1,9]}]}`,
		"binding":      `{"version":1,"bindings":[{"node":"missing","tag":"x","property":"position.x","gain":1}]}`,
		"utf8":         "{\"version\":1,\"warnings\":[\"\xff\"]}",
		"depth":        `{"version":1,"unknown":` + strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34) + "}",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, e := LoadDocument(ctx, strings.NewReader(input)); e == nil {
				t.Fatal("invalid document accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*Document){
		"nan":               func(d *Document) { d.Nodes[0].Transform[0] = math.NaN() },
		"index":             func(d *Document) { d.Meshes[0].Indices[0] = 999 },
		"duplicate-mesh":    func(d *Document) { d.Meshes = append(d.Meshes, d.Meshes[0]) },
		"alpha":             func(d *Document) { d.Nodes[1].Color.A = 1 },
		"oversize-nodes":    func(d *Document) { d.Nodes = make([]DocumentNode, MaxNodes+1) },
		"oversize-vertices": func(d *Document) { d.Meshes[0].Vertices = make([]Vec3, MaxVertices+1) },
		"bad-tag":           func(d *Document) { d.Bindings = []Binding{{Node: "a", Tag: "x\x00y", Property: PositionX, Gain: 1}} },
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := CaptureDocument(documentScene(t), nil)
			mutate(d)
			var out bytes.Buffer
			if e := SaveDocument(ctx, &out, d); e == nil || out.Len() != 0 {
				t.Fatal("invalid save wrote output", e, out.Len())
			}
		})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = LoadDocument(canceled, strings.NewReader(`{"version":1}`)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = LoadDocument(nil, strings.NewReader(`{}`)); e == nil {
		t.Fatal("nil context")
	}
	if _, e = LoadDocument(ctx, nil); e == nil {
		t.Fatal("nil reader")
	}
	if _, e = LoadDocument(ctx, io.LimitReader(zeroDocumentReader{}, MaxDocumentBytes+1)); !errors.Is(e, ErrLimit) {
		t.Fatal("byte budget", e)
	}
	if e = SaveDocument(ctx, shortDocumentWriter{}, &Document{Version: 1}); !errors.Is(e, io.ErrShortWrite) {
		t.Fatal(e)
	}
}

type zeroDocumentReader struct{}

func (zeroDocumentReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type shortDocumentWriter struct{}

func (shortDocumentWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestSceneBindingsComposeAtomicSamplesAndAuthoredCapture(t *testing.T) {
	s := documentScene(t)
	base := Translation(Vec3{1, 2, 3}).Mul(RotationXYZ(Vec3{Y: .2})).Mul(Scaling(Vec3{2, 3, 4}))
	if e := s.SetTransform("a", base); e != nil {
		t.Fatal(e)
	}
	specs := []Binding{
		{Node: "a", Tag: "height", Property: PositionY, Gain: .01, Offset: -.2},
		{Node: "a", Tag: "angle", Property: RotationZ, Gain: math.Pi / 180},
		{Node: "a", Tag: "size", Property: ScaleX, Gain: 1},
		{Node: "a", Tag: "show", Property: Visibility},
		{Node: "a", Tag: "alarm", Property: AlarmColor, FalseColor: color.RGBA{20, 200, 50, 255}, TrueColor: color.RGBA{240, 40, 20, 255}},
	}
	b, e := NewBindingSet(s, specs)
	if e != nil {
		t.Fatal(e)
	}
	for _, sample := range []struct {
		tag   string
		value any
	}{{"angle", 90.0}, {"size", 2.0}, {"height", int64(120)}, {"show", false}, {"alarm", true}} {
		if _, e = b.Apply(sample.tag, sample.value); e != nil {
			t.Fatal(e)
		}
	}
	n, _ := s.Node("a")
	want := base.Mul(RotationXYZ(Vec3{Z: math.Pi / 2})).Mul(Scaling(Vec3{2, 1, 1}))
	want[13] = 1
	if n.Transform != want || !n.Hidden || n.Color != specs[4].TrueColor {
		t.Fatal("combined channels lost", n, want)
	}
	rev := s.Revision()
	if changed, e := b.Apply("height", 120.0); e != nil || changed || rev != s.Revision() {
		t.Fatal("equal sample dirtied scene", e)
	}
	for _, v := range []any{math.NaN(), math.Inf(1), "120", true, nil, json.Number("1e999")} {
		if _, e = b.Apply("height", v); !errors.Is(e, ErrSample) || s.Revision() != rev {
			t.Fatal("bad sample mutated", v, e)
		}
	}
	if _, e = b.Apply("missing", 1.0); !errors.Is(e, ErrBinding) {
		t.Fatal(e)
	}
	// Save live geometry without accumulating a bound rotation/scale on reload.
	doc, e := b.CaptureDocument()
	if e != nil {
		t.Fatal(e)
	}
	restored, e := doc.Build(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	authored, _ := restored.Node("a")
	if authored.Transform != base || authored.Hidden {
		t.Fatal("live sample became authored base")
	}
	again, e := NewBindingSet(restored, doc.Bindings)
	if e != nil {
		t.Fatal(e)
	}
	_, _ = again.Apply("height", 120.0)
	_, _ = again.Apply("size", 2.0)
	_, _ = again.Apply("angle", 90.0)
	final, _ := restored.Node("a")
	if final.Transform != want {
		t.Fatal("reload accumulated transforms")
	}
	// Replacing the live binding program does not bake its current output.
	next, e := b.Reconfigure(specs)
	if e != nil {
		t.Fatal(e)
	}
	reset, _ := s.Node("a")
	if reset.Transform != base || reset.Hidden {
		t.Fatal("reconfiguration did not restore authored channels")
	}
	_, _ = next.Apply("height", 120.0)
	_, _ = next.Apply("size", 2.0)
	_, _ = next.Apply("angle", 90.0)
	reset, _ = s.Node("a")
	if reset.Transform != want {
		t.Fatal("reconfiguration compounded rotation")
	}
	rev = s.Revision()
	if _, e = next.Reconfigure([]Binding{{Node: "missing", Tag: "height", Property: PositionY, Gain: 1}}); e == nil || rev != s.Revision() {
		t.Fatal("bad reconfiguration mutated live state")
	}
	cleared, e := next.Reconfigure(nil)
	if e != nil {
		t.Fatal(e)
	}
	reset, _ = s.Node("a")
	if reset.Transform != base || len(cleared.Bindings()) != 0 {
		t.Fatal("clearing bindings baked runtime output")
	}
	copySpecs := b.Bindings()
	copySpecs[0].Gain = 999
	tags := b.Tags()
	tags[0] = "bad"
	if b.Bindings()[0].Gain != .01 || b.Tags()[0] == "bad" {
		t.Fatal("exposed configuration")
	}
}

func TestSceneBindingFanoutIsAtomicAndZeroAllocation(t *testing.T) {
	s := documentScene(t)
	b, e := NewBindingSet(s, []Binding{{Node: "a", Tag: "x", Property: PositionX, Gain: 1}, {Node: "other", Tag: "x", Property: PositionX, Gain: 1e308}})
	if e != nil {
		t.Fatal(e)
	}
	rev := s.Revision()
	if _, e = b.Apply("x", 2.0); !errors.Is(e, ErrSample) || s.Revision() != rev {
		t.Fatal("partial fanout on overflow", e)
	}
	n, _ := s.Node("a")
	if n.Transform[12] != 0 {
		t.Fatal("first destination already changed")
	}
	if _, e = b.Apply("x", 0.0); e != nil {
		t.Fatal(e)
	}
	s.Remove("other")
	rev = s.Revision()
	if _, e = b.Apply("x", 1.0); !errors.Is(e, ErrNode) || s.Revision() != rev {
		t.Fatal("partial fanout after removal")
	}
	for _, specs := range [][]Binding{
		{{Node: "a", Tag: "t", Property: "unknown"}},
		{{Node: "a", Tag: "t", Property: PositionX}, {Node: "a", Tag: "u", Property: PositionX}},
		{{Node: "a", Tag: "t", Property: PositionX}, {Node: "a", Tag: "t", Property: Visibility}},
		{{Node: "a", Tag: "t", Property: Visibility, Gain: 1}},
		{{Node: "a", Tag: "t", Property: AlarmColor}},
	} {
		if _, e = NewBindingSet(s, specs); !errors.Is(e, ErrBinding) {
			t.Fatal("invalid binding", specs, e)
		}
	}
	b, e = NewBindingSet(s, []Binding{{Node: "a", Tag: "angle", Property: RotationZ, Gain: 1}})
	if e != nil {
		t.Fatal(e)
	}
	value := .25
	if allocs := testing.AllocsPerRun(1000, func() {
		value = -value
		if _, e := b.Apply("angle", value); e != nil {
			panic(e)
		}
	}); allocs != 0 {
		t.Fatal("per-sample allocations", allocs)
	}
}

func FuzzLoadSceneDocument(f *testing.F) {
	d, _ := CaptureDocument(documentScene(f), nil)
	raw, _ := json.Marshal(d)
	f.Add(raw)
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte(`{"version":1,"nodes":[{"id":"a","parent":"a"}]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		d, e := LoadDocument(context.Background(), bytes.NewReader(raw))
		if e != nil {
			return
		}
		s, e := d.Build(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Snapshot(); e != nil {
			t.Fatal(e)
		}
	})
}
func BenchmarkSceneBindingFanout(b *testing.B) {
	s := NewScene()
	specs := make([]Binding, 1000)
	mesh := Box()
	for i := range specs {
		id := string(rune(0x1000 + i))
		if e := s.Add(Node{ID: id, Mesh: mesh}); e != nil {
			b.Fatal(e)
		}
		specs[i] = Binding{Node: id, Tag: "height", Property: PositionY, Gain: 1}
	}
	set, e := NewBindingSet(s, specs)
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e = set.Apply("height", float64(i)); e != nil {
			b.Fatal(e)
		}
	}
}

func TestSceneDocumentModelFileLimitsAndInertMetadata(t *testing.T) {
	d, e := CaptureDocument(documentScene(t), []Binding{{Node: "a", Tag: "height", Property: PositionY, Gain: 1}})
	if e != nil {
		t.Fatal(e)
	}
	var data bytes.Buffer
	if e = SaveDocument(context.Background(), &data, d); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "assembly.SILKSCENE")
	if e = os.WriteFile(path, data.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadModelFile(context.Background(), path, LoadLimits{})
	if e != nil {
		t.Fatal(e)
	}
	if len(loaded.Bindings) != 1 || len(loaded.Scene.Nodes()) != 4 {
		t.Fatal("lost scene metadata")
	}
	for _, limits := range []LoadLimits{{MaxBytes: 1}, {MaxVertices: 1}, {MaxTriangles: 1}} {
		if _, e = LoadModelFile(context.Background(), path, limits); !errors.Is(e, ErrLimit) {
			t.Fatal("ignored caller budget", limits, e)
		}
	}
}

func TestSaveSceneDocumentFilePreservesDestinationOnFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.silkscene")
	original := []byte("original file")
	if e := os.WriteFile(path, original, 0640); e != nil {
		t.Fatal(e)
	}
	if e := SaveDocumentFile(ctx, path, &Document{Version: 99}); e == nil {
		t.Fatal("bad version accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := SaveDocumentFile(canceled, path, &Document{Version: 1}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(raw, original) {
		t.Fatal("failure truncated destination", e)
	}
	d, _ := CaptureDocument(documentScene(t), nil)
	for i := 0; i < 2; i++ {
		if e = SaveDocumentFile(ctx, path, d); e != nil {
			t.Fatal(e)
		}
	}
	loaded, e := LoadModelFile(ctx, path, LoadLimits{})
	if e != nil || len(loaded.Scene.Nodes()) != 4 {
		t.Fatal("replace/readback failed", e)
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed", info.Mode())
	}
	if e = SaveDocumentFile(ctx, dir, d); !errors.Is(e, ErrUnsupported) {
		t.Fatal("directory accepted", e)
	}
	link := filepath.Join(dir, "linked.silkscene")
	if e = os.Symlink(path, link); e == nil {
		if e = SaveDocumentFile(ctx, link, d); !errors.Is(e, ErrUnsupported) {
			t.Fatal("symlink destination accepted", e)
		}
		info, e = os.Lstat(link)
		if e != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("symlink changed", e)
		}
	} else {
		t.Log("symlink creation unavailable:", e)
	}
	temporary, e := filepath.Glob(filepath.Join(dir, ".silk-scene-*"))
	if e != nil || len(temporary) != 0 {
		t.Fatal("temporary file leak", temporary, e)
	}
	if e = SaveDocumentFile(ctx, filepath.Join(dir, "missing", "saved.silkscene"), d); e == nil {
		t.Fatal("created implicit directories")
	}
}
