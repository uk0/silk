package gui

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

func TestSceneTemplateInstancesBindPickExportAndClose(t *testing.T) {
	isolateTagBridge(t)
	source := scene3d.NewScene()
	if err := source.Add(scene3d.Node{ID: "body", Mesh: scene3d.Box(), Color: color.RGBA{30, 180, 80, 255}}); err != nil {
		t.Fatal(err)
	}
	doc, err := scene3d.CaptureDocument(source, []scene3d.Binding{
		{Node: "body", Tag: "angle", Property: scene3d.RotationZ, Gain: 1},
		{Node: "body", Tag: "enabled", Property: scene3d.Visibility},
	})
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := scene3d.NewTemplate(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	s := scene3d.NewScene()
	instances, err := tmpl.InstantiateBatch(context.Background(), s, []scene3d.InstanceOptions{
		{ID: "left", Transform: scene3d.Translation(scene3d.Vec3{X: -2.5}), TagPrefix: "left."},
		{ID: "right", Transform: scene3d.Translation(scene3d.Vec3{X: 2.5}), TagPrefix: "right."},
	})
	if err != nil {
		t.Fatal(err)
	}
	v := NewSceneView()
	defer v.Close()
	v.SetRenderBackend("cpu")
	v.SetSize(320, 240)
	v.SetYaw(0)
	v.SetPitch(0)
	if err = v.SetScene(s); err != nil {
		t.Fatal(err)
	}
	if err = v.FitScene(); err != nil {
		t.Fatal(err)
	}
	var bindings []scene3d.Binding
	db := core.NewTagDB()
	for _, instance := range instances {
		bindings = append(bindings, instance.Bindings()...)
		db.GetOrCreate(instance.RootID()+".angle", core.Meta{}).SetValue(.0)
		db.GetOrCreate(instance.RootID()+".enabled", core.Meta{}).SetValue(true)
	}
	stop, err := v.SetSceneBindings(db, bindings)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	angle := db.GetOrCreate("left.angle", core.Meta{})
	for i := 0; i < 1000; i++ {
		angle.SetValue(float64(i) / 1000)
	}
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued != 1 {
		t.Fatal("instance tag flood", queued)
	}
	drainUITasks()
	left, _ := s.Node("left/body")
	right, _ := s.Node("right/body")
	if math.Abs(left.Transform[0]-math.Cos(.999)) > 1e-12 || right.Transform != scene3d.Identity() || left.Mesh != right.Mesh {
		t.Fatal("instances not independent/shared", left, right)
	}
	paintScene(t, v)
	count := v.RenderCount()
	seen := map[string]bool{}
	for y := 20; y < 220; y += 4 {
		for x := 10; x < 310; x += 4 {
			if id := v.PickNode(float64(x), float64(y)); id != "" {
				seen[strings.SplitN(id, "/", 2)[0]] = true
			}
		}
	}
	if !seen["left"] || !seen["right"] || v.RenderCount() != count {
		t.Fatal("instance pick/cache", seen, v.RenderCount(), count)
	}
	captured, err := v.CaptureSceneDocument()
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Meshes) != 1 || len(captured.Nodes) != 4 || len(captured.Bindings) != 4 {
		t.Fatal("export lost sharing/bindings")
	}
	baseline, err := captured.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n, _ := baseline.Node("left/body")
	if n.Transform != scene3d.Identity() {
		t.Fatal("export baked live rotation")
	}
	db.GetOrCreate("right.enabled", core.Meta{}).SetValue(false)
	drainUITasks()
	paintScene(t, v)
	for y := 20; y < 220; y += 8 {
		for x := 10; x < 310; x += 8 {
			if strings.HasPrefix(v.PickNode(float64(x), float64(y)), "right/") {
				t.Fatal("hidden instance still pickable")
			}
		}
	}
	// Closing the view must detach all placed subscriptions; queued updates may
	// not mutate the retained SDK scene after its UI owner goes away.
	angle.SetValue(2.0)
	v.Close()
	drainUITasks()
	n, _ = s.Node("left/body")
	if n.Transform != left.Transform {
		t.Fatal("late callback crossed closed view")
	}
}

func TestSceneDocumentNodeEditPreservesFraming(t *testing.T) {
	s := scene3d.NewScene()
	if err := s.Add(scene3d.Node{ID: "device", Name: "Device", Mesh: scene3d.Box(), Transform: scene3d.Translation(scene3d.Vec3{X: 20, Z: 30})}); err != nil {
		t.Fatal(err)
	}
	d, err := scene3d.CaptureDocument(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewSceneDocumentEditor(d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.SetSize(960, 620)
	e.preview.SetRenderBackend("cpu")
	e.preview.SetYaw(25)
	e.preview.SetPitch(20)
	e.preview.SetDistance(7)
	target, yaw, pitch, distance := e.preview.target, e.preview.Yaw(), e.preview.Pitch(), e.preview.Distance()
	if target == (scene3d.Vec3{}) {
		t.Fatal("fixture must have a nonzero observation center")
	}
	paintScene(t, e.preview)
	type sample struct {
		x, y float64
		id   string
	}
	var samples []sample
	for y := 10.0; y < e.preview.Height()-10; y += 8 {
		for x := 10.0; x < e.preview.Width()-10; x += 8 {
			samples = append(samples, sample{x, y, e.preview.PickNode(x, y)})
		}
	}
	n := d.Nodes[0]
	n.Name = "Inlet pump"
	if err = e.UpdateNode(n); err != nil {
		t.Fatal(err)
	}
	if e.preview.target != target || e.preview.Yaw() != yaw || e.preview.Pitch() != pitch || e.preview.Distance() != distance {
		t.Fatal("editing a node changed the observation camera", target, e.preview.target)
	}
	paintScene(t, e.preview)
	for _, p := range samples {
		if got := e.preview.PickNode(p.x, p.y); got != p.id {
			t.Fatal("renaming changed framing/picking", p, got)
		}
	}
	n.Transform = append([]float64(nil), n.Transform...)
	n.Transform[12] += 4
	if err = e.UpdateNode(n); err != nil {
		t.Fatal(err)
	}
	if e.preview.target != target || e.preview.Distance() != distance {
		t.Fatal("position edit auto-reframed the scene")
	}
	if err = e.preview.FitScene(); err != nil {
		t.Fatal(err)
	}
	if e.preview.target.X != target.X+4 {
		t.Fatal("explicit FitScene stopped following geometry", target, e.preview.target)
	}
}
