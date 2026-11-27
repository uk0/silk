package ged

import (
	"reflect"
	"testing"
	"time"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func reorderLayoutFixture(t *testing.T, kind LayoutKind) (*GedView, *FakeWidget, []*FakeWidget) {
	t.Helper()
	v := NewGedView()
	v.SetSize(900, 650)
	v.GedScene().SetSize(180, 140)
	v.SetZoomFactor(1.1)
	v.SetScrollX(8)
	v.SetScrollY(7)
	parent := sceneWidget(t, v.GedScene(), string(kind), "linear", 20, 18, 118, 90)
	var widgets []*FakeWidget
	for _, id := range []string{"one", "two", "three"} {
		w := sceneWidget(t, v.GedScene(), "gui.Button", id, 25, 20, 20, 10)
		w.SetParent(parent)
		widgets = append(widgets, w)
	}
	parent.layoutManaged = true
	parent.Layout()
	lastClickTime = time.Time{}
	return v, parent, widgets
}

func layoutBodyPoint(v *GedView, w *FakeWidget, kind LayoutKind, t float64) (float64, float64) {
	r := w.MapRectToScene1(w.Bounds1())
	if kind == LayoutHorizontal {
		return v.MapFromScene(r.X+r.Width*t, r.Y+r.Height/2)
	}
	return v.MapFromScene(r.X+r.Width/2, r.Y+r.Height*t)
}

func TestManagedLinearChildReorderUsesReleaseSlotAndOneUndo(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical} {
		t.Run(string(kind), func(t *testing.T) {
			v, parent, items := reorderLayoutFixture(t, kind)
			scene := v.GedScene()
			attach := 0
			changes := 0
			scene.SigItemAttached(func(_ interface{}, _, _ graph.IItem) { attach++ })
			scene.SigItemDetached(func(_ interface{}, _, _ graph.IItem) { attach++ })
			scene.SigDesignChanged(func() { changes++ })
			v.Selection().Add(items[0])
			sx, sy := layoutBodyPoint(v, items[0], kind, .5)
			v.OnLeftDown(sx, sy)
			if !v.managedPress || v.layoutReorder.item != items[0] {
				t.Fatal("did not arm single body reorder")
			}
			x, y := layoutBodyPoint(v, items[2], kind, .75)
			v.OnMouseMove(x, y)
			if !v.managedHint.insertion.valid || v.managedHint.insertion.index != 2 || v.managedHint.insertion.ordinal != 3 {
				t.Fatalf("wrong live slot: %+v", v.managedHint)
			}
			if changes != 0 || scene.UndoStack().CanUndo() {
				t.Fatal("hover mutated design")
			}
			x, y = layoutBodyPoint(v, items[2], kind, .25)
			v.OnLeftUp(x, y)
			before := []graph.IItem{items[0], items[1], items[2]}
			after := []graph.IItem{items[1], items[0], items[2]}
			if !reflect.DeepEqual(parent.Children(), after) || v.managedPress || v.managedHint.active || changes != 1 || attach != 0 {
				t.Fatalf("release slot/notifications mismatch: got=%v changes=%d attach=%d", parent.Children(), changes, attach)
			}
			if !reflect.DeepEqual(v.Selection().ItemList(), []graph.IItem{items[0]}) {
				t.Fatal("changed identity of selected control")
			}
			v.Undo()
			if !reflect.DeepEqual(parent.Children(), before) || scene.UndoStack().CanUndo() || attach != 0 {
				t.Fatal("not a single undo, or detach altered the tree")
			}
			v.Redo()
			if !reflect.DeepEqual(parent.Children(), after) || attach != 0 {
				t.Fatal("redo used wrong slot")
			}
			doc := scene.SaveDesign()
			loaded := NewGedScene()
			if err := loaded.LoadDesign(doc); err != nil {
				t.Fatal(err)
			}
			for _, node := range []*decl.Node{buildSceneDeclNode(loaded), mustDeclFromDesign(t, doc)} {
				_, idx, err := node.BuildWithIndex()
				if err != nil {
					t.Fatal(err)
				}
				one, two, three := idx["one"].(gui.IWidget), idx["two"].(gui.IWidget), idx["three"].(gui.IWidget)
				if !reflect.DeepEqual(one.Parent().Children(), []gui.IWidget{two, one, three}) {
					t.Fatal("generated order differs from canvas")
				}
			}
		})
	}
}

func TestManagedLinearChildReorderRejectsNoopLockExitAndCancellation(t *testing.T) {
	for _, scenario := range []string{"no-motion", "same-slot", "move-locked", "cross-locked", "parent-locked", "outside-form", "escape", "pointer-cancel", "mixed-selection", "handle"} {
		t.Run(scenario, func(t *testing.T) {
			v, parent, items := reorderLayoutFixture(t, LayoutVertical)
			before := parent.Children()
			v.Selection().Add(items[0])
			switch scenario {
			case "move-locked":
				items[0].SetLockPos(true)
			case "cross-locked":
				items[1].SetLockPos(true)
			case "parent-locked":
				parent.SetLockPos(true)
			case "mixed-selection":
				v.Selection().Add(items[1])
			}
			x, y := layoutBodyPoint(v, items[0], LayoutVertical, .5)
			if scenario == "handle" {
				r := items[0].MapRectToScene1(items[0].Bounds1())
				x, y = v.MapFromScene(r.Right()-.2, r.Bottom()-.2)
			}
			v.OnLeftDown(x, y)
			gx, gy := layoutBodyPoint(v, items[2], LayoutVertical, .8)
			if scenario == "same-slot" {
				gx, gy = layoutBodyPoint(v, items[0], LayoutVertical, .25)
			}
			if scenario == "outside-form" {
				gx, gy = v.MapFromScene(v.Scene().Width()+5, 130)
			}
			if scenario != "no-motion" {
				v.OnMouseMove(gx, gy)
			}
			if scenario == "escape" || scenario == "pointer-cancel" {
				if !v.managedHint.insertion.valid {
					t.Fatal("cancellation case did not preview an actual move")
				}
				if scenario == "escape" {
					v.OnKeyDown(gui.KeyEsc, false)
				} else {
					v.OnPointerCancel()
				}
			}
			v.OnLeftUp(gx, gy)
			if !reflect.DeepEqual(parent.Children(), before) || v.Scene().UndoStack().CanUndo() || v.managedPress || v.managedHint.insertion.valid {
				t.Fatalf("invalid/cancelled reorder mutated or retained gesture: %+v", v.managedHint)
			}
		})
	}
}
