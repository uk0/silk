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

// TestCanvasDropSnapParkIsOneUndoStep ensures there is no history or listener
// state where a newly owned control has been clipped completely out of view.
func TestCanvasDropSnapParkIsOneUndoStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn := sceneWidget(t, scene, "gui.Button", "btn", 10, 10, 25, 7)
	box := sceneWidget(t, scene, "gui.VBox", "box", 66, 10, 74, 60)
	changes := 0
	scene.SigDesignChanged(func() {
		changes++
		if btn.Parent() != graph.IItem(box) || drawnRect(btn).IsEmpty() {
			t.Errorf("design signal %d observed incomplete drop: parent=%v clip=%v", changes, btn.Parent(), drawnRect(btn))
		}
	})

	view.Selection().Add(btn)
	dragOnCanvas(view, 34, 11.75, 66.4, 41.75)

	if changes != 1 || scene.UndoStack().Count() != 1 {
		t.Fatalf("release committed %d changes / %d history entries; want 1/1", changes, scene.UndoStack().Count())
	}
	scene.SigDesignChanged(nil)
	after := btn.Bounds1()
	view.Undo()
	if btn.Parent() != graph.IItem(scene) || scene.UndoStack().CanUndo() || drawnRect(btn).IsEmpty() {
		t.Fatal("one undo did not restore the visible original root widget")
	}
	if x, y := btn.Pos(); x != 10 || y != 10 {
		t.Fatalf("restored position = (%v,%v), want original (10,10)", x, y)
	}
	view.Redo()
	if btn.Parent() != graph.IItem(box) || btn.Bounds1() != after || drawnRect(btn).IsEmpty() {
		t.Fatalf("redo lost the parked placement: bounds=%v want %v", btn.Bounds1(), after)
	}
	checkWidgetsAreDrawn(t, scene, "redo one atomic canvas drop")
}
