package gui

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/scene3d"
)

func surfaceDeviceFixture(t *testing.T) (*SceneDocumentEditor, float64, float64) {
	t.Helper()
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	s := scene3d.NewScene()
	// Top of a raised, sloped mounting platform; ground y=0 is not its surface.
	m := scene3d.Translation(scene3d.Vec3{Y: 3}).Mul(scene3d.RotationXYZ(scene3d.Vec3{Z: .25})).Mul(scene3d.Scaling(scene3d.Vec3{X: 4, Y: .25, Z: 4}))
	mustAuthor(t, s.Add(scene3d.Node{ID: "platform", Mesh: scene3d.Box(), Transform: m}))
	d, err := scene3d.CaptureDocument(s, nil)
	mustAuthor(t, err)
	e, err := NewSceneDocumentEditor(d)
	mustAuthor(t, err)
	t.Cleanup(e.Close)
	e.SetSize(1000, 620)
	e.preview.SetRenderBackend("cpu")
	e.SetDeviceLibraryVisible(true)
	e.Layout()
	mustAuthor(t, e.preview.FitScene())
	e.preview.SetYaw(.4)
	e.preview.SetPitch(.9)
	return e, e.preview.Width() / 2, e.preview.Height() / 2
}
func surfaceDeviceDrag(e *SceneDocumentEditor) *sceneDeviceTestDnd {
	data, action := e.library.list.sigDragStart([]int{0})
	return &sceneDeviceTestDnd{source: e.library.list, data: data, possible: action}
}
func awaitDeviceSurface(t *testing.T, e *SceneDocumentEditor) {
	t.Helper()
	scenePump(t, func() bool { return !e.IsDeviceSurfacePreparing() })
	if !e.DeviceSurfaceReady() {
		t.Fatal("surface not ready", e.DeviceSurfaceError())
	}
}
func nearDevicePoint(a, b scene3d.Vec3) bool { return a.Sub(b).Length() < 1e-9 }
func TestSceneDeviceSurfaceDropHistoryBindingsAndPersistence(t *testing.T) {
	for _, mode := range []SceneDevicePlacementMode{SceneDeviceSurfaceUpright, SceneDeviceSurfaceAligned} {
		t.Run(map[SceneDevicePlacementMode]string{SceneDeviceSurfaceUpright: "upright", SceneDeviceSurfaceAligned: "aligned"}[mode], func(t *testing.T) {
			e, x, y := surfaceDeviceFixture(t)
			v := e.preview
			original, _ := e.Document()
			// Drive the actual selector callback, not only the public setter.
			e.library.placement.SetActiveIndex(int(mode))
			if !e.IsDeviceSurfacePreparing() || e.library.insert.IsEnabled() || e.DevicePlacementMode() != mode {
				t.Fatal("mode did not start worker or disable insert")
			}
			awaitDeviceSurface(t, e)
			if e.Modified() || v.RenderCount() != 0 {
				t.Fatal("preparation edited or rendered scene")
			}
			dnd := surfaceDeviceDrag(e)
			paintScene(t, v)
			before := v.RenderCount()
			v.OnDragEnter(x, y, dnd)
			if dnd.Action() != DndCopy || !v.deviceDropActive || v.deviceDropNode != "platform" || v.deviceDropPosition.Y < 2 {
				t.Fatal("no surface marker", dnd.Action(), v.deviceDropPosition, v.deviceDropNode)
			}
			point, normal := v.deviceDropPosition, v.deviceDropNormal
			for i := 0; i < 25; i++ {
				v.OnDragMove(x, y, dnd)
				paintScene(t, v)
			}
			if e.Modified() || v.RenderCount() != before {
				t.Fatal("hover changed scene or rerasterized")
			}
			v.OnDrop(x, y, dnd)
			if dnd.Action() != DndCopy || v.deviceDropActive {
				t.Fatal("surface drop rejected")
			}
			root, _ := e.scene.Node("device-1")
			anchor, err := e.library.devices[0].Template.BaseAnchor()
			mustAuthor(t, err)
			if !nearDevicePoint(root.Transform.Point(anchor), point) {
				t.Fatal("device anchor floats", root.Transform.Point(anchor), point)
			}
			up := scene3d.Vec3{X: root.Transform[4], Y: root.Transform[5], Z: root.Transform[6]}
			want := scene3d.Vec3{Y: 1}
			if mode == SceneDeviceSurfaceAligned {
				want = normal
			}
			if !nearDevicePoint(up, want) {
				t.Fatal("wrong orientation", up, want)
			}
			if states, _ := e.session.HistoryUsage(); states != 1 || len(e.document.Bindings) != 2 || e.preview.bindingProgram != nil {
				t.Fatal("not one inert edit")
			}
			after, _ := e.Document()
			mustAuthor(t, e.Undo())
			back, _ := e.Document()
			if !reflect.DeepEqual(original, back) {
				t.Fatal("undo not exact")
			}
			mustAuthor(t, e.Redo())
			redo, _ := e.Document()
			if !reflect.DeepEqual(after, redo) {
				t.Fatal("redo not exact")
			}
			awaitDeviceSurface(t, e)
			path := filepath.Join(t.TempDir(), "mounted.silkscene")
			mustAuthor(t, scene3d.SaveDocumentFile(t.Context(), path, redo))
			file, err := os.Open(path)
			mustAuthor(t, err)
			read, err := scene3d.LoadDocument(t.Context(), file)
			file.Close()
			mustAuthor(t, err)
			if !reflect.DeepEqual(read, after) {
				t.Fatal("saved placement/bindings changed")
			}
			live, err := read.Build(t.Context())
			mustAuthor(t, err)
			program, err := scene3d.NewBindingSet(live, read.Bindings)
			mustAuthor(t, err)
			_, err = program.Apply("device-1.angle", .6)
			mustAuthor(t, err)
			liveRoot, _ := live.Node("device-1")
			if liveRoot.Transform != root.Transform {
				t.Fatal("binding overwrote placement")
			}
			// Reusing a stale drag must not add a second device after undo/redo.
			v.OnDrop(x, y, dnd)
			if dnd.Action() != DndIgnore {
				t.Fatal("old drag was accepted")
			}
		})
	}
}
func TestSceneDeviceSurfaceNoFallbackAndRetiredCompletion(t *testing.T) {
	e, x, y := surfaceDeviceFixture(t)
	l := e.library
	v := e.preview
	original, _ := e.Document()
	oldDrag := surfaceDeviceDrag(e)
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceSurfaceAligned))
	generation := l.surface.generation
	snapshot, _ := e.scene.Snapshot()
	// Pending work is not synchronously completed on pointer input.
	dnd := surfaceDeviceDrag(e)
	v.OnDrop(x, y, dnd)
	if dnd.Action() != DndIgnore || e.Modified() {
		t.Fatal("pending index accepted")
	}
	awaitDeviceSurface(t, e)
	index := l.surface.index
	// An empty scene query misses even though a ground intersection is possible.
	for _, p := range [][2]float64{{0, 0}, {-1, y}, {math.NaN(), y}} {
		dnd = surfaceDeviceDrag(e)
		v.OnDrop(p[0], p[1], dnd)
		if dnd.Action() != DndIgnore || e.Modified() {
			t.Fatal("invalid/missed query fell back")
		}
	}
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceGround))
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceSurfaceAligned))
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceGround))
	v.OnDrop(x, y, oldDrag)
	if dnd.Action() != DndIgnore || oldDrag.Action() != DndIgnore {
		t.Fatal("mode roundtrip revived old drag")
	}
	l.finishSurface(generation, snapshot, index, nil)
	if e.IsDeviceSurfacePreparing() || l.surface.index != nil || e.DeviceSurfaceReady() {
		t.Fatal("retired completion installed")
	}
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceSurfaceAligned))
	generation = l.surface.generation
	e.SetDeviceLibraryVisible(false)
	l.finishSurface(generation, snapshot, index, nil)
	if l.surface.index != nil || e.IsDeviceSurfacePreparing() {
		t.Fatal("hidden completion installed")
	}
	e.SetDeviceLibraryVisible(true)
	awaitDeviceSurface(t, e)
	// A direct preview mutation is detected by snapshot identity, not revision alone.
	mustAuthor(t, v.SetNodeTransform("platform", scene3d.Translation(scene3d.Vec3{Y: 5})))
	if e.DeviceSurfaceReady() {
		t.Fatal("stale snapshot ready")
	}
	dnd = surfaceDeviceDrag(e)
	v.OnDrop(x, y, dnd)
	if dnd.Action() != DndIgnore {
		t.Fatal("stale index accepted")
	}
	// Read-only operations never alter the authored document.
	after, _ := e.Document()
	if !reflect.DeepEqual(original, after) {
		t.Fatal("preparation edited document")
	}
	e.Close()
	l.finishSurface(generation, snapshot, index, nil)
	if l.surface.index != nil || e.DeviceSurfaceReady() || !errors.Is(e.PrepareDeviceSurface(t.Context()), async.ErrClosed) {
		t.Fatal("closed state retained")
	}
}
func TestSceneDeviceSurfaceRefitRetryAndInsertButton(t *testing.T) {
	e, x, y := surfaceDeviceFixture(t)
	l := e.library
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceSurfaceAligned))
	awaitDeviceSurface(t, e)
	old := l.surface.index
	rev := e.revision
	generation := l.surface.generation
	l.prepareSurface.Action().Trigger(nil)
	if l.surface.index != old || e.IsDeviceSurfacePreparing() || e.revision != rev || l.surface.generation <= generation {
		t.Fatal("ready retry rebuilt")
	}
	// A geometry edit automatically invalidates and refits, keeping mesh data shared.
	node := e.document.Nodes[0]
	node.Transform[13] += .2
	mustAuthor(t, e.UpdateNode(node))
	if !e.IsDeviceSurfacePreparing() || e.DeviceSurfaceReady() {
		t.Fatal("edit retained stale readiness")
	}
	awaitDeviceSurface(t, e)
	stats := l.surface.index.Stats()
	if stats.MeshBuilds != 0 || stats.MeshReuses != 1 || stats.RefittedNodes == 0 {
		t.Fatal("not refitted", stats)
	}
	matrix, err := e.preview.devicePlacementAt(x, y, l.devices[0].Template)
	mustAuthor(t, err)
	l.insert.Action().Trigger(nil)
	root, ok := e.scene.Node("device-1")
	if !ok || root.Transform != matrix.transform {
		t.Fatal("button used wrong placement")
	}
	awaitDeviceSurface(t, e)
	if !errors.Is(e.PrepareDeviceSurface(nil), ErrUIInvalid) || e.SetDevicePlacementMode(255) == nil {
		t.Fatal("validation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(e.PrepareDeviceSurface(ctx), context.Canceled) {
		t.Fatal("pre-cancel")
	}
}
func TestSceneDeviceSurfaceCancellationAndImportIndependence(t *testing.T) {
	e, _, _ := surfaceDeviceFixture(t)
	l := e.library
	path := filepath.Join(t.TempDir(), "device.obj")
	mustAuthor(t, os.WriteFile(path, []byte(sceneOBJ), 0600))
	mustAuthor(t, e.ImportDeviceTemplate(t.Context(), path))
	generation := l.generation
	mustAuthor(t, e.SetDevicePlacementMode(SceneDeviceSurfaceAligned))
	if l.generation != generation {
		t.Fatal("mode invalidated pending model import")
	}
	scenePump(t, func() bool { return !e.IsDeviceImporting() && !e.IsDeviceSurfacePreparing() })
	if e.DeviceImportError() != nil || len(e.DeviceLibrary()) != 4 || !e.DeviceSurfaceReady() {
		t.Fatal("concurrent import/prepare", e.DeviceImportError(), e.DeviceSurfaceError())
	}
	// Invalidate the cache, start a real build, cancel before UI delivery.
	l.retireSurface(true)
	ctx, cancel := context.WithCancel(t.Context())
	mustAuthor(t, e.PrepareDeviceSurface(ctx))
	cancel()
	scenePump(t, func() bool { return !e.IsDeviceSurfacePreparing() })
	if !errors.Is(e.DeviceSurfaceError(), context.Canceled) || e.DeviceSurfaceReady() || !l.prepareSurface.IsEnabled() {
		t.Fatal("cancel did not release/retry", e.DeviceSurfaceError())
	}
	l.prepareSurface.Action().Trigger(nil)
	awaitDeviceSurface(t, e)
	e.Close()
	if l.prepareSurface.IsEnabled() || l.insert.IsEnabled() {
		t.Fatal("closed buttons")
	}
}

func TestSceneDeviceSurfaceCompactLayoutKeepsUsableRows(t *testing.T) {
	e, _, _ := surfaceDeviceFixture(t)
	l := e.library
	for _, size := range [][2]float64{{704, 430}, {720, 460}, {960, 560}} {
		e.SetSize(size[0], size[1])
		e.Layout()
		l.Layout()
		for _, widget := range []IWidget{l.back, l.importButton, l.search, l.placement, l.prepareSurface, l.prefix, l.insert} {
			if widget.Height() < Theme().ItemHeight {
				t.Fatalf("squashed control at %v: %T height=%v shelf=%v", size, widget, widget.Height(), l.Height())
			}
		}
		if l.list.Height() < 2*Theme().ItemHeight {
			t.Fatal("device list vanished", size, l.list.Height())
		}
	}
	e.SetSize(704, 430)
	e.Layout()
	l.Layout()
	if !l.prefixLabel.IsVisible() || l.details.IsVisible() || l.prepareSurface.IsTextVisible() || l.prefixLabel.Text() == "" {
		t.Fatal("compact affordances")
	}
	// Icon-only retry still exposes full status via its tooltip/action caption.
	if l.prepareSurface.Text() == "" {
		t.Fatal("lost accessible caption")
	}
}
