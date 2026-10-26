package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
)

type propertyGeometryObject struct{ item *FakeWidget }

func (o *propertyGeometryObject) EnumProperties(l prop.IPropertyList) {
	l.AddProperty("X", o.item.X, o.item.SetX)
	l.AddProperty("Y", o.item.Y, o.item.SetY)
	l.AddProperty("Width", o.item.Width, o.item.SetWidth)
	l.AddProperty("Height", o.item.Height, o.item.SetHeight)
}
func propertyCommandFixture(t *testing.T, multi bool) (*GedView, *prop.PropertySheet, []*FakeWidget, map[string]*prop.FloatEdit) {
	t.Helper()
	v := NewGedView()
	t.Cleanup(v.Close)
	s := v.GedScene()
	s.SetSize(200, 100)
	a := addFakeAt(t, s, "a", 15.25, 20.75, 60.5, 12.125)
	items := []*FakeWidget{a}
	if multi {
		items = append(items, addFakeAt(t, s, "b", 90.5, 50.125, 20.25, 12.125), addFakeAt(t, s, "c", 140.75, 70.25, 15.125, 12.125))
	}
	objs := make([]interface{}, len(items))
	for i, it := range items {
		v.Selection().Add(it)
		objs[i] = &propertyGeometryObject{it}
	}
	v.OnIdle() // Consume selection before injecting the real, stable test binding.
	sheet := prop.NewPropertySheet()
	sheet.SetSize(400, 500)
	v.SetPropertyView(sheet)
	sheet.Bind(objs, "", s)
	fields := map[string]*prop.FloatEdit{}
	for _, child := range sheet.Children() {
		if e, ok := child.(*prop.FloatEdit); ok {
			id := strings.SplitN(gui.GetToolTip(e), "\n", 2)[0]
			fields[id] = e
		}
	}
	for _, id := range []string{"X", "Y", "Width", "Height"} {
		if fields[id] == nil {
			t.Fatalf("missing %s field: %v", id, fields)
		}
	}
	return v, sheet, items, fields
}
func TestPropertyCommandSyncMoveAndHistory(t *testing.T) {
	v, _, items, f := propertyCommandFixture(t, false)
	v.OnKeyDown(gui.KeyRight, false)
	if f["X"].Text() != "16.25" || f["Y"].Text() != "20.75" {
		t.Fatalf("nudge stale: X=%q Y=%q model=%v", f["X"].Text(), f["Y"].Text(), items[0].Bounds1())
	}
	v.Undo()
	if f["X"].Text() != "15.25" {
		t.Fatal("undo stale", f["X"].Text())
	}
	v.Redo()
	if f["X"].Text() != "16.25" {
		t.Fatal("redo stale", f["X"].Text())
	}
	v.resizeSelection(2.5, 1.25)
	if f["Width"].Text() != "63" || f["Height"].Text() != "13.375" {
		t.Fatal("resize stale")
	}
}
func TestPropertyCommandSyncMultiAlignment(t *testing.T) {
	v, sheet, _, f := propertyCommandFixture(t, true)
	sheet.SetFilter("X")
	if f["X"].Text() != "" {
		t.Fatal("mixed field not empty")
	}
	v.AlignSelection(AlignLeft)
	if f["X"].Text() != "15.25" || f["Y"].Text() != "" {
		t.Fatalf("align stale mixed state: X=%q Y=%q", f["X"].Text(), f["Y"].Text())
	}
	v.Undo()
	if f["X"].Text() != "" {
		t.Fatal("undo stale mixed state")
	}
	v.Redo()
	if f["X"].Text() != "15.25" || sheet.Filter() != "X" {
		t.Fatal("redo/filter changed")
	}
	v.SameSizeSelection(graph.SameSizeWidth)
	if f["Width"].Text() == "" {
		t.Fatal("same width remained mixed")
	}
}
func TestPropertyCommandSyncDirectNotification(t *testing.T) {
	v, _, items, f := propertyCommandFixture(t, false)
	items[0].SetX(42.125)
	v.GedScene().NotifyDesignChanged()
	if f["X"].Text() != "42.125" {
		t.Fatal("SDK notification stale", f["X"].Text())
	}
}
func TestPropertyCommandSyncPreservesTypingSession(t *testing.T) {
	v, _, items, f := propertyCommandFixture(t, false)
	e := f["X"]
	e.SelectAll()
	for _, ch := range []string{"2", ".", "7", "5"} {
		e.OnTextInput(ch)
	}
	if items[0].X() != 2.75 || e.Text() != "2.75" || v.Scene().UndoStack().Count() != 1 {
		t.Fatal("refresh interrupted typing", items[0].X(), e.Text(), v.Scene().UndoStack().Count())
	}
	v.Undo()
	if e.Text() != "15.25" {
		t.Fatal("typing undo stale")
	}
}

func TestPropertyCommandSyncVisibilityAndRetirement(t *testing.T) {
	t.Run("hidden-show", func(t *testing.T) {
		v, _, items, f := propertyCommandFixture(t, false)
		v.Hide()
		items[0].SetX(42.125)
		v.GedScene().NotifyDesignChanged()
		if f["X"].Text() != "15.25" {
			t.Fatal("hidden designer refreshed")
		}
		v.Show()
		if f["X"].Text() != "42.125" {
			t.Fatal("show did not catch up")
		}
	})
	for _, kind := range []string{"closed", "scene-replaced", "binding-replaced", "listener-rebind"} {
		t.Run(kind, func(t *testing.T) {
			v, sheet, items, f := propertyCommandFixture(t, false)
			old := v.GedScene()
			switch kind {
			case "closed":
				v.Close()
			case "scene-replaced":
				v.SetScene(NewGedScene())
			case "binding-replaced":
				sheet.Bind([]interface{}{&propertyGeometryObject{items[0]}}, "", NewGedScene())
			case "listener-rebind":
				old.AddDesignChangedCallback(func() { sheet.Bind([]interface{}{&propertyGeometryObject{items[0]}}, "", NewGedScene()) })
			}
			// A stale field must not be revived/refreshed by the retired owner.
			f["X"].SetText("stale-draft")
			items[0].SetX(99.125)
			old.NotifyDesignChanged()
			if f["X"].Text() != "stale-draft" {
				t.Fatal("retired/foreign field refreshed")
			}
		})
	}
}
func TestPropertyCommandSyncNilAndBlockedCommandsStaySilent(t *testing.T) {
	v, _, items, f := propertyCommandFixture(t, false)
	scene := v.GedScene()
	calls := 0
	scene.AddDesignChangedCallback(func() { calls++ })
	f["X"].SetText("pending-draft")
	scene.PushCommand(nil)
	id := scene.AddPendingEditor(&failingDraft{true})
	v.OnKeyDown(gui.KeyRight, false)
	scene.RemovePendingEditor(id)
	if calls != 0 || items[0].X() != 15.25 || f["X"].Text() != "pending-draft" || scene.UndoStack().Count() != 0 {
		t.Fatal("no-op/blocked command notified or discarded a draft")
	}
}
func TestPropertyCommandSyncNoIdlePolling(t *testing.T) {
	v, _, _, _ := propertyCommandFixture(t, false)
	calls := 0
	v.SetPropertyView(&propertyRefreshProbe{refresh: func() { calls++ }})
	for range 100 {
		v.OnIdle()
	}
	if calls != 0 {
		t.Fatal("idle refreshed inspector", calls)
	}
	v.OnKeyDown(gui.KeyRight, false)
	if calls != 1 {
		t.Fatal("command refresh count", calls)
	}
	for range 100 {
		v.OnIdle()
	}
	if calls != 1 {
		t.Fatal("idle kept refreshing", calls)
	}
}
