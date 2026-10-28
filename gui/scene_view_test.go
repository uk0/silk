package gui

import (
	"context"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
)

func paintScene(t *testing.T, v *SceneView) paint.Pixmap {
	t.Helper()
	pm := paint.NewPixmap(int(v.Width()), int(v.Height()))
	g := pm.NewPainter()
	v.Draw(g)
	paint.ClosePainter(g)
	t.Cleanup(func() { paint.CloseSurface(pm) })
	return pm
}
func TestSceneViewFactoryDrawCacheAndPickRefresh(t *testing.T) {
	v := core.New("gui.SceneView").(*SceneView)
	defer v.Close()
	v.SetSize(240, 180)
	v.SetYaw(0)
	v.SetPitch(0)
	pm := paintScene(t, v)
	first, _ := pm.Image()
	before := color.RGBAModel.Convert(first.At(120, 90)).(color.RGBA)
	if before.B <= 41 {
		t.Fatal("model not painted", before)
	}
	renders := v.RenderCount()
	paintScene(t, v)
	if v.RenderCount() != renders {
		t.Fatal("static scene rerasterized")
	}
	if v.PickNode(120, 90) != "model" {
		t.Fatal("missing center pick")
	}
	if err := v.SetNodeColor("model", color.RGBA{240, 10, 10, 255}); err != nil {
		t.Fatal(err)
	}
	// A pick can render before Draw. Pixmap upload must follow frame sequence,
	// not just Frame.Stats.Cached, otherwise the old model color persists.
	if v.PickNode(120, 90) != "model" {
		t.Fatal("pick after color change")
	}
	afterPM := paintScene(t, v)
	after, _ := afterPM.Image()
	c := color.RGBAModel.Convert(after.At(120, 90)).(color.RGBA)
	if c.R <= c.B || c == before {
		t.Fatal("stale upload after picking", c, before)
	}
	_ = v.SetNodeVisible("model", false)
	if v.PickNode(120, 90) != "" {
		t.Fatal("invisible node pick")
	}
	v.SetYaw(math.NaN())
	v.SetPitch(math.Inf(1))
	if !finiteSceneValue(v.Yaw()) || !finiteSceneValue(v.Pitch()) {
		t.Fatal("nonfinite camera")
	}
}
func TestSceneViewTagBurstQualityAndLifecycle(t *testing.T) {
	isolateTagBridge(t)
	v := NewSceneView()
	defer v.Close()
	tag := core.NewTagDB().GetOrCreate("tank.level", core.Meta{})
	stop, err := v.BindNodePosition(tag, "model", 1, .5, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		tag.SetValue(float64(i))
	}
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued != 1 {
		t.Fatal("tag flood", queued)
	}
	drainUITasks()
	n, _ := v.Scene().Node("model")
	if n.Transform[13] != 5001.5 {
		t.Fatal(n.Transform)
	}
	tag.Publish(core.Value{Raw: -100.0, Quality: core.QualityBad})
	drainUITasks()
	n, _ = v.Scene().Node("model")
	if n.Transform[13] != 5001.5 {
		t.Fatal("bad quality moved model")
	}
	tag.SetValue("invalid")
	drainUITasks()
	n, _ = v.Scene().Node("model")
	if n.Transform[13] != 5001.5 {
		t.Fatal("string became numeric zero")
	}
	tag.SetValue(9.0)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); stop(); stop() }()
	wg.Wait()
	drainUITasks()
	n, _ = v.Scene().Node("model")
	if n.Transform[13] != 5001.5 {
		t.Fatal("cancelled binding escaped")
	}
	_, err = v.BindNodeVisibility(tag, "model")
	if err != nil {
		t.Fatal(err)
	}
	tag.SetValue(false)
	v.Close()
	drainUITasks()
	if len(v.bindings) != 0 || v.scene != nil {
		t.Fatal("close retained binding state")
	}
}

const sceneOBJ = "v -1 -1 0\nv 1 -1 0\nv 0 1 0\nf 1 2 3\n"

func scenePump(t *testing.T, done func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		drainUITasks()
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("3D completion timeout")
}
func TestSceneViewLoadExplicitAsyncAndCancel(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	v := NewSceneView()
	defer v.Close()
	path := filepath.Join(t.TempDir(), "model.obj")
	if err := os.WriteFile(path, []byte(sceneOBJ), 0600); err != nil {
		t.Fatal(err)
	}
	v.SetModelSource(path)
	original := v.Scene()
	if v.IsLoading() {
		t.Fatal("property setter performed I/O")
	}
	loaded := 0
	v.SigModelLoaded(func() { loaded++ })
	if err := v.LoadModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v.Scene() != original || loaded != 0 {
		t.Fatal("background work bypassed UI queue")
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	node, _ := v.Scene().Node("model")
	if loaded != 1 || v.LastError() != nil || node.Mesh.TriangleCount() != 1 {
		t.Fatal("model not installed", loaded, v.LastError())
	}
	before := v.Scene()
	if err := v.LoadModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	v.CancelLoad()
	time.Sleep(30 * time.Millisecond)
	drainUITasks()
	if v.Scene() != before || loaded != 1 {
		t.Fatal("stale load installed")
	}
	v.SetModelSource(filepath.Join(t.TempDir(), "missing.obj"))
	if err := v.LoadModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if v.LastError() == nil || v.Scene() != before {
		t.Fatal("failure lost scene")
	}
}
func TestSceneViewSnapshotIsSafeForBackgroundRenderer(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	snap, e := v.Scene().Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var r scene3d.Renderer
		for i := 0; i < 20; i++ {
			_, err := r.Render(snap, scene3d.DefaultCamera(), 80, 60, color.RGBA{})
			if err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 20; i++ {
		_ = v.SetNodeTransform("model", scene3d.Translation(scene3d.Vec3{X: float64(i)}))
	}
	wg.Wait()
}

func TestSceneViewHiDPIPickAndClip(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 2} {
		v := NewSceneView()
		v.SetSize(120, 90)
		v.SetYaw(0)
		v.SetPitch(0)
		pm := paint.NewPixmap(300, 240)
		g := pm.NewPainter()
		g.SetBrush1(paint.Color{R: 219, G: 13, B: 201, A: 255})
		g.Paint()
		g.Scale(scale, scale)
		g.Translate(10, 10)
		v.Draw(g)
		if v.pixmap.Width() != int(math.Ceil(120*scale)) || v.pixmap.Height() != int(math.Ceil(90*scale)) {
			t.Fatal("not physical resolution", scale)
		}
		before := v.RenderCount()
		if got := v.PickNode(60, 45); got != "model" {
			t.Fatal("HiDPI center pick", scale, got)
		}
		v.Draw(g)
		if v.RenderCount() != before {
			t.Fatal("picking discarded unchanged physical framebuffer", scale)
		}
		im, err := pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		for _, pt := range [][2]int{{0, 0}, {299, 239}, {299, 40}, {40, 239}} {
			if got := color.RGBAModel.Convert(im.At(pt[0], pt[1])).(color.RGBA); got != (color.RGBA{219, 13, 201, 255}) {
				t.Fatal("scene escaped viewport", scale, pt, got)
			}
		}
		// Draw must restore its clip: a sibling may paint beyond this viewport.
		g.SetBrush1(paint.Color{R: 12, G: 244, B: 23, A: 255})
		g.Rectangle(135, 0, 4, 4)
		g.Fill()
		im, err = pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		if got := color.RGBAModel.Convert(im.At(int(147*scale), int(12*scale))).(color.RGBA); got != (color.RGBA{12, 244, 23, 255}) {
			t.Fatal("clip leaked to sibling", scale, got)
		}
		paint.ClosePainter(g)
		paint.CloseSurface(pm)
		v.Close()
	}
}

func TestSceneViewWheelBudgetAndRenderErrorRecovery(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	for _, z := range []float64{-1e300, -120, -1, -.1, 0, .1, 1, 120, 1e300} {
		v.SetDistance(6)
		v.OnMouseWheel(0, 0, z)
		if got := v.Distance(); got < 6*math.Exp(-.1)-1e-12 || got > 6*math.Exp(.1)+1e-12 {
			t.Fatal("single scroll event lost model", z, got)
		}
	}
	v.SetDistance(6)
	v.OnMouseWheel(0, 0, .25)
	if math.Abs(v.Distance()-6*math.Exp(-.025)) > 1e-12 {
		t.Fatal("fractional scroll lost precision")
	}
	v.SetSize(scene3d.MaxDimension+1, 1)
	pm := paint.NewPixmap(120, 90)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	defer paint.ClosePainter(g)
	v.Draw(g)
	if v.LastError() != scene3d.ErrLimit {
		t.Fatal("oversize viewport accepted", v.LastError())
	}
	v.SetSize(120, 90)
	v.Draw(g)
	if v.LastError() != nil {
		t.Fatal("recovered viewport retained stale render error", v.LastError())
	}
	v.Close()
	v.Draw(g)
	if v.PickNode(60, 45) != "" {
		t.Fatal("closed view picked model")
	}
}

func TestSceneViewBackendSelectionIsExplicit(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	v.SetSize(120, 90)
	if v.RenderBackend() != "auto" {
		t.Fatal("default backend")
	}
	paintScene(t, v)
	if v.ActiveRenderBackend() != "cpu" || v.GPUError() != nil {
		t.Fatal("offscreen tool attempted native context")
	}
	count := v.RenderCount()
	v.SetRenderBackend("cpu")
	paintScene(t, v)
	if v.RenderCount() != count {
		t.Fatal("backend switch fabricated a raster count")
	}
	v.SetRenderBackend("opengl")
	paintScene(t, v)
	if v.LastError() == nil {
		t.Fatal("explicit GPU silently fell back offscreen")
	}
	v.SetRenderBackend("invalid")
	if v.RenderBackend() != "opengl" {
		t.Fatal("invalid backend accepted")
	}
	v.SetRenderBackend("auto")
	paintScene(t, v)
	if v.LastError() != nil {
		t.Fatal("backend error did not recover", v.LastError())
	}
}
