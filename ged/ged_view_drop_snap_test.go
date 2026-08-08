package ged

import (
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
)

// drawnRect is the rectangle Item.DrawAll leaves on the canvas for item: the
// clip it inherits from every ancestor, intersected with its own bounds. That
// walk is DrawAll's own arithmetic (Bounds1, IntersectCopy) read from the
// bottom up, and DrawAll returns early when the intersection is empty — so an
// empty result is not a proxy for "the user cannot see it", it is the thing
// itself. Nothing of the item and nothing of its children reaches the painter.
func drawnRect(item graph.IItem) geom.Rect {
	clip := item.Bounds1()
	for p := item.Parent(); p != nil; p = p.Parent() {
		clip = clip.IntersectCopy(p.Bounds1())
	}
	return clip
}

// TestCanvasDropSnapKeepsTheWidgetVisible is the canvas twin of the tree drop's
// park. The drop container is picked from the CURSOR mid-drag; the grid snap
// then runs on release and moves the widget by up to pitch/2 (2.5mm at the
// default pitch of 5). A widget released just inside a container's edge is
// therefore snapped clear of the container it is about to be parented into, and
// DrawAll clips it to nothing: in the document, correct Parent(), invisible on
// the canvas and impossible to drag back.
//
// The geometry is arranged so only the snap does the damage, at the default
// pitch of 5. The button is grabbed 24mm from its left edge — between the
// corner and mid-edge resize handles, so the press is a move and not a resize —
// and released at x=66.4, just inside the box: the move lands the button at
// x=42.4 with 1.4mm of it inside the box, and the snap then rounds x down to
// 40, leaving its right edge 1mm short of the box. The y axis is left on a grid
// line so the whole story is on one axis.
func TestCanvasDropSnapKeepsTheWidgetVisible(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn := sceneWidget(t, scene, "gui.Button", "btn", 10, 10, 25, 7)
	box := sceneWidget(t, scene, "gui.VBox", "box", 66, 10, 74, 60)

	view.Selection().Add(btn)
	dragOnCanvas(view, 34, 11.75, 66.4, 41.75)

	// The re-parent is the premise, not the property: without it the widget
	// would simply have been dropped on the form root and stayed visible.
	if btn.Parent() != graph.IItem(box) {
		t.Fatalf("parent after the drop = %v, want the box — the gesture never re-parented, so this test proves nothing", btn.Parent())
	}
	if r := drawnRect(btn); r.IsEmpty() {
		t.Errorf("btn at (%.1f,%.1f %.1fx%.1f) draws as %v inside the box at (%.1f,%.1f %.1fx%.1f) — the snap pushed it out of the container it was parented into, and DrawAll paints none of it",
			btn.X(), btn.Y(), btn.Width(), btn.Height(), r,
			box.X(), box.Y(), box.Width(), box.Height())
	}
	checkWidgetsAreDrawn(t, scene, "canvas drop near a container edge")
}

// TestCanvasDropSnapParkRidesTheReparentsUndoStep is the placement half: the
// park has to be applied and taken back with the re-parent as a single step.
// Pushed as a step of its own it would leave a state one Ctrl+Z can stop in —
// the widget owned by the box and snapped outside it — which is the invisible
// widget the park exists to prevent; and left unrecorded it would not replay,
// so Ctrl+Y would put the widget back under the box without it.
//
// The same drag as above, so what changes here is only what the undo stack
// does with it.
func TestCanvasDropSnapParkRidesTheReparentsUndoStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn := sceneWidget(t, scene, "gui.Button", "btn", 10, 10, 25, 7)
	sceneWidget(t, scene, "gui.VBox", "box", 66, 10, 74, 60)

	view.Selection().Add(btn)
	dragOnCanvas(view, 34, 11.75, 66.4, 41.75)

	// One step back: out of the box again, at the position the drag left it.
	// Undrawn here is the stack having stopped between the re-parent and its
	// park — on the very state the pair exists to skip over.
	view.Scene().UndoStack().Undo()
	if r := drawnRect(btn); r.IsEmpty() {
		t.Errorf("one undo left btn at (%.1f,%.1f) owned by %v and drawing as %v — the park came off the stack on its own and stranded the widget inside a container it does not overlap",
			btn.X(), btn.Y(), btn.Parent(), r)
	}
	if btn.Parent() != graph.IItem(scene) {
		t.Errorf("parent after one undo = %v, want the form root — one Ctrl+Z has to take the whole re-parent back", btn.Parent())
	}
	if x, y := btn.Pos(); x != 40 || y != 40 {
		t.Errorf("position after one undo = (%v,%v), want the snapped drop position (40,40) — undoing the park must not leave the widget where the park put it", x, y)
	}

	// And forward again: the re-done re-parent must bring its park with it.
	view.Scene().UndoStack().Redo()
	if r := drawnRect(btn); r.IsEmpty() {
		t.Errorf("btn draws as %v after redo — it is at (%.1f,%.1f), back under the box and outside it", r, btn.X(), btn.Y())
	}
	checkWidgetsAreDrawn(t, scene, "redo of a canvas drop near a container edge")

	// The move underneath is still its own step: a second undo restores the
	// pre-drag position, exactly as it did before the park existed.
	view.Scene().UndoStack().Undo()
	view.Scene().UndoStack().Undo()
	if x, y := btn.Pos(); x != 10 || y != 10 {
		t.Errorf("position after undoing the move too = (%v,%v), want (10,10)", x, y)
	}
}
