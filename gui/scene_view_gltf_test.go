package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

func TestSceneViewGLTFHierarchyBindingPickFitAndRecovery(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	isolateTagBridge(t)
	v := NewSceneView()
	defer v.Close()
	v.SetRenderBackend("cpu")
	v.SetSize(320, 240)
	v.SetModelSource(filepath.Join("..", "examples", "scene3d", "assembly.gltf"))
	before := v.Scene()
	loaded := 0
	v.SigModelLoaded(func() { loaded++ })
	if v.IsLoading() {
		t.Fatal("setter opened model")
	}
	if e := v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	if before != v.Scene() || loaded != 0 {
		t.Fatal("worker changed UI-owned scene")
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if loaded != 1 || v.LastError() != nil || len(v.ModelWarnings()) == 0 {
		t.Fatal(loaded, v.LastError(), v.ModelWarnings())
	}
	a, ok := v.Scene().Node("model/node/1/primitive/0")
	b, _ := v.Scene().Node("model/node/2/primitive/0")
	if !ok || a.Mesh != b.Mesh {
		t.Fatal("hierarchy/shared mesh missing")
	}
	paintScene(t, v)
	count := v.RenderCount()
	pick := ""
	for y := 20; y < 220 && pick == ""; y += 10 {
		for x := 20; x < 300 && pick == ""; x += 10 {
			pick = v.PickNode(float64(x), float64(y))
		}
	}
	if !strings.HasPrefix(pick, "model/node/") || v.RenderCount() != count {
		t.Fatal("missing primitive pick/cache", pick)
	}
	tag := core.NewTagDB().GetOrCreate("assembly.position", core.Meta{})
	stop, e := v.BindNodePosition(tag, "model", 1, .01, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	for i := 0; i < 1000; i++ {
		tag.SetValue(float64(i))
	}
	drainUITasks()
	root, _ := v.Scene().Node("model")
	if root.Transform[13] != 9.99 {
		t.Fatal(root.Transform)
	}
	node, _ := v.Scene().Node("model/node/2")
	if node.Transform[13] != -.2 {
		t.Fatal("binding destroyed authored child transform")
	}
	beforeTarget := v.target
	if e = v.FitScene(); e != nil {
		t.Fatal(e)
	}
	if v.target.Y-beforeTarget.Y < 9.98 {
		t.Fatal("FitScene ignored world hierarchy", beforeTarget, v.target)
	}
	paintScene(t, v)
	if v.LastError() != nil {
		t.Fatal(v.LastError())
	}
	old := v.Scene()
	bad := filepath.Join(t.TempDir(), "bad.gltf")
	if e = os.WriteFile(bad, []byte(`{"asset":{"version":"2.0"},"extensionsRequired":["KHR_draco_mesh_compression"]}`), 0600); e != nil {
		t.Fatal(e)
	}
	v.SetModelSource(bad)
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if v.Scene() != old || v.LastError() == nil || loaded != 1 {
		t.Fatal("failed import replaced scene")
	}
	tag.SetValue(100.0)
	drainUITasks()
	root, _ = v.Scene().Node("model")
	if root.Transform[13] != 1 {
		t.Fatal("failed import cancelled valid old binding")
	}
	v.SetModelSource(filepath.Join("..", "examples", "scene3d", "assembly.gltf"))
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	v.CancelLoad()
	time.Sleep(30 * time.Millisecond)
	drainUITasks()
	if v.Scene() != old || loaded != 1 {
		t.Fatal("cancelled generation installed")
	}
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	tag.SetValue(200.0)
	drainUITasks()
	root, _ = v.Scene().Node("model")
	if root.Transform[13] != 0 {
		t.Fatal("old binding crossed model replacement")
	}
	if loaded != 2 || v.LastError() != nil {
		t.Fatal(loaded, v.LastError())
	}
	v.Close()
	if e = v.FitScene(); e == nil {
		t.Fatal("fit after close")
	}
}
func TestSceneViewFitEmptyDoesNotChangeCamera(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	if e := v.SetScene(scene3d.NewScene()); e != nil {
		t.Fatal(e)
	}
	target, dist := v.target, v.Distance()
	if e := v.FitScene(); e == nil {
		t.Fatal("empty scene fit succeeded")
	}
	if target != v.target || dist != v.Distance() {
		t.Fatal("failed fit changed camera")
	}
}
