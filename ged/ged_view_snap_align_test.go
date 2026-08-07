package ged

import (
	"testing"
)

// TestDragOffGridCarriesContainerChildrenThroughSnap is the drag the merged
// move-command fix does not finish on its own. The move command carries the
// subtree, but the grid snap that runs straight after it corrects the container
// alone — so a drag by anything that is not a whole number of grid cells leaves
// every child behind by the rounding.
//
// The delta is +13/+7 against a 5mm pitch on purpose: the container lands at
// (23,17), the snap pulls it to (25,15), and that (+2,-2) correction is exactly
// what the children have to receive as well.
func TestDragOffGridCarriesContainerChildrenThroughSnap(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	view.SetGridStep(5)
	view.SetSnapToGrid(true)
	box, inner, btn := nestedContainer(t, view)

	dragOnCanvas(view, 15, 15, 28, 22) // +13, +7 — neither is a multiple of 5

	if x, y := box.Pos(); x != 25 || y != 15 {
		t.Fatalf("container after the drag = (%v,%v), want (25,15) — the drag or the snap did not land", x, y)
	}
	// Offsets inside the container went in as (10,20) and (15,25); they have to
	// come out the same, or the widgets are sitting outside the box that owns
	// them.
	if x, y := inner.Pos(); x != 35 || y != 35 {
		t.Errorf("inner container = (%v,%v), want (35,35) — the snap moved the box without it", x, y)
	}
	if x, y := btn.Pos(); x != 40 || y != 40 {
		t.Errorf("button = (%v,%v), want (40,40) — the snap moved the box without it", x, y)
	}
}

// TestSnapSelectionToGridDoesNotSnapChildTwice: with a container and one of its
// own children both selected, the container's correction already moves the
// child. Snapping the child on its own account as well shifts it a second time
// and pushes it out of the container — the double-move every command generator
// in graph.Selection guards against.
func TestSnapSelectionToGridDoesNotSnapChildTwice(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)
	view.SetGridStep(5)
	view.SetSnapToGrid(true)

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetBounds(13, 13, 80, 60)
	box.SetParent(scene)

	btn := addFakeAt(t, scene, "btn", 16, 16, 25, 7)
	btn.SetParent(box)

	view.Selection().Clear()
	view.Selection().Add(box)
	view.Selection().Add(btn)

	view.snapSelectionToGrid()

	if x, y := box.Pos(); x != 15 || y != 15 {
		t.Fatalf("container after snap = (%v,%v), want (15,15)", x, y)
	}
	// (16,16) rode the container's (+2,+2) to (18,18). Snapping it again would
	// round that to (20,20) and cost it 2mm of its 3mm offset.
	if x, y := btn.Pos(); x != 18 || y != 18 {
		t.Errorf("child after snap = (%v,%v), want (18,18) — it was moved twice", x, y)
	}
}

// alignedContainer builds a container with one child plus two loose widgets, the
// tree a multi-item align has to keep intact: a VBox at (20,30,40,30) holding a
// Button at (25,35,20,7), and Buttons at X 0 and X 100 to anchor the alignment.
func alignedContainer(t *testing.T, view *GedView) (a, box, child, c *FakeWidget) {
	t.Helper()
	scene := view.GedScene()

	a = addFakeAt(t, scene, "a", 0, 5, 10, 4)

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("box")
	box.SetBounds(20, 30, 40, 30)
	box.SetParent(scene)

	child = addFakeAt(t, scene, "child", 25, 35, 20, 7)
	child.SetParent(box)

	c = addFakeAt(t, scene, "c", 100, 50, 10, 4)
	return
}

// TestAlignSelectionMultiCarriesContainerChildren: the multi-item align branch
// builds its own MoveCommand with one target per selected item. A container's
// target has to bring the container's contents with it, the same way a drag and
// a nudge do — otherwise 左对齐 on a form's layout box slides the box out from
// under its own widgets.
//
// One undo has to return the whole subtree, which is only true if the child was
// recorded on the same command as its container.
func TestAlignSelectionMultiCarriesContainerChildren(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	a, box, child, c := alignedContainer(t, view)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(box)
	view.Selection().Add(c)

	view.AlignSelection(AlignLeft) // min left = 0, so the box moves -20 in X

	if x, y := box.Pos(); x != 0 || y != 30 {
		t.Fatalf("container after align = (%v,%v), want (0,30) — the align itself did not land", x, y)
	}
	if x, y := child.Pos(); x != 5 || y != 35 {
		t.Errorf("child after align = (%v,%v), want (5,35) — it was left behind", x, y)
	}

	view.Scene().UndoStack().Undo()
	if x, y := box.Pos(); x != 20 || y != 30 {
		t.Errorf("container after undo = (%v,%v), want (20,30)", x, y)
	}
	if x, y := child.Pos(); x != 25 || y != 35 {
		t.Errorf("child after undo = (%v,%v), want (25,35) — undo only reaches what the command recorded", x, y)
	}
}

// TestAlignSelectionMultiDoesNotMoveChildTwice: selecting a container and one of
// its children and aligning both would hand the child two targets — the one it
// rides from its container, and its own. Applying both tears it out of the
// container, so its container's target is the one that counts.
func TestAlignSelectionMultiDoesNotMoveChildTwice(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	a, box, child, _ := alignedContainer(t, view)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(box)
	view.Selection().Add(child)

	view.AlignSelection(AlignLeft)

	if x, y := box.Pos(); x != 0 || y != 30 {
		t.Fatalf("container after align = (%v,%v), want (0,30)", x, y)
	}
	if x, y := child.Pos(); x != 5 || y != 35 {
		t.Errorf("child after align = (%v,%v), want (5,35) — its own target moved it a second time", x, y)
	}
}
