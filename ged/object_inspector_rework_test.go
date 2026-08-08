package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestParkInsideParentRescuesAWidgetTheClipWouldHide is the regression guard
// for a tree re-parent that made the widget disappear. FakeWidget has no local
// coordinates, so a child keeps its scene rect while Item.DrawAll clips it to
// the intersection with its new parent — and returns early when that is empty.
// A canvas drop cannot hit this (the pointer was already over the container);
// a tree drop has no pointer at all.
func TestParkInsideParentRescuesAWidgetTheClipWouldHide(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 10, 60, 40, 8) // below the box, no overlap
	btn.SetParent(box)

	park([]graph.IItem{btn}, box)

	if !rectsOverlap(btn, box) {
		t.Errorf("button at (%.1f,%.1f) still has no intersection with its parent box (%.1f,%.1f %.1fx%.1f); DrawAll clips it away",
			btn.X(), btn.Y(), box.X(), box.Y(), box.Width(), box.Height())
	}
}

// TestParkInsideParentLeavesAnOverlappingWidgetAlone keeps the rescue from
// becoming a layout stomp: a tree drop states intent about ownership, not about
// position, so a widget that is already visible must not be moved.
func TestParkInsideParentLeavesAnOverlappingWidgetAlone(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 20, 20, 30, 8)
	btn.SetParent(box)
	x, y := btn.X(), btn.Y()

	park([]graph.IItem{btn}, box)

	if btn.X() != x || btn.Y() != y {
		t.Errorf("an already-visible widget was moved from (%.1f,%.1f) to (%.1f,%.1f)", x, y, btn.X(), btn.Y())
	}
}

// TestParkInsideParentHonoursAPositionLock: a lock means the user asked for the
// position to stay put, which outranks the rescue.
func TestParkInsideParentHonoursAPositionLock(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 10, 60, 40, 8)
	btn.SetParent(box)
	btn.SetLockPos(true)

	park([]graph.IItem{btn}, box)

	if btn.Y() != 60 {
		t.Errorf("a position-locked widget was moved to y=%.1f", btn.Y())
	}
}

// TestParkInsideParentIgnoresTheFormRoot — the root clips nothing, so parking
// against it would move widgets for no reason.
func TestParkInsideParentIgnoresTheFormRoot(t *testing.T) {
	btn := newTestFake(t, "gui.Button", 200, 200, 40, 8)
	park([]graph.IItem{btn}, nil)
	if btn.X() != 200 || btn.Y() != 200 {
		t.Errorf("re-parenting to the form root moved the widget to (%.1f,%.1f)", btn.X(), btn.Y())
	}
}

// TestParkInsideParentRescuesAnEdgeFlushWidget pins the boundary the overlap
// test is drawn on. A widget whose edge sits exactly on its new parent's edge
// shares a line with it and nothing more: DrawAll intersects the two rects and
// returns early on an empty result, and geom.Rect counts a zero width or
// height as empty — so "touching" renders as the same invisible widget a
// widget miles away does, and the park has to rescue it too.
//
// One case per comparison: each subtest is flush on one side and comfortably
// inside on the other three, so a single loosened comparison is enough to fail
// exactly one of them.
func TestParkInsideParentRescuesAnEdgeFlushWidget(t *testing.T) {
	// The parent spans x 10..70, y 10..50 throughout.
	cases := []struct {
		name string
		x, y float64
	}{
		{"left edge on the parent's right edge", 70, 20},
		{"right edge on the parent's left edge", -30, 20},
		{"top edge on the parent's bottom edge", 20, 50},
		{"bottom edge on the parent's top edge", 20, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
			btn := newTestFake(t, "gui.Button", c.x, c.y, 40, 8)
			btn.SetParent(box)

			park([]graph.IItem{btn}, box)

			// The predicate DrawAll itself uses, not a re-spelling of it.
			if box.Bounds1().IntersectCopy(btn.Bounds1()).IsEmpty() {
				t.Errorf("button at (%.1f,%.1f %.1fx%.1f) still shares nothing but a line with its parent box (%.1f,%.1f %.1fx%.1f); DrawAll clips it away",
					btn.X(), btn.Y(), btn.Width(), btn.Height(),
					box.X(), box.Y(), box.Width(), box.Height())
			}
		})
	}
}

// park applies whatever parkInsideParent decided, the way the undo stack does
// it (Push calls Redo). A nil command is the "nothing had fallen outside"
// answer, which applies as nothing.
func park(items []graph.IItem, parent graph.IItem) {
	if cmd := parkInsideParent(items, parent); cmd != nil {
		cmd.Redo()
	}
}

// TestTreeDropCarriesTheContainersChildren is the same failure one level down.
// The park moved the dropped container on its own, so the widgets inside it
// kept the coordinates they had wherever the container used to be: the
// container lands inside its new owner and its contents are left outside it,
// where DrawAll's clip erases them. Round 6 rescued the widget the user
// dropped; nothing rescued the widgets riding inside it.
//
// The gesture is replayed through the tree rather than by calling the park, so
// the reparent's own coordinate handling is part of what is under test.
func TestTreeDropCarriesTheContainersChildren(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "box", 120, 100, 40, 30)
	btn := newTestFake(t, "gui.Button", 125, 105, 20, 8)
	btn.SetWidgetName("btn")
	btn.SetParent(box)
	group := sceneWidget(t, scene, "gui.GroupBox", "group", 10, 10, 60, 50)
	dx, dy := btn.X()-box.X(), btn.Y()-box.Y()

	dragTreeRow(t, boundInspector(view), box, group)

	if !rectsOverlap(btn, box) {
		t.Errorf("btn at (%.1f,%.1f) has no intersection with the box it lives in (%.1f,%.1f %.1fx%.1f) — the drop parked the container and left its contents behind, where DrawAll clips them away",
			btn.X(), btn.Y(), box.X(), box.Y(), box.Width(), box.Height())
	}
	if gx, gy := btn.X()-box.X(), btn.Y()-box.Y(); gx != dx || gy != dy {
		t.Errorf("btn sits at (%+.1f,%+.1f) inside the box, was (%+.1f,%+.1f) — a tree drop states intent about ownership, not about the layout inside the container",
			gx, gy, dx, dy)
	}
	if !rectsOverlap(box, group) {
		t.Errorf("the box at (%.1f,%.1f) never landed inside the group at (%.1f,%.1f %.1fx%.1f)",
			box.X(), box.Y(), group.X(), group.Y(), group.Width(), group.Height())
	}
}

// TestTreeDropUndoRestoresTheWholeSubtree: the park rides on the re-parent's
// own undo record, so a Ctrl+Z that puts the container back where it came from
// has to bring its contents with it. A subtree that only half returns is the
// same invisible widget, reached from the other side.
func TestTreeDropUndoRestoresTheWholeSubtree(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "box", 120, 100, 40, 30)
	btn := newTestFake(t, "gui.Button", 125, 105, 20, 8)
	btn.SetWidgetName("btn")
	btn.SetParent(box)
	group := sceneWidget(t, scene, "gui.GroupBox", "group", 10, 10, 60, 50)

	dragTreeRow(t, boundInspector(view), box, group)
	for scene.UndoStack().CanUndo() {
		scene.UndoStack().Undo()
	}

	if box.X() != 120 || box.Y() != 100 {
		t.Errorf("after undo the box is at (%.1f,%.1f), want (120,100)", box.X(), box.Y())
	}
	if btn.X() != 125 || btn.Y() != 105 {
		t.Errorf("after undo btn is at (%.1f,%.1f), want (125,105) — it stayed where the park put it while its container went home, so it is invisible again and no undo step is left to repair it",
			btn.X(), btn.Y())
	}
}

// TestTreeDropRedoParksAgain: a redo has to end where the drop ended. The
// re-parent's own record replays the position the widget had BEFORE the park,
// so a park that is not on the stack replays as "back inside the container, at
// the coordinates it had across the form" — the same invisible widget, this
// time handed over by Ctrl+Y.
func TestTreeDropRedoParksAgain(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "box", 120, 100, 40, 30)
	btn := newTestFake(t, "gui.Button", 125, 105, 20, 8)
	btn.SetWidgetName("btn")
	btn.SetParent(box)
	group := sceneWidget(t, scene, "gui.GroupBox", "group", 10, 10, 60, 50)

	dragTreeRow(t, boundInspector(view), box, group)
	x, y := box.X(), box.Y()
	for scene.UndoStack().CanUndo() {
		scene.UndoStack().Undo()
	}
	for scene.UndoStack().CanRedo() {
		scene.UndoStack().Redo()
	}

	if box.X() != x || box.Y() != y {
		t.Errorf("after undo+redo the box is at (%.1f,%.1f), the drop left it at (%.1f,%.1f)", box.X(), box.Y(), x, y)
	}
	if !rectsOverlap(box, group) {
		t.Errorf("redo put the box back inside the group at (%.1f,%.1f), outside the group's own rect (%.1f,%.1f %.1fx%.1f) — invisible again",
			box.X(), box.Y(), group.X(), group.Y(), group.Width(), group.Height())
	}
	if !rectsOverlap(btn, box) {
		t.Errorf("redo left btn at (%.1f,%.1f), outside the box at (%.1f,%.1f)", btn.X(), btn.Y(), box.X(), box.Y())
	}
}

// TestTreeDropOntoAnOverlappingContainerUndoesInOneStep: one drop stays one
// Ctrl+Z. The rescue rides inside the re-parent's step and a drop that needed
// no rescue records none at all, so nothing stands between the user and the
// gesture they are taking back.
func TestTreeDropOntoAnOverlappingContainerUndoesInOneStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	group := sceneWidget(t, scene, "gui.GroupBox", "group", 10, 10, 60, 50)
	btn := sceneWidget(t, scene, "gui.Button", "btn", 20, 20, 30, 8) // already inside the group's rect

	dragTreeRow(t, boundInspector(view), btn, group)
	if btn.Parent() != graph.IItem(group) {
		t.Fatalf("the drop did not nest btn: parent is %T", btn.Parent())
	}
	scene.UndoStack().Undo()

	if btn.Parent() != graph.IItem(scene) {
		t.Errorf("one Ctrl+Z left btn parented to %T, want the form — the drop needed no park, so nothing else may sit on top of the re-parent", btn.Parent())
	}
}

func rectsOverlap(a, b graph.IItem) bool {
	return a.X() < b.X()+b.Width() && a.X()+a.Width() > b.X() &&
		a.Y() < b.Y()+b.Height() && a.Y()+a.Height() > b.Y()
}

func newTestFake(t *testing.T, factory string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	it, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("NewFakeWidgetFromFactory(%q): %v", factory, err)
	}
	it.SetBounds(x, y, w, h)
	return it
}
