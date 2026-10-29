package gui

import (
	"context"
	"errors"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/scene3d"
)

func sceneEditorFixture(t *testing.T) *scene3d.Document {
	t.Helper()
	s := scene3d.NewScene()
	if err := s.Add(scene3d.Node{ID: "assembly", Name: "设备组"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(scene3d.Node{ID: "tank", Parent: "assembly", Name: "储罐", Mesh: scene3d.Box(), Transform: scene3d.RotationXYZ(scene3d.Vec3{Y: .5})}); err != nil {
		t.Fatal(err)
	}
	d, err := scene3d.CaptureDocument(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Warnings = []string{"geometry only"}
	return d
}
func TestSceneDocumentEditorTransactionsAndPrivateCopy(t *testing.T) {
	d := sceneEditorFixture(t)
	original := cloneSceneDocument(d)
	e, err := NewSceneDocumentEditor(d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.preview.SetRenderBackend("cpu")
	e.SetSize(900, 560)
	if e.Modified() || !e.SelectNode("tank") || e.SelectedNodeID() != "tank" {
		t.Fatal("selection")
	}
	if !reflect.DeepEqual(d, original) {
		t.Fatal("construction mutated source")
	}
	n := d.Nodes[1]
	n.Name = "Pump"
	n.Transform = append([]float64(nil), n.Transform...)
	n.Transform[12] = 2
	if err = e.UpdateNode(n); err != nil {
		t.Fatal(err)
	}
	saved, _ := e.Document()
	if saved.Nodes[1].Transform[12] != 2 || !e.Modified() || saved.Nodes[1].Parent != "assembly" || saved.Meshes[0].ID != d.Meshes[0].ID {
		t.Fatal(saved)
	}
	n.Transform[12] = 999
	if v, _ := e.scene.Node("tank"); v.Transform[12] != 2 {
		t.Fatal("retained caller transform")
	}
	saved.Meshes[0].Vertices[0].X = 777
	saved.Nodes[1].Transform[12] = 888
	saved.Warnings[0] = "changed"
	fresh, _ := e.Document()
	if fresh.Meshes[0].Vertices[0].X == 777 || fresh.Nodes[1].Transform[12] != 2 || fresh.Warnings[0] != "geometry only" {
		t.Fatal("export aliases")
	}
	before, _ := e.Document()
	revision := e.revision
	for _, mutate := range []func(*scene3d.DocumentNode){
		func(n *scene3d.DocumentNode) { n.Parent = n.ID },
		func(n *scene3d.DocumentNode) { n.Parent = "missing" },
		func(n *scene3d.DocumentNode) { n.Transform[0] = math.NaN() },
		func(n *scene3d.DocumentNode) { n.Color.A = 128 },
		func(n *scene3d.DocumentNode) { n.Mesh = "other" },
	} {
		bad := before.Nodes[1]
		bad.Transform = append([]float64(nil), bad.Transform...)
		mutate(&bad)
		if e.UpdateNode(bad) == nil {
			t.Fatal("invalid node accepted")
		}
		after, _ := e.Document()
		if !reflect.DeepEqual(after, before) || e.revision != revision {
			t.Fatal("partial invalid edit")
		}
	}
	cycle := before.Nodes[0]
	cycle.Parent = "tank"
	if e.UpdateNode(cycle) == nil {
		t.Fatal("cycle accepted")
	}
	spec := scene3d.Binding{Node: "tank", Tag: "tank.level", Property: scene3d.PositionY, Gain: .1}
	if err = e.SetBinding(spec); err != nil {
		t.Fatal(err)
	}
	spec.Offset = 2
	if err = e.SetBinding(spec); err != nil {
		t.Fatal(err)
	}
	if len(e.document.Bindings) != 1 {
		t.Fatal("upsert duplicated")
	}
	rev := e.revision
	if err = e.SetBinding(spec); err != nil || rev != e.revision {
		t.Fatal("unchanged binding dirty")
	}
	bad := scene3d.Binding{Node: "assembly", Tag: "alarm", Property: scene3d.AlarmColor, FalseColor: color.RGBA{A: 255}, TrueColor: color.RGBA{R: 255, A: 255}}
	if err = e.SetBinding(bad); !errors.Is(err, scene3d.ErrBinding) {
		t.Fatal("group color accepted", err)
	}
	if err = e.SetBinding(scene3d.Binding{Node: "tank", Tag: "tank.level", Property: scene3d.Visibility}); err == nil {
		t.Fatal("mixed type accepted")
	}
	if e.scene.Revision() == 0 {
		t.Fatal("no scene")
	}
	if !e.RemoveBinding("tank", scene3d.PositionY) || e.RemoveBinding("tank", scene3d.PositionY) {
		t.Fatal("remove")
	}
	if !reflect.DeepEqual(d, original) {
		t.Fatal("source mutated")
	}
	e.Close()
	if _, err = e.Document(); !errors.Is(err, async.ErrClosed) {
		t.Fatal(err)
	}
	if err = e.SetBinding(spec); !errors.Is(err, async.ErrClosed) {
		t.Fatal(err)
	}
}
func TestSceneDocumentEditorLayoutAndModalCancelCommit(t *testing.T) {
	d := sceneEditorFixture(t)
	e, err := NewSceneDocumentEditor(d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, size := range [][2]float64{{320, 500}, {620, 520}, {960, 560}, {1280, 720}} {
		e.SetSize(size[0], size[1])
		e.Layout()
		assertColumn := func() {
			want := e.tree.clientRect().Width
			if bar := e.tree.VertScrollBar(); bar.IsVisible() {
				want -= max(bar.Width(), Theme().ScrollWidth)
			}
			if got := e.tree.header.LogicSection(0).Size; got != max(8, want) {
				t.Fatalf("scene tree column leaves unused width: got %v want %v", got, want)
			}
		}
		assertColumn()
		e.rebuildTree() // applying a node edit must not reset width to 128.
		assertColumn()
		widgets := []IWidget{e.tree, e.bindings, e.preview, e.status}
		for _, b := range e.buttons {
			widgets = append(widgets, b)
			if b.Icon() == nil || !b.IsTextVisible() {
				t.Fatal("missing icon or action caption")
			}
		}
		for i, a := range widgets {
			if a.X() < 0 || a.Y() < 0 || a.X()+a.Width() > size[0]+1e-9 || a.Y()+a.Height() > size[1]+1e-9 {
				t.Fatal("outside", size, i, a.X(), a.Y(), a.Width(), a.Height())
			}
			for j, b := range widgets {
				if j <= i {
					continue
				}
				if a.Width() > 0 && a.Height() > 0 && b.Width() > 0 && b.Height() > 0 && a.X() < b.X()+b.Width() && b.X() < a.X()+a.Width() && a.Y() < b.Y()+b.Height() && b.Y() < a.Y()+a.Height() {
					t.Fatal("overlap", size, i, j)
				}
			}
		}
	}
	show := showSceneDocumentDialog
	t.Cleanup(func() { showSceneDocumentDialog = show })
	commits := 0
	showSceneDocumentDialog = func(dlg *Dialog) DialogResult {
		editor := dlg.Content().(*SceneDocumentEditor)
		copy, _ := editor.Document()
		copy.Nodes[1].Name = "Edited"
		if err := editor.UpdateNode(copy.Nodes[1]); err != nil {
			t.Fatal(err)
		}
		return DialogCancel
	}
	err = ShowSceneDocumentEditor(nil, d, func(*scene3d.Document) error { commits++; return nil })
	if err != nil || commits != 0 {
		t.Fatal(err, commits)
	}
	showSceneDocumentDialog = func(dlg *Dialog) DialogResult {
		editor := dlg.Content().(*SceneDocumentEditor)
		copy, _ := editor.Document()
		copy.Nodes[1].Name = "Edited"
		_ = editor.UpdateNode(copy.Nodes[1])
		dlg.buttons[1].Action().Trigger(nil)
		if dlg.closed {
			t.Fatal("failed commit closed dialog")
		}
		return DialogCancel
	}
	err = ShowSceneDocumentEditor(nil, d, func(*scene3d.Document) error { commits++; return errors.New("stale source") })
	if err != nil || commits != 1 {
		t.Fatal(err, commits)
	}
	showSceneDocumentDialog = func(dlg *Dialog) DialogResult {
		editor := dlg.Content().(*SceneDocumentEditor)
		copy, _ := editor.Document()
		copy.Nodes[1].Name = "Edited"
		_ = editor.UpdateNode(copy.Nodes[1])
		dlg.buttons[1].Action().Trigger(nil)
		if !dlg.closed {
			t.Fatal("successful commit stayed open")
		}
		return DialogOK
	}
	err = ShowSceneDocumentEditor(nil, d, func(next *scene3d.Document) error {
		commits++
		if next.Nodes[1].Name != "Edited" {
			t.Fatal("lost edit")
		}
		_, err := next.Build(context.Background())
		return err
	})
	if err != nil || commits != 2 {
		t.Fatal(err, commits)
	}
}
func sceneEditorTestField(t *testing.T, dlg *Dialog, id string) IWidget {
	t.Helper()
	for _, r := range dlg.Content().(*FormLayout).rows {
		if r.widget.ExtraData() == id {
			return r.widget
		}
	}
	t.Fatal(id)
	return nil
}
func TestSceneDocumentEditorNodeAndBindingForms(t *testing.T) {
	e, err := NewSceneDocumentEditor(sceneEditorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.SelectNode("tank")
	saved := showSceneDocumentDialog
	t.Cleanup(func() { showSceneDocumentDialog = saved })
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		sceneEditorTestField(t, d, "scene.node.X").(*Edit).SetText("NaN")
		d.buttons[1].Action().Trigger(nil)
		if d.closed || e.Modified() {
			t.Fatal("invalid numeric committed")
		}
		sceneEditorTestField(t, d, "scene.node.X").(*Edit).SetText("3.25")
		sceneEditorTestField(t, d, "scene.node.name").(*Edit).SetText("泵 A")
		sceneEditorTestField(t, d, "scene.node.color").(*Edit).SetText("#B040A0")
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("valid node not committed")
		}
		return DialogOK
	}
	e.editNode()
	n, _ := e.scene.Node("tank")
	if n.Transform[12] != 3.25 || n.Name != "泵 A" || n.Color != (color.RGBA{R: 176, G: 64, B: 160, A: 255}) {
		t.Fatal(n)
	}
	if n.Transform[0] != math.Cos(.5) {
		t.Fatal("lost rotation")
	}
	showSceneDocumentDialog = func(d *Dialog) DialogResult {
		sceneEditorTestField(t, d, "scene.binding.tag").(*Edit).SetText("plant.alarm")
		p := sceneEditorTestField(t, d, "scene.binding.property").(*ComboBox)
		p.SetActiveIndex(10)
		// Programmatic SetActiveIndex shares the user selection notification.
		if sceneEditorTestField(t, d, "scene.binding.gain").IsEnabled() {
			t.Fatal("bool gain still enabled")
		}
		d.buttons[1].Action().Trigger(nil)
		if !d.closed {
			t.Fatal("binding not applied")
		}
		return DialogOK
	}
	e.editBinding()
	if len(e.document.Bindings) != 1 || e.document.Bindings[0].Property != scene3d.AlarmColor {
		t.Fatal(e.document.Bindings)
	}
	e.removeSelectedBinding()
	if len(e.document.Bindings) != 0 {
		t.Fatal("remove action")
	}
}
