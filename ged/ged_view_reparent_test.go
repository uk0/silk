package ged

import (
	"testing"
	"time"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// clearClickHistory forgets the last canvas press. OnLeftDown keeps that in
// package globals, so two tests pressing the same spot within 400ms are read as
// a double-click and the second gesture is swallowed — an ordering dependency
// between tests, not a behaviour any of them means to exercise.
func clearClickHistory() {
	lastClickTime = time.Time{}
}

// dragOnCanvas replays a press-move-release gesture on the canvas. Positions
// are scene millimetres; they are mapped through the view so the tool sees the
// same pixel coordinates a real mouse would deliver.
func dragOnCanvas(view *GedView, fromX, fromY, toX, toY float64) {
	clearClickHistory()
	px, py := view.MapFromScene(fromX, fromY)
	qx, qy := view.MapFromScene(toX, toY)
	view.OnLeftDown(px, py)
	view.OnMouseMove(qx, qy)
	view.OnLeftUp(qx, qy)
}

// TestCanvasDragReparentsIntoContainer is the drag-to-nest behaviour: dragging
// an existing widget over a container makes that container its parent, without
// the widget moving away from where it was dropped. Undo puts it back under the
// scene root at its original sibling index.
//
// The scene position assertion is the whole point of the coordinate rule: the
// re-parent runs after the move command and the grid snap, so the widget must
// still sit at the snapped resting place (100,40) once it belongs to the VBox.
func TestCanvasDragReparentsIntoContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(10, 10, 25, 7)
	btn.SetParent(scene)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	view.Selection().Add(btn)
	dragOnCanvas(view, 12, 12, 100, 40)

	if btn.Parent() != graph.IItem(vbox) {
		t.Fatalf("parent after dragging onto the VBox = %v, want the VBox", btn.Parent())
	}
	// Detaching an item drops it from the selection; the widget the user just
	// dropped has to stay selected for the property panel to follow it.
	if !view.Selection().Contains(btn) {
		t.Errorf("selection after the drop = %v, want the dropped widget", view.Selection().ItemList())
	}
	if x, y := btn.Pos(); x != 100 || y != 40 {
		t.Errorf("position after the re-parent = (%v,%v), want (100,40) — the widget moved", x, y)
	}

	// Undo pops the re-parent: back under the scene root, in slot 0 (it was
	// added before the VBox), still at the dragged position.
	view.Scene().UndoStack().Undo()
	if btn.Parent() != graph.IItem(scene) {
		t.Fatalf("parent after undo = %v, want the scene root", btn.Parent())
	}
	if got := btn.IndexInParent(); got != 0 {
		t.Errorf("sibling index after undo = %d, want 0", got)
	}
	if x, y := btn.Pos(); x != 100 || y != 40 {
		t.Errorf("position after undo = (%v,%v), want (100,40) — undo of a re-parent must not move the widget", x, y)
	}

	// The move underneath it is a separate step, so a second undo restores the
	// pre-drag position.
	view.Scene().UndoStack().Undo()
	if x, y := btn.Pos(); x != 10 || y != 10 {
		t.Errorf("position after undoing the move = (%v,%v), want (10,10)", x, y)
	}
}

// TestCanvasDragReparentsEverySelectedItem: the drop applies to the whole
// selection, not just the widget under the cursor, and one undo returns all of
// them to their original parent and slots.
func TestCanvasDragReparentsEverySelectedItem(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btnA, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btnA.SetBounds(10, 10, 25, 7)
	btnA.SetParent(scene)

	btnB, _ := NewFakeWidgetFromFactory("gui.Button")
	btnB.SetBounds(10, 30, 25, 7)
	btnB.SetParent(scene)

	vbox, _ := NewFakeWidgetFromFactory("gui.VBox")
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	view.Selection().Add(btnA)
	view.Selection().Add(btnB)
	dragOnCanvas(view, 12, 12, 100, 40)

	for i, it := range []*FakeWidget{btnA, btnB} {
		if it.Parent() != graph.IItem(vbox) {
			t.Errorf("item %d parent = %v, want the VBox", i, it.Parent())
		}
	}
	// Both moved by the same delta and snapped: 10+88 → 100, 10/30+28 → 40/60.
	if x, y := btnB.Pos(); x != 100 || y != 60 {
		t.Errorf("second item position = (%v,%v), want (100,60)", x, y)
	}

	view.Scene().UndoStack().Undo()
	if btnA.IndexInParent() != 0 || btnB.IndexInParent() != 1 {
		t.Errorf("indices after undo = %d,%d, want 0,1",
			btnA.IndexInParent(), btnB.IndexInParent())
	}
}

// TestCanvasDragOntoBackgroundReparentsToRoot is the way out of a container:
// dragging a nested widget onto empty canvas hands it back to the form root,
// which is the same code path with no container under the cursor.
func TestCanvasDragOntoBackgroundReparentsToRoot(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(65, 15, 25, 7)
	btn.SetParent(vbox)

	view.Selection().Add(btn)
	dragOnCanvas(view, 67, 17, 170, 120)

	if btn.Parent() != graph.IItem(scene) {
		t.Fatalf("parent after dragging onto the background = %v, want the scene root", btn.Parent())
	}

	view.Scene().UndoStack().Undo()
	if btn.Parent() != graph.IItem(vbox) {
		t.Errorf("parent after undo = %v, want the VBox it came from", btn.Parent())
	}
}

// TestEscapeDuringDragCancelsTheReparent: ESC empties the selection, which is
// how a drag is cancelled today — the move command comes out empty and so must
// the re-parent. Both are derived from the live selection at release time, so
// there is nothing left to apply.
func TestEscapeDuringDragCancelsTheReparent(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(10, 10, 25, 7)
	btn.SetParent(scene)

	vbox, _ := NewFakeWidgetFromFactory("gui.VBox")
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	view.Selection().Add(btn)

	clearClickHistory()
	px, py := view.MapFromScene(12, 12)
	qx, qy := view.MapFromScene(100, 40)
	view.OnLeftDown(px, py)
	view.OnMouseMove(qx, qy)
	view.OnKeyDown(gui.KeyEsc, false)
	view.OnLeftUp(qx, qy)

	if btn.Parent() != graph.IItem(scene) {
		t.Errorf("parent after a cancelled drag = %v, want the scene root", btn.Parent())
	}
	if x, y := btn.Pos(); x != 10 || y != 10 {
		t.Errorf("position after a cancelled drag = (%v,%v), want (10,10)", x, y)
	}
	if got := view.Scene().UndoStack().Count(); got != 0 {
		t.Errorf("undo steps after a cancelled drag = %d, want 0", got)
	}
}

// TestClickInsideContainerDoesNotReparent: a press-release with no movement is
// a selection click, not a drag. Every press arms the drag state, so without a
// "the pointer actually moved" test the click would hand the widget to the form
// root — clicking a nested widget would pop it out of its container.
func TestClickInsideContainerDoesNotReparent(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetBounds(65, 15, 25, 7)
	btn.SetParent(vbox)

	clearClickHistory()
	px, py := view.MapFromScene(67, 17)
	view.OnLeftDown(px, py)
	view.OnLeftUp(px, py)

	if btn.Parent() != graph.IItem(vbox) {
		t.Errorf("parent after a plain click = %v, want the VBox it sits in", btn.Parent())
	}
	if got := view.Scene().UndoStack().Count(); got != 0 {
		t.Errorf("undo steps after a click = %d, want 0", got)
	}
}

// TestNearestDropContainerSkipsTheMovingSet drives the drop-target rule
// headlessly: the deepest container under the cursor wins, but a container
// that is being dragged — or that sits inside one that is — is not a legal
// destination, so the walk keeps looking further up.
func TestNearestDropContainerSkipsTheMovingSet(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	outer, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create outer VBox: %v", err)
	}
	outer.SetBounds(0, 0, 150, 120)
	outer.SetParent(scene)

	inner, _ := NewFakeWidgetFromFactory("gui.VBox")
	inner.SetBounds(10, 10, 80, 60)
	inner.SetParent(outer)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetBounds(20, 20, 25, 7)
	btn.SetParent(inner)

	moving := func(set ...graph.IItem) func(graph.IItem) bool {
		return func(it graph.IItem) bool {
			for _, s := range set {
				if s == it {
					return true
				}
			}
			return false
		}
	}
	hit := scene.FindItemAt(25, 24, nil) // the button

	cases := []struct {
		name string
		set  []graph.IItem
		want graph.IItem
	}{
		{"nothing moving", nil, inner},
		{"the button itself", []graph.IItem{btn}, inner},
		{"its container", []graph.IItem{inner}, outer},
		{"the outer container", []graph.IItem{outer}, nil},
		{"both containers", []graph.IItem{inner, outer}, nil},
	}
	for _, tc := range cases {
		if got := nearestDropContainer(hit, moving(tc.set...)); got != tc.want {
			t.Errorf("%s: drop container = %v, want %v", tc.name, got, tc.want)
		}
	}

	// Empty canvas has no hit at all → the form root.
	if got := nearestDropContainer(scene.FindItemAt(180, 140, nil), moving()); got != nil {
		t.Errorf("drop container over empty canvas = %v, want nil (the form root)", got)
	}
}

// TestMarqueeDragDoesNotReparent: a rubber-band selection never moves a
// widget, so ending one over a container must leave the tree alone — even
// though the canvas had a selection when the drag began and the band replaces
// it with a different widget on release.
//
// The band has to start outside the page: the scene root is itself selectable
// and movable, so a press anywhere on the canvas is claimed by MovePart (see
// the note on the pre-existing scene-root move in the report).
func TestMarqueeDragDoesNotReparent(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	// Nested, and inside the band: a re-parent driven by this gesture would
	// lift it out of the VBox and hand it to the form root.
	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetBounds(65, 50, 25, 7)
	btn.SetParent(vbox)

	// Selected before the gesture, and outside the band, so the drag begins
	// with a non-empty selection the marquee will replace on release.
	pre, _ := NewFakeWidgetFromFactory("gui.Button")
	pre.SetBounds(150, 120, 25, 7)
	pre.SetParent(scene)
	view.Selection().Add(pre)

	// Press off-page, release inside the VBox; the band swallows btn.
	dragOnCanvas(view, -10, 145, 100, 40)

	if !view.Selection().Contains(btn) {
		t.Fatalf("the band should have selected btn, selection = %v", view.Selection().ItemList())
	}
	if btn.Parent() != graph.IItem(vbox) {
		t.Errorf("parent after a marquee ending over the VBox = %v, want the VBox it started in", btn.Parent())
	}
	if got := view.Scene().UndoStack().Count(); got != 0 {
		t.Errorf("undo steps after a marquee = %d, want 0", got)
	}
}

// TestCanvasDragInsideSameParentPushesNoReparent: a plain move that ends over
// the widget's own parent changes nothing structurally, so the gesture must
// leave a single undo step (the move) behind.
func TestCanvasDragInsideSameParentPushesNoReparent(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(10, 10, 25, 7)
	btn.SetParent(scene)

	view.Selection().Add(btn)
	dragOnCanvas(view, 12, 12, 50, 40)

	if got := view.Scene().UndoStack().Count(); got != 1 {
		t.Errorf("undo steps after a move inside one parent = %d, want 1 (the move)", got)
	}
}
