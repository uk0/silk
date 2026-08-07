package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// nestedContainer builds the tree a container drag has to carry: a VBox at
// (10,10,80,60) holding an HBox at (20,30,50,20) holding a Button at
// (25,35,25,7). Every coordinate is a multiple of the 5mm grid pitch, so the
// post-drag snap has nothing left to change and any position difference is the
// move itself. The press point (15,15) is inside the VBox and outside both of
// its descendants, so MovePart claims the container.
func nestedContainer(t *testing.T, view *GedView) (box, inner, btn *FakeWidget) {
	t.Helper()

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetBounds(10, 10, 80, 60)
	box.SetParent(view.GedScene())

	inner, err = NewFakeWidgetFromFactory("gui.HBox")
	if err != nil {
		t.Fatalf("create HBox: %v", err)
	}
	inner.SetBounds(20, 30, 50, 20)
	inner.SetParent(box)

	btn, err = NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(25, 35, 25, 7)
	btn.SetParent(inner)

	return box, inner, btn
}

// TestCanvasDragCarriesContainerChildren: dragging a container moves what is
// inside it. ged keeps every widget in scene coordinates — no container turns
// on local coords — so a child that keeps its old rectangle is left standing
// outside its own parent, which is what dragging a form's layout box used to do.
//
// The assertion is each descendant's absolute position: the offset inside the
// container has to come out of the drag identical to what went in, at both
// nesting levels. One undo puts the whole subtree back, because the same
// command that moved it recorded it.
func TestCanvasDragCarriesContainerChildren(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	box, inner, btn := nestedContainer(t, view)

	dragOnCanvas(view, 15, 15, 75, 55) // +60, +40

	if x, y := box.Pos(); x != 70 || y != 50 {
		t.Fatalf("container after the drag = (%v,%v), want (70,50) — the drag itself did not land", x, y)
	}
	if x, y := inner.Pos(); x != 80 || y != 70 {
		t.Errorf("inner container after the drag = (%v,%v), want (80,70) — it was left behind", x, y)
	}
	if x, y := btn.Pos(); x != 85 || y != 75 {
		t.Errorf("button after the drag = (%v,%v), want (85,75) — it was left behind", x, y)
	}

	view.Scene().UndoStack().Undo()
	if x, y := box.Pos(); x != 10 || y != 10 {
		t.Errorf("container after undo = (%v,%v), want (10,10)", x, y)
	}
	if x, y := inner.Pos(); x != 20 || y != 30 {
		t.Errorf("inner container after undo = (%v,%v), want (20,30)", x, y)
	}
	if x, y := btn.Pos(); x != 25 || y != 35 {
		t.Errorf("button after undo = (%v,%v), want (25,35) — undo only reaches what the command recorded", x, y)
	}
}

// TestNudgeCarriesContainerChildren: the arrow keys reach the same command
// generator as a drag, so a nudged container carries its contents too.
func TestNudgeCarriesContainerChildren(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	box, inner, btn := nestedContainer(t, view)

	view.Selection().Add(box)
	view.nudgeSelection(5, 0)

	if x, y := box.Pos(); x != 15 || y != 10 {
		t.Fatalf("container after the nudge = (%v,%v), want (15,10) — the nudge itself did not land", x, y)
	}
	if x, y := inner.Pos(); x != 25 || y != 30 {
		t.Errorf("inner container after the nudge = (%v,%v), want (25,30) — it was left behind", x, y)
	}
	if x, y := btn.Pos(); x != 30 || y != 35 {
		t.Errorf("button after the nudge = (%v,%v), want (30,35) — it was left behind", x, y)
	}
}

// TestHeldArrowKeyOverContainerIsOneUndoStep: carrying the children makes a
// move command's record list a whole subtree, and a held arrow key collapses
// its repeats by walking those lists. The burst still has to be one undo step,
// and that step has to take the children back with it.
func TestHeldArrowKeyOverContainerIsOneUndoStep(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	box, _, btn := nestedContainer(t, view)

	view.Selection().Clear()
	view.Selection().Add(box)

	before := view.Scene().UndoStack().Count()
	view.OnKeyDown(gui.KeyRight, false)
	for i := 0; i < 4; i++ {
		view.OnKeyDown(gui.KeyRight, true)
	}

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Errorf("1 press + 4 repeats pushed %d undo commands, want 1", got)
	}
	if x, y := box.Pos(); x != 15 || y != 10 {
		t.Fatalf("container after the burst = (%v,%v), want (15,10)", x, y)
	}
	if x, y := btn.Pos(); x != 30 || y != 35 {
		t.Errorf("button after the burst = (%v,%v), want (30,35)", x, y)
	}

	view.Scene().UndoStack().Undo()
	if x, y := box.Pos(); x != 10 || y != 10 {
		t.Errorf("container after one undo = (%v,%v), want (10,10)", x, y)
	}
	if x, y := btn.Pos(); x != 25 || y != 35 {
		t.Errorf("button after one undo = (%v,%v), want (25,35) — the whole burst reverses at once", x, y)
	}
}
