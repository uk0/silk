package gui

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/scene3d"
)

func authoringEditor(t *testing.T) *SceneDocumentEditor {
	t.Helper()
	e, err := NewSceneDocumentEditor(sceneEditorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	e.preview.SetRenderBackend("cpu")
	t.Cleanup(e.Close)
	return e
}
func mustAuthor(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func TestSceneDocumentAuthoringTransactionsAndCaches(t *testing.T) {
	e := authoringEditor(t)
	original, _ := e.Document()
	target := e.preview.target
	snap, _ := e.scene.Snapshot()
	mesh := e.scene.Nodes()[1].Mesh
	n := e.document.Nodes[1]
	n.Name = "New tank"
	mustAuthor(t, e.UpdateNode(n))
	after, _ := e.scene.Snapshot()
	if after != snap || e.scene.Nodes()[1].Mesh != mesh || e.preview.target != target || e.preview.SelectedNode() != "tank" {
		t.Fatal("metadata edit lost caches/frame/selection")
	}
	mustAuthor(t, e.SetBinding(scene3d.Binding{Node: "tank", Tag: "level", Property: scene3d.PositionY, Gain: 1}))
	ids, err := e.DuplicateSubtree("assembly", scene3d.DuplicateOptions{RootID: "second", CopyBindings: true})
	mustAuthor(t, err)
	if len(e.document.Nodes) != 4 || len(e.document.Bindings) != 2 || len(e.document.Meshes) != 1 || e.selected != "second" {
		t.Fatal("copy")
	}
	mustAuthor(t, e.ReparentNode(ids["tank"], "assembly", true))
	mustAuthor(t, e.RemoveSubtree("second"))
	if len(e.document.Nodes) != 3 || e.selected != "assembly" {
		t.Fatal("remove/fallback")
	}
	for e.CanUndo() {
		mustAuthor(t, e.Undo())
	}
	restored, _ := e.Document()
	if e.Modified() || !reflect.DeepEqual(original, restored) || e.buttons[5].IsEnabled() || !e.buttons[6].IsEnabled() {
		t.Fatal("undo original/buttons")
	}
	e.buttons[6].Action().Trigger(nil)
	if !e.Modified() || !e.CanUndo() {
		t.Fatal("redo action")
	}
	mustAuthor(t, e.Undo())
	mustAuthor(t, e.AddNode(scene3d.DocumentNode{ID: "branch"}))
	if e.CanRedo() {
		t.Fatal("branch")
	}
	mustAuthor(t, e.RemoveSubtree("assembly"))
	mustAuthor(t, e.RemoveSubtree("branch"))
	if e.selected != "" || e.preview.SelectedNode() != "" || e.buttons[0].IsEnabled() || !e.buttons[4].IsEnabled() || e.nodeMenu.items[1].IsEnabled() {
		t.Fatal("empty scene actions")
	}
	e.Close()
	if !errors.Is(e.Undo(), async.ErrClosed) || !errors.Is(e.AddNode(scene3d.DocumentNode{ID: "closed"}), async.ErrClosed) {
		t.Fatal("closed mutation")
	}
	for _, b := range e.buttons {
		if b.IsEnabled() {
			t.Fatal("closed action enabled")
		}
	}
}
func TestSceneDocumentAuthoringMeshUndoAndBranchCollision(t *testing.T) {
	e := authoringEditor(t)
	id, mesh, err := e.primitive("box")
	mustAuthor(t, err)
	mustAuthor(t, e.AddMeshNode(scene3d.DocumentNode{ID: "first"}, id, mesh))
	mustAuthor(t, e.AddMeshNode(scene3d.DocumentNode{ID: "second"}, id, mesh))
	if len(e.document.Meshes) != 2 {
		t.Fatal("duplicated geometry")
	}
	mustAuthor(t, e.Undo())
	mustAuthor(t, e.Undo())
	if len(e.document.Meshes) != 1 {
		t.Fatal("resource undo")
	}
	other, err := scene3d.Cylinder(2, 2, 8)
	mustAuthor(t, err)
	mustAuthor(t, e.AddMeshNode(scene3d.DocumentNode{ID: "different"}, id, other))
	newID, newMesh, err := e.primitive("box")
	mustAuthor(t, err)
	if newID == id || newMesh != mesh {
		t.Fatal("primitive reused conflicting resource ID")
	}
	mustAuthor(t, e.AddMeshNode(scene3d.DocumentNode{ID: "box"}, newID, newMesh))
	d, _ := e.Document()
	sc, err := d.Build(context.Background())
	mustAuthor(t, err)
	n, _ := sc.Node("different")
	if n.Mesh.VertexCount() != other.VertexCount() {
		t.Fatal("GUI retained old branch geometry")
	}
}
func TestSceneDocumentAuthoringAllMenuActionsAndStaleDialogs(t *testing.T) {
	e := authoringEditor(t)
	old := showSceneDocumentDialog
	t.Cleanup(func() { showSceneDocumentDialog = old })
	if e.buttons[4].SubPopup() != e.nodeMenu || e.nodeMenu.IsVisible() || len(e.nodeMenu.items) != 5 {
		t.Fatal("popup not owned/deferred")
	}
	for _, button := range e.buttons {
		if button.Icon() == nil || button.Icon() == LoadIcon("image-missing") || button.Icon().IsAir() {
			t.Fatal("missing toolbar icon", button.Text())
		}
	}
	for _, item := range e.nodeMenu.items {
		if item.(*Button).Icon() == nil || item.(*Button).Icon() == LoadIcon("image-missing") || item.(*Button).Icon().IsAir() {
			t.Fatal("missing menu icon")
		}
	}
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		sceneEditorTestField(t, d, "scene.add.id").(*Edit).SetText("valve")
		sceneEditorTestField(t, d, "scene.add.kind").(*ComboBox).SetActiveIndex(2)
		sceneEditorTestField(t, d, "scene.add.X").(*Edit).SetText("1.25")
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("add menu not committed")
		}
		return DialogOK
	}
	e.nodeMenu.items[0].(*Button).Action().Trigger(nil)
	n, ok := e.scene.Node("valve")
	if !ok || n.Mesh == nil || n.Transform[12] != 1.25 || n.Parent != "assembly" {
		t.Fatal(n)
	}
	mustAuthor(t, e.SetBinding(scene3d.Binding{Node: "valve", Tag: "level", Property: scene3d.PositionY, Gain: 1}))
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		if sceneEditorTestField(t, d, "scene.duplicate.bindings").(*CheckBox).IsChecked() {
			t.Fatal("dangerous binding copy default")
		}
		sceneEditorTestField(t, d, "scene.duplicate.id").(*Edit).SetText("valve-copy")
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("duplicate menu")
		}
		return DialogOK
	}
	e.nodeMenu.items[1].(*Button).Action().Trigger(nil)
	if len(e.document.Bindings) != 1 || e.selected != "valve-copy" {
		t.Fatal("duplicate activated/copied bindings")
	}
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		if !sceneEditorTestField(t, d, "scene.reparent.keepWorld").(*CheckBox).IsChecked() {
			t.Fatal("world default")
		}
		sceneEditorTestField(t, d, "scene.reparent.parent").(*Edit).SetText("")
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("reparent menu")
		}
		return DialogOK
	}
	e.nodeMenu.items[2].(*Button).Action().Trigger(nil)
	n, _ = e.scene.Node("valve-copy")
	if n.Parent != "" {
		t.Fatal("parent not changed")
	}
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		if d.defaultBtn != d.buttons[0] || d.initialFocus != d.buttons[0] {
			t.Fatal("delete not cancel-safe")
		}
		d.buttons[0].Action().Trigger(nil)
		return DialogCancel
	}
	e.nodeMenu.items[3].(*Button).Action().Trigger(nil)
	if _, ok = e.scene.Node("valve-copy"); !ok {
		t.Fatal("cancel deleted")
	}
	showSceneDocumentDialog = func(d *Dialog) DialogResult { d.buttons[1].Action().Trigger(nil); return DialogOK }
	e.nodeMenu.items[3].(*Button).Action().Trigger(nil)
	if _, ok = e.scene.Node("valve-copy"); ok {
		t.Fatal("delete action")
	}
	// Every delayed authoring form must reject a changed revision, including after
	// undo back to the original document (revision is monotonic, not a dirty bit).
	for _, action := range []func(){e.addNodeDialog, e.duplicateNodeDialog, e.reparentNodeDialog, e.deleteNodeDialog} {
		e.SelectNode("valve")
		showSceneDocumentDialog = func(d *Dialog) DialogResult {
			before := len(e.document.Nodes)
			mustAuthor(t, e.AddNode(scene3d.DocumentNode{ID: "transient"}))
			mustAuthor(t, e.Undo())
			rev := e.revision
			d.buttons[1].Action().Trigger(nil)
			if d.closed || e.revision != rev || len(e.document.Nodes) != before {
				t.Fatal("stale form changed model")
			}
			d.buttons[0].Action().Trigger(nil)
			return DialogCancel
		}
		action()
	}
}
func TestSceneDocumentAuthoringTinyLayoutNeverEscapesOrOverlaps(t *testing.T) {
	e := authoringEditor(t)
	for _, size := range [][2]float64{{0, 0}, {1, 1}, {7, 5}, {80, 20}, {299, 40}, {300, 60}, {320, 500}, {620, 520}, {960, 560}} {
		e.SetSize(size[0], size[1])
		e.Layout()
		all := []IWidget{e.tree, e.bindings, e.preview, e.status}
		for _, b := range e.buttons {
			all = append(all, b)
		}
		for i, a := range all {
			if math.IsNaN(a.X()) || math.IsNaN(a.Width()) || a.X() < -1e-9 || a.Y() < -1e-9 || a.Width() < 0 || a.Height() < 0 || a.X()+a.Width() > size[0]+1e-9 || a.Y()+a.Height() > size[1]+1e-9 {
				t.Fatalf("outside %v item %d: %g,%g %gx%g", size, i, a.X(), a.Y(), a.Width(), a.Height())
			}
			for j := 0; j < i; j++ {
				b := all[j]
				if math.Min(a.X()+a.Width(), b.X()+b.Width())-math.Max(a.X(), b.X()) > 1e-9 && math.Min(a.Y()+a.Height(), b.Y()+b.Height())-math.Max(a.Y(), b.Y()) > 1e-9 {
					t.Fatalf("overlap %v %d %d", size, i, j)
				}
			}
		}
	}
}

func TestSceneDocumentFitUnavailableWithoutVisibleGeometry(t *testing.T) {
	e, err := NewSceneDocumentEditor(&scene3d.Document{Version: 1, Nodes: []scene3d.DocumentNode{{ID: "root"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.buttons[3].IsEnabled() {
		t.Fatal("Fit scene enabled on group-only scene; actual UI reports invalid geometry")
	}
	mustAuthor(t, e.AddMeshNode(scene3d.DocumentNode{ID: "box", Parent: "root"}, "box", scene3d.Box()))
	if !e.buttons[3].IsEnabled() {
		t.Fatal("visible geometry not fit-enabled")
	}
	n := e.document.Nodes[0]
	n.Hidden = true
	mustAuthor(t, e.UpdateNode(n))
	if e.buttons[3].IsEnabled() {
		t.Fatal("Fit scene enabled when every mesh is hidden")
	}
	mustAuthor(t, e.Undo())
	if !e.buttons[3].IsEnabled() {
		t.Fatal("undo did not restore Fit state")
	}
}
