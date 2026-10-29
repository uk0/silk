package gui

import (
	"image/color"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

func TestSceneViewFrustumTagReentryPickUploadAndClose(t *testing.T) {
	isolateTagBridge(t)
	scene := scene3d.NewScene()
	box := scene3d.Box()
	for _, n := range []scene3d.Node{
		{ID: "culled-first", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{X: -1e4})},
		{ID: "moving-second", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{X: 1e4}), Color: color.RGBA{R: 230, G: 20, B: 30, A: 255}},
		{ID: "stationary-third", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{X: -1.4}), Color: color.RGBA{R: 20, G: 220, B: 30, A: 255}},
	} {
		if err := scene.Add(n); err != nil {
			t.Fatal(err)
		}
	}
	old, err := scene.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	view := NewSceneView()
	defer view.Close()
	view.SetRenderBackend("cpu")
	view.SetSize(160, 120)
	view.SetYaw(0)
	view.SetPitch(0)
	view.SetDistance(6)
	if err = view.SetScene(scene); err != nil {
		t.Fatal(err)
	}
	db := core.NewTagDB()
	position := db.GetOrCreate("position", core.Meta{})
	position.SetValue(1e4)
	enabled := db.GetOrCreate("enabled", core.Meta{})
	enabled.SetValue(true)
	stop, err := view.SetSceneBindings(db, []scene3d.Binding{{Node: "moving-second", Tag: "position", Property: scene3d.PositionX, Gain: 1}, {Node: "moving-second", Tag: "enabled", Property: scene3d.Visibility}})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	drainUITasks()
	beforePM := paintScene(t, view)
	before, err := beforePM.Image()
	if err != nil {
		t.Fatal(err)
	}
	beforePixel := color.RGBAModel.Convert(before.At(80, 60)).(color.RGBA)
	if view.PickNode(80, 60) != "" {
		t.Fatal("offscreen instance picked")
	}
	sequence := view.RenderCount()
	paintScene(t, view)
	if view.RenderCount() != sequence {
		t.Fatal("idle culling rerendered")
	}
	for i := 0; i < 1000; i++ {
		position.SetValue(float64(999-i) * 10)
	}
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued != 1 {
		t.Fatal("reentry flood did not coalesce", queued)
	}
	drainUITasks()
	// A pick may trigger the new raster before Draw: the displayed pixmap must
	// still receive the new sequence even when the next Render is cached.
	if id := view.PickNode(80, 60); id != "moving-second" {
		t.Fatal("culled instance failed to reenter with stable ID", id)
	}
	afterPM := paintScene(t, view)
	after, err := afterPM.Image()
	if err != nil {
		t.Fatal(err)
	}
	pixel := color.RGBAModel.Convert(after.At(80, 60)).(color.RGBA)
	if pixel == beforePixel || pixel.R <= pixel.G || pixel.R <= pixel.B {
		t.Fatal("pick refreshed but pixmap stayed stale", beforePixel, pixel)
	}
	if previous, _ := old.Instance(1); previous.Transform[12] != 1e4 {
		t.Fatal("old immutable snapshot mutated")
	}
	count := view.RenderCount()
	paintScene(t, view)
	if view.RenderCount() != count {
		t.Fatal("static reentered model rerasterized")
	}
	enabled.SetValue(false)
	drainUITasks()
	paintScene(t, view)
	if view.PickNode(80, 60) != "" {
		t.Fatal("hidden instance remained pickable")
	}
	position.SetValue(123)
	view.Close()
	drainUITasks()
	n, _ := scene.Node("moving-second")
	if n.Transform[12] != 0 {
		t.Fatal("pending point crossed closed view")
	}
}
