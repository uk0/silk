package gui

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

type sceneDeviceTestDnd struct {
	source           interface{}
	data             []interface{}
	possible, action DndAction
}

func (d *sceneDeviceTestDnd) PosibleActions() DndAction { return d.possible }
func (d *sceneDeviceTestDnd) Action() DndAction         { return d.action }
func (d *sceneDeviceTestDnd) SetAction(a DndAction)     { d.action = a & d.possible }
func (d *sceneDeviceTestDnd) From() interface{}         { return d.source }
func (d *sceneDeviceTestDnd) Formats() []string         { return []string{"[]interface{}"} }
func (d *sceneDeviceTestDnd) HasFormat(s string) bool   { return s == "[]interface{}" }
func (d *sceneDeviceTestDnd) Data(s string) interface{} {
	if s == "[]interface{}" {
		return d.data
	}
	return nil
}
func deviceDragFixture(t *testing.T) (*SceneDocumentEditor, *sceneDeviceTestDnd, float64, float64) {
	t.Helper()
	e := authoringEditor(t)
	e.SetSize(960, 560)
	e.SetDeviceLibraryVisible(true)
	e.Layout()
	e.preview.SetDistance(12)
	mustAuthor(t, e.SetTranslationSnapStep(.5))
	payload, action := e.library.list.sigDragStart([]int{0})
	ctx := &sceneDeviceTestDnd{source: e.library.list, data: payload, possible: action}
	pixel, visible, err := e.preview.camera().Project(scene3d.Vec3{X: 1.23, Z: .8}, e.preview.Width(), e.preview.Height())
	mustAuthor(t, err)
	if !visible {
		t.Fatal("fixture not in view")
	}
	return e, ctx, pixel.X, pixel.Y
}
func TestSceneDeviceLibraryCatalogSearchAndActions(t *testing.T) {
	e := authoringEditor(t)
	original, _ := e.Document()
	e.nodeMenu.items[4].(*Button).Action().Trigger(nil)
	if !e.DeviceLibraryVisible() || len(e.DeviceLibrary()) != 3 || e.tree.IsVisible() || e.bindings.IsVisible() {
		t.Fatal("shelf action")
	}
	l := e.library
	for _, b := range []*Button{l.back, l.importButton, l.insert} {
		if !b.textVisible || b.Text() == "" {
			t.Fatal("library action must show its label", b.Text())
		}
		if b.Icon() == nil || b.Icon() == LoadIcon("image-missing") || b.Icon().IsAir() {
			t.Fatal("missing icon", b.Text())
		}
	}
	if l.prepareSurface.Icon() == nil || l.prepareSurface.Icon() == LoadIcon("image-missing") || l.prepareSurface.Icon().IsAir() {
		t.Fatal("missing surface status icon")
	}
	copy := e.DeviceLibrary()
	copy[0].Name = "changed"
	if e.DeviceLibrary()[0].Name == "changed" {
		t.Fatal("metadata alias")
	}
	l.search.SetText("CONVEYOR")
	if len(l.rows) != 1 || l.devices[l.rows[0]].ID != "conveyor" {
		t.Fatal("search")
	}
	oldLocale := i18n.Locale()
	i18n.SetLocale("zh")
	t.Cleanup(func() { i18n.SetLocale(oldLocale) })
	l.search.SetText("储罐")
	if len(l.rows) != 1 || l.devices[l.rows[0]].ID != "tank" {
		t.Fatal("localized search")
	}
	l.search.SetText("not-a-device")
	if l.insert.IsEnabled() || len(l.rows) != 0 {
		t.Fatal("empty search remains actionable")
	}
	l.search.SetText("")
	before := e.DeviceLibrary()
	for _, bad := range [][]scene3d.DeviceDefinition{{before[0], before[0]}, {{ID: "bad", Name: "Bad"}}, {{ID: "\x00", Name: "Bad", Template: before[0].Template}}} {
		if err := e.SetDeviceLibrary(bad); err == nil {
			t.Fatal("bad shelf accepted")
		}
		if !reflect.DeepEqual(before, e.DeviceLibrary()) {
			t.Fatal("bad shelf changed entries")
		}
	}
	// Replacing a selected tail with a shorter catalog must not read stale indices.
	l.list.SetActiveIndex(2)
	mustAuthor(t, e.SetDeviceLibrary(before[:1]))
	if len(l.rows) != 1 || l.list.ActiveIndex() != 0 {
		t.Fatal("short replacement")
	}
	after, _ := e.Document()
	if e.Modified() || !reflect.DeepEqual(original, after) {
		t.Fatal("shelf edited document")
	}
	mustAuthor(t, e.SetDeviceLibrary(nil))
	e.SetDeviceLibraryVisible(false)
	e.SetDeviceLibraryVisible(true)
	if len(e.DeviceLibrary()) != 0 {
		t.Fatal("explicit empty shelf overwritten by defaults")
	}
	l.back.Action().Trigger(nil)
	if e.DeviceLibraryVisible() || !e.tree.IsVisible() {
		t.Fatal("back action")
	}
	e.Close()
	if !errors.Is(e.SetDeviceLibrary(before), async.ErrClosed) {
		t.Fatal("closed shelf")
	}
}
func TestSceneDeviceDropAtomicHistoryBindingsAndResources(t *testing.T) {
	e, ctx, x, y := deviceDragFixture(t)
	original, _ := e.Document()
	v := e.preview
	paintScene(t, v)
	renders := v.RenderCount()
	v.OnDragEnter(x, y, ctx)
	if ctx.Action() != DndCopy || !v.deviceDropActive || v.deviceDropPosition != (scene3d.Vec3{X: 1, Z: 1}) {
		t.Fatal("ground placement", ctx.Action(), v.deviceDropPosition)
	}
	paintScene(t, v)
	paintScene(t, v)
	if v.RenderCount() != renders || e.Modified() {
		t.Fatal("drop marker rebuilt geometry or authored a preview")
	}
	v.OnDragLeave()
	if e.Modified() || v.deviceDropActive {
		t.Fatal("canceled drag mutated")
	}
	v.OnDragEnter(x, y, ctx)
	v.OnDrop(x, y, ctx)
	if ctx.Action() != DndCopy || v.deviceDropActive || e.selected != "device-1" {
		t.Fatal("drop did not commit")
	}
	first, _ := e.Document()
	root, ok := e.scene.Node("device-1")
	if !ok || root.Transform[12] != 1 || root.Transform[13] != 0 || root.Transform[14] != 1 || len(first.Bindings) != 2 || first.Bindings[0].Tag != "device-1.angle" || len(first.Warnings) != len(original.Warnings)+1 {
		t.Fatal("wrong persisted placement", root, first.Bindings, first.Warnings)
	}
	if states, _ := e.session.HistoryUsage(); states != 1 || v.bindingProgram != nil {
		t.Fatal("not one inert edit")
	}
	v.OnDrop(x, y, ctx)
	if ctx.Action() != DndIgnore || len(e.document.Nodes) != len(first.Nodes) {
		t.Fatal("old revision reused")
	}
	l := e.library
	l.prefix.SetText("line.two.")
	payload, action := l.list.sigDragStart([]int{0})
	ctx = &sceneDeviceTestDnd{source: l.list, data: payload, possible: action}
	l.prefix.SetText("changed.after.drag.")
	v.OnDrop(x+50, y, ctx)
	if ctx.Action() != DndCopy {
		t.Fatal("second drop rejected")
	}
	second, _ := e.Document()
	if len(second.Meshes) != len(first.Meshes) || second.Bindings[2].Tag != "line.two.angle" {
		t.Fatal("geometry copied or drag parameters changed")
	}
	a, _ := e.scene.Node("device-1/housing")
	b, _ := e.scene.Node("device-2/housing")
	if a.Mesh != b.Mesh {
		t.Fatal("instance mesh not shared")
	}
	mustAuthor(t, e.Undo())
	after, _ := e.Document()
	if !reflect.DeepEqual(first, after) {
		t.Fatal("undo second not exact")
	}
	mustAuthor(t, e.Undo())
	after, _ = e.Document()
	if !reflect.DeepEqual(original, after) {
		t.Fatal("undo did not restore original warnings/resources")
	}
	mustAuthor(t, e.Redo())
	mustAuthor(t, e.Redo())
	after, _ = e.Document()
	if !reflect.DeepEqual(second, after) {
		t.Fatal("redo not exact")
	}
	path := filepath.Join(t.TempDir(), "devices.silkscene")
	mustAuthor(t, scene3d.SaveDocumentFile(context.Background(), path, after))
	f, err := os.Open(path)
	mustAuthor(t, err)
	loaded, err := scene3d.LoadDocument(context.Background(), f)
	f.Close()
	mustAuthor(t, err)
	if !reflect.DeepEqual(loaded, after) {
		t.Fatal("saved device document changed")
	}
}
func TestSceneDeviceDropRejectsStaleForeignAndInvalidTargets(t *testing.T) {
	for _, mode := range []string{"foreign", "no-copy", "malformed", "outside", "nan", "behind", "hidden-shelf", "disabled", "replaced", "revision", "closed", "invalid-prefix"} {
		t.Run(mode, func(t *testing.T) {
			e, ctx, x, y := deviceDragFixture(t)
			v := e.preview
			before, _ := e.Document()
			switch mode {
			case "foreign":
				ctx.source = NewListWidget()
			case "no-copy":
				ctx.possible = DndMove
			case "malformed":
				ctx.data = []interface{}{"not a device"}
			case "outside":
				x = -1
			case "nan":
				y = math.NaN()
			case "behind":
				v.SetPitch(.1)
				y = 0
				ray, err := v.camera().Ray(x, y, v.Width(), v.Height())
				mustAuthor(t, err)
				if ray.Origin.Y <= 0 || ray.Direction.Y <= 0 {
					t.Fatal("fixture ray does not face away from ground", ray)
				}
			case "hidden-shelf":
				e.SetDeviceLibraryVisible(false)
			case "disabled":
				v.SetEnabled(false)
			case "replaced":
				mustAuthor(t, e.SetDeviceLibrary(e.DeviceLibrary()))
			case "revision":
				mustAuthor(t, e.AddNode(scene3d.DocumentNode{ID: "other"}))
				mustAuthor(t, e.Undo())
			case "closed":
				e.Close()
			case "invalid-prefix":
				p := ctx.data[0].(sceneDeviceDrag)
				p.prefix = "\x00"
				ctx.data[0] = p
			}
			v.OnDragEnter(x, y, ctx)
			v.OnDrop(x, y, ctx)
			if ctx.Action() != DndIgnore || v.deviceDropActive {
				t.Fatal("invalid drag accepted", mode)
			}
			if mode != "closed" {
				after, _ := e.Document()
				if !reflect.DeepEqual(before, after) || e.Modified() {
					t.Fatal("rejected drag changed document")
				}
			}
		})
	}
}
func TestSceneDeviceLibraryTinyLayoutAndInsertButton(t *testing.T) {
	e, _, _, _ := deviceDragFixture(t)
	l := e.library
	for _, size := range [][2]float64{{0, 0}, {1, 1}, {7, 5}, {80, 20}, {299, 40}, {300, 60}, {320, 500}, {620, 520}, {960, 560}} {
		e.SetSize(size[0], size[1])
		e.Layout()
		l.Layout()
		widgets := []IWidget{l.back, l.importButton, l.search, l.list, l.placement, l.prepareSurface, l.prefixLabel, l.prefix, l.details, l.insert}
		for i, a := range widgets {
			if a.X() < 0 || a.Y() < 0 || a.Width() < 0 || a.Height() < 0 || a.X()+a.Width() > l.Width()+1e-9 || a.Y()+a.Height() > l.Height()+1e-9 {
				t.Fatalf("outside library %v child %d", size, i)
			}
			for j := 0; j < i; j++ {
				b := widgets[j]
				if math.Min(a.X()+a.Width(), b.X()+b.Width())-math.Max(a.X(), b.X()) > 1e-9 && math.Min(a.Y()+a.Height(), b.Y()+b.Height())-math.Max(a.Y(), b.Y()) > 1e-9 {
					t.Fatalf("overlap %v %d %d", size, i, j)
				}
			}
		}
	}
	e.SetSize(960, 560)
	e.Layout()
	l.prefix.SetText("plant.pump.")
	l.insert.Action().Trigger(nil)
	if !e.Modified() || e.document.Bindings[0].Tag != "plant.pump.angle" {
		t.Fatal("insert button")
	}
	// Avoid shadowing any existing root-prefix namespace, even without its root.
	mustAuthor(t, e.AddNode(scene3d.DocumentNode{ID: "device-2/orphan"}))
	l.insert.Action().Trigger(nil)
	if _, ok := e.scene.Node("device-3"); !ok {
		t.Fatal("automatic namespace collision")
	}
}
func TestSceneDeviceImportAsyncAndRetiredCompletion(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	e := authoringEditor(t)
	e.SetDeviceLibraryVisible(true)
	original, _ := e.Document()
	before := len(e.DeviceLibrary())
	path := filepath.Join(t.TempDir(), "import.obj")
	mustAuthor(t, os.WriteFile(path, []byte(sceneOBJ), 0600))
	mustAuthor(t, e.ImportDeviceTemplate(context.Background(), path))
	generation := e.library.generation
	if !e.IsDeviceImporting() || len(e.DeviceLibrary()) != before {
		t.Fatal("load bypassed worker/UI queue")
	}
	scenePump(t, func() bool { return !e.IsDeviceImporting() })
	if e.DeviceImportError() != nil || len(e.DeviceLibrary()) != before+1 || e.Modified() {
		t.Fatal("import not shelf-only", e.DeviceImportError())
	}
	imported := e.DeviceLibrary()[before].Template
	after, _ := e.Document()
	if !reflect.DeepEqual(original, after) {
		t.Fatal("import replaced scene")
	}
	// The callback guards are deterministic even if a canceled worker already posted.
	current := e.DeviceLibrary()
	e.finishDeviceImport(generation, path, imported, nil)
	if !reflect.DeepEqual(current, e.DeviceLibrary()) {
		t.Fatal("stale generation resurrected")
	}
	mustAuthor(t, e.ImportDeviceTemplate(context.Background(), path))
	generation = e.library.generation
	e.CancelDeviceImport()
	e.finishDeviceImport(generation, path, imported, nil)
	if e.IsDeviceImporting() || !reflect.DeepEqual(current, e.DeviceLibrary()) {
		t.Fatal("canceled completion installed")
	}
	e.SetDeviceLibraryVisible(false)
	e.finishDeviceImport(generation, path, imported, nil)
	if !reflect.DeepEqual(current, e.DeviceLibrary()) {
		t.Fatal("hidden shelf resurrected")
	}
	e.Close()
	e.finishDeviceImport(generation, path, imported, nil)
	if len(e.library.devices) != 0 || e.IsDeviceImporting() {
		t.Fatal("closed import retained templates")
	}
	// Drain callbacks from the real canceled request; no new native window is opened.
	time.Sleep(30 * time.Millisecond)
	drainUITasks()
}
