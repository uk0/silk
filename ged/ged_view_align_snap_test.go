package ged

import "testing"

// Alignment must preserve its exact computed centres and the original offset
// of every carried child; neither independent targets nor descendants round.
func TestAlignSelectionPreservesTargetsAndContainerContents(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	box, err := NewFakeWidgetFromFactory("gui.GroupBox")
	if err != nil {
		t.Fatalf("create GroupBox: %v", err)
	}
	box.SetBounds(10.4, 20, 60, 40)
	box.SetParent(scene)

	inner := addFakeAt(t, scene, "inner", 15.9, 25, 10, 5)
	inner.SetParent(box)

	other := addFakeAt(t, scene, "other", 100, 60, 20, 10)

	view.Selection().Clear()
	view.Selection().Add(box)
	view.Selection().Add(other)

	view.AlignSelection(AlignHCenter)

	// Mean centre X = 75.2, hence exact targets 45.2 and 65.2.
	requirePrecisePosition(t, box, 45.2, 20)
	requirePrecisePosition(t, other, 65.2, 60)
	bx, _ := box.Pos()
	// The child's 5.5 mm relative offset must survive the fractional move.

	ix, _ := inner.Pos()
	const eps = 1e-9
	if d := (ix - bx) - 5.5; d > eps || d < -eps {
		t.Errorf("widget inside the container sits at %v with the container at %v: offset %v, want 5.5 — align moved it inside its own container", ix, bx, ix-bx)
	}

	view.Scene().UndoStack().Undo()
	if x, _ := box.Pos(); x != 10.4 {
		t.Errorf("container X after undo = %v, want 10.4", x)
	}
	if x, _ := inner.Pos(); x != 15.9 {
		t.Errorf("inner widget X after undo = %v, want 15.9", x)
	}
}
