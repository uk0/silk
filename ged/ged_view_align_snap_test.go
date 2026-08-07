package ged

import "testing"

// TestAlignSelectionSnapsTargetsButNotContainerContents drives the multi-item
// align branch with a container in the selection. The two things it asks for
// pull in opposite directions and both have to hold:
//
//   - each selected widget has its own computed target, and each of those still
//     snaps to the 1mm grid;
//   - the widget inside the container has no target of its own — it was handed
//     the container's delta — so it has to take the container's snap too. Round
//     it separately and it slides inside the container it lives in.
//
// AlignHCenter is the mode that makes this routine rather than exotic: it
// averages the centres, so fractional targets are the normal case, not an edge
// one.
func TestAlignSelectionSnapsTargetsButNotContainerContents(t *testing.T) {
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

	// Mean centre X = ((10.4+30) + (100+10)) / 2 = 75.2, so the targets are
	// 45.2 and 65.2 — both land on the grid at 45 and 65.
	bx, _ := box.Pos()
	if bx != 45 {
		t.Fatalf("container X = %v, want 45 — its own aligned target must still snap to the grid", bx)
	}
	if x, _ := other.Pos(); x != 65 {
		t.Errorf("second widget X = %v, want 65 — it must snap onto its own target, not follow the container's", x)
	}

	// 15.9 - 10.4 = 5.5. Rounding the inner widget on its own turns that into
	// 6 (50.7 rounds up while the container rounds down), moving it half a
	// millimetre inside a container the user never touched.
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
