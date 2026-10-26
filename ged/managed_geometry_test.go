package ged

import (
	"testing"
	"time"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func managedGeometryFixture(t *testing.T) (*GedView, *FakeWidget, *FakeWidget) {
	t.Helper()
	view := NewGedView()
	view.SetSnapToGrid(false)
	box := sceneWidget(t, view.GedScene(), "gui.VBox", "layout", 10, 10, 70, 70)
	button := sceneWidget(t, view.GedScene(), "gui.Button", "button", 20, 20, 25, 10)
	button.SetParent(box)
	box.layoutManaged = true
	return view, box, button
}

func TestManagedChildDragIsSelectionWithoutGeometryOrUndo(t *testing.T) {
	view, box, button := managedGeometryFixture(t)
	before := button.Bounds1()
	lastClickTime = time.Time{}
	x, y := view.MapFromScene(30, 25)
	view.OnLeftDown(x, y)
	if !view.managedPress {
		t.Fatal("managed child did not guard its gesture")
	}
	dx, dy := view.MapFromScene(55, 50)
	view.OnMouseMove(dx, dy)
	if !view.managedHint.active || view.managedHint.hint != managedGeometryMessage {
		t.Fatal("blocked drag has no explicit layout explanation")
	}
	view.OnLeftUp(dx, dy)
	view.OnIdle()
	if button.Bounds1() != before || button.Parent() != graph.IItem(box) || view.Scene().UndoStack().CanUndo() {
		t.Fatal("blocked drag changed position, hierarchy or undo history")
	}
	if !view.Selection().Contains(button) || view.managedPress {
		t.Fatal("blocked drag prevented normal selection or kept capture state")
	}
}

func TestManagedChildResizeHandleCannotCommitGeometry(t *testing.T) {
	view, _, button := managedGeometryFixture(t)
	view.Selection().Add(button)
	before := button.Bounds1()
	lastClickTime = time.Time{}
	x, y := view.MapFromScene(44.5, 29.5)
	view.OnLeftDown(x, y)
	if !view.managedPress {
		t.Fatal("managed resize handle did not guard its gesture")
	}
	dx, dy := view.MapFromScene(65, 50)
	view.OnMouseMove(dx, dy)
	view.OnLeftUp(dx, dy)
	if button.Bounds1() != before || view.Scene().UndoStack().CanUndo() {
		t.Fatalf("layout-owned resize mutated geometry/history: %v -> %v", before, button.Bounds1())
	}
}

func TestManagedLayoutContainerStillMovesAndResizes(t *testing.T) {
	view, box, button := managedGeometryFixture(t)
	view.Selection().Add(box)
	// Include the child too: moving a selected ancestor carries it rather
	// than treating the child as an independent free-position edit.
	view.Selection().Add(button)
	if view.selectionHasManagedGeometry() {
		t.Fatal("selected container cannot carry its managed child")
	}
	dragOnCanvas(view, 65, 65, 75, 70)
	// Screen/scene transforms may leave sub-ulp noise when snapping is off.
	requirePrecisePosition(t, box, 20, 15)
	if !view.Scene().UndoStack().CanUndo() {
		t.Fatalf("layout container itself was incorrectly locked: %v", box.Bounds1())
	}
	view.Selection().Clear()
	view.Selection().Add(box)
	before := box.Bounds1()
	lastClickTime = time.Time{}
	x, y := view.MapFromScene(before.Right()-0.5, before.Bottom()-0.5)
	view.OnLeftDown(x, y)
	if view.managedPress {
		t.Fatal("layout's own resize handle was blocked")
	}
	dx, dy := view.MapFromScene(before.Right()+10, before.Bottom()+10)
	view.OnMouseMove(dx, dy)
	view.OnLeftUp(dx, dy)
	if box.Width() <= before.Width || box.Height() <= before.Height {
		t.Fatal("layout container itself cannot resize")
	}
}

func TestManagedGeometryAlsoGuardsKeyboardAndMixedSelection(t *testing.T) {
	view, _, button := managedGeometryFixture(t)
	free := sceneWidget(t, view.GedScene(), "gui.Button", "free", 20, 85, 25, 10)
	view.Selection().AddMulti([]graph.IItem{button, free})
	before := []geom.Rect{button.Bounds1(), free.Bounds1()}
	view.nudgeSelection(5, 0)
	view.resizeSelection(5, 5)
	dragOnCanvas(view, 30, 90, 40, 95)
	if button.Bounds1() != before[0] || free.Bounds1() != before[1] || view.Scene().UndoStack().CanUndo() {
		t.Fatal("mixed selection partially moved or resized a layout-owned child")
	}
}

func TestManagedGeometryEscapeDoesNotStrandNextGesture(t *testing.T) {
	view, _, button := managedGeometryFixture(t)
	lastClickTime = time.Time{}
	x, y := view.MapFromScene(30, 25)
	view.OnLeftDown(x, y)
	view.OnMouseMove(x+20, y+20)
	view.OnKeyDown(gui.KeyEsc, false)
	if view.managedPress || view.managedHint.active {
		t.Fatal("Escape retained guarded drag state")
	}
	if view.Scene().UndoStack().CanUndo() {
		t.Fatal("Escape created history for a rejected drag")
	}
	// A plain follow-up click must still select the child.
	lastClickTime = time.Time{}
	view.OnLeftDown(x, y)
	view.OnLeftUp(x, y)
	if !view.Selection().Contains(button) {
		t.Fatal("Escape stranded the graph tool's button state")
	}
}

func TestManagedGeometryHintClearsAfterHistoryAndSelection(t *testing.T) {
	view := NewGedView()
	one := placeFromPalette(t, view, "gui.Button", 20, 20)
	view.OnIdle()
	view.showManagedGeometryHint(100, 100)
	view.Undo()
	if view.managedHint.active {
		t.Fatal("Undo retained an obsolete layout warning")
	}
	view.showManagedGeometryHint(100, 100)
	view.Redo()
	if view.managedHint.active {
		t.Fatal("Redo retained an obsolete layout warning")
	}
	view.OnIdle()
	view.showManagedGeometryHint(100, 100)
	view.Selection().Clear()
	view.Selection().Add(one)
	view.OnIdle()
	if view.managedHint.active {
		t.Fatal("changed selection retained an obsolete layout warning")
	}
}
