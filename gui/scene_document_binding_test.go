package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

func TestSceneDocumentLoadBindSaveReloadAndFailureRecovery(t *testing.T) {
	isolateTagBridge(t)
	s := scene3d.NewScene()
	base := scene3d.Translation(scene3d.Vec3{X: .25})
	if e := s.Add(scene3d.Node{ID: "device", Mesh: scene3d.Box(), Transform: base}); e != nil {
		t.Fatal(e)
	}
	specs := []scene3d.Binding{
		{Node: "device", Tag: "height", Property: scene3d.PositionY, Gain: .001},
		{Node: "device", Tag: "angle", Property: scene3d.RotationZ, Gain: 1},
		{Node: "device", Tag: "alarm", Property: scene3d.AlarmColor, FalseColor: color.RGBA{20, 180, 80, 255}, TrueColor: color.RGBA{240, 40, 30, 255}},
	}
	doc, e := scene3d.CaptureDocument(s, specs)
	if e != nil {
		t.Fatal(e)
	}
	doc.Warnings = []string{"opaque geometry preview"}
	var raw bytes.Buffer
	if e = scene3d.SaveDocument(context.Background(), &raw, doc); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "设备.silkscene")
	if e = os.WriteFile(path, raw.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	db := core.NewTagDB()
	height := db.GetOrCreate("height", core.Meta{})
	angle := db.GetOrCreate("angle", core.Meta{})
	alarm := db.GetOrCreate("alarm", core.Meta{})
	height.SetValue(.0)
	angle.SetValue(.0)
	alarm.SetValue(false)
	v := NewSceneView()
	defer v.Close()
	v.SetRenderBackend("cpu")
	v.SetSize(240, 180)
	v.SetModelSource(path)
	old := v.Scene()
	if len(v.SceneBindings()) != 0 || v.IsLoading() {
		t.Fatal("source setter performed I/O")
	}
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	if v.Scene() != old {
		t.Fatal("background load mutated UI scene")
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if v.LastError() != nil || len(v.SceneBindings()) != 3 {
		t.Fatal(v.LastError(), v.SceneBindings())
	}
	// Saved bindings are inert until the caller explicitly chooses a registry.
	height.SetValue(500.0)
	drainUITasks()
	n, _ := v.Scene().Node("device")
	if n.Transform[13] != 0 {
		t.Fatal("implicit subscription")
	}
	stop, e := v.SetSceneBindings(db, v.SceneBindings())
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	for i := 0; i < 1000; i++ {
		height.SetValue(float64(i))
		angle.SetValue(float64(i) * .001)
	}
	alarm.SetValue(true)
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued != 1 {
		t.Fatal("per-tag UI flood", queued)
	}
	drainUITasks()
	n, _ = v.Scene().Node("device")
	want := base.Mul(scene3d.RotationXYZ(scene3d.Vec3{Z: .999}))
	want[13] = .999
	if n.Transform != want || n.Color != specs[2].TrueColor {
		t.Fatal("saved channels did not compose", n)
	}
	paintScene(t, v)
	renders := v.RenderCount()
	paintScene(t, v)
	if v.RenderCount() != renders {
		t.Fatal("idle render")
	}
	height.Publish(core.Value{Raw: 99999.0, Quality: core.QualityBad})
	angle.SetValue("wrong")
	drainUITasks()
	n, _ = v.Scene().Node("device")
	if n.Transform != want {
		t.Fatal("bad sample changed scene")
	}
	// Switching registries/configuration in place must not compound live transforms.
	angle.SetValue(.999)
	height.SetValue(999.0)
	if _, e = v.SetSceneBindings(db, v.SceneBindings()); e != nil {
		t.Fatal(e)
	}
	drainUITasks()
	n, _ = v.Scene().Node("device")
	if n.Transform != want {
		t.Fatal("in-place binding replacement compounded transforms", n.Transform, want)
	}
	captured, e := v.CaptureSceneDocument()
	if e != nil {
		t.Fatal(e)
	}
	restored, e := captured.Build(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	authored, _ := restored.Node("device")
	if authored.Transform != base || len(captured.Warnings) != 1 {
		t.Fatal("runtime state became authored geometry")
	}
	raw.Reset()
	if e = scene3d.SaveDocument(context.Background(), &raw, captured); e != nil {
		t.Fatal(e)
	}
	saved := filepath.Join(t.TempDir(), "saved.silkscene")
	if e = os.WriteFile(saved, raw.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	// Invalid registry replacement and failed file loads preserve the old session.
	if _, e = v.SetSceneBindings(core.NewTagDB(), specs); !errors.Is(e, scene3d.ErrBinding) {
		t.Fatal(e)
	}
	bad := filepath.Join(t.TempDir(), "bad.silkscene")
	_ = os.WriteFile(bad, []byte(`{"version":99}`), 0600)
	old = v.Scene()
	v.SetModelSource(bad)
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if v.Scene() != old || v.LastError() == nil {
		t.Fatal("failed document replaced live scene")
	}
	height.SetValue(200.0)
	drainUITasks()
	n, _ = v.Scene().Node("device")
	if n.Transform[13] != .2 {
		t.Fatal("failed replacement cancelled old session")
	}
	// Successful reload cancels old subscriptions and retains inert metadata.
	v.SetModelSource(saved)
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	height.SetValue(800.0)
	drainUITasks()
	n, _ = v.Scene().Node("device")
	if n.Transform != base {
		t.Fatal("old callbacks crossed document replacement")
	}
	if _, e = v.SetSceneBindings(db, v.SceneBindings()); e != nil {
		t.Fatal(e)
	}
	angle.SetValue(.999)
	drainUITasks()
	n, _ = v.Scene().Node("device")
	want[13] = .8
	if n.Transform != want {
		t.Fatal("rebind compounded rotation", n.Transform, want)
	}
	// Final explicitly empty documents are legitimate editable scenes.
	empty := filepath.Join(t.TempDir(), "empty.silkscene")
	_ = os.WriteFile(empty, []byte(`{"version":1}`), 0600)
	v.SetModelSource(empty)
	if e = v.LoadModel(context.Background()); e != nil {
		t.Fatal(e)
	}
	scenePump(t, func() bool { return !v.IsLoading() })
	if len(v.Scene().Nodes()) != 0 || v.LastError() != nil || len(v.SceneBindings()) != 0 {
		t.Fatal("empty document rejected", v.LastError())
	}
}

func TestSceneBindingsWideBurstYieldsToInputAndCancellation(t *testing.T) {
	isolateTagBridge(t)
	v := NewSceneView()
	defer v.Close()
	s := scene3d.NewScene()
	db := core.NewTagDB()
	specs := make([]scene3d.Binding, 1000)
	tags := make([]*core.Tag, len(specs))
	for i := range specs {
		id := fmt.Sprintf("node/%04d", i)
		if e := s.Add(scene3d.Node{ID: id}); e != nil {
			t.Fatal(e)
		}
		tags[i] = db.GetOrCreate(id, core.Meta{})
		tags[i].SetValue(float64(i + 1))
		specs[i] = scene3d.Binding{Node: id, Tag: id, Property: scene3d.PositionX, Gain: 1}
	}
	if e := v.SetScene(s); e != nil {
		t.Fatal(e)
	}
	before := s.Revision()
	stop, e := v.SetSceneBindings(db, specs)
	if e != nil {
		t.Fatal(e)
	}
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued != 1 {
		t.Fatal("seed storm", queued)
	}
	var atInput uint64
	Post(func() { atInput = s.Revision() - before })
	drainUITasks()
	if atInput != 256 {
		t.Fatal("wide batch starved UI", atInput)
	}
	for i := 0; i < 4; i++ {
		drainUITasks()
	}
	if s.Revision()-before != 1000 {
		t.Fatal("lost final values", s.Revision()-before)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			tags[i%len(tags)].SetValue(float64(i + 1))
		}
	}()
	stop()
	stop()
	wg.Wait()
	before = s.Revision()
	for i := 0; i < 6; i++ {
		drainUITasks()
	}
	if s.Revision() != before {
		t.Fatal("cancelled queued sample applied")
	}
	if _, e = v.SetSceneBindings(db, specs); e != nil {
		t.Fatal(e)
	}
	v.Close()
	before = s.Revision()
	drainUITasks()
	if s.Revision() != before {
		t.Fatal("closed view received primed values")
	}
}
