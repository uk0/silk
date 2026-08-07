package graph

import "testing"

// nearly compares two millimetre coordinates. The grid snap is computed as a
// difference of float64 positions, so a rigid subtree comes back within a few
// ulps rather than bit-identical; the defect this guards against is worth
// 0.8mm, six orders of magnitude above the tolerance.
func nearly(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	return d < eps && d > -eps
}

// TestMoveCommandSnapKeepsSubtreeRigid: one gesture records a container and the
// widgets inside it at the same delta, so the grid snap has to move them as one
// body. Rounding each record on its own pulls them apart by up to 1mm — the
// child slides inside the container it lives in, which is a layout the designer
// never drew and which only shows up in the saved form, because Undo restores
// the unrounded positions.
//
// Fractional positions are routine: AlignHCenter divides, and a zoomed drag
// hands over a fractional delta.
func TestMoveCommandSnapKeepsSubtreeRigid(t *testing.T) {
	v := NewView()
	box := NewRectItem()
	box.SetBounds(10.4, 20, 60, 60)

	child := NewRectItem()
	child.SetBounds(15.6, 25, 10, 10)
	child.SetParent(box)

	offsetX := 15.6 - 10.4
	offsetY := 25.0 - 20.0

	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 0)
	if cmd == nil {
		t.Fatal("command is nil, want the container and its child")
	}
	cmd.Redo()

	bx, by := box.Pos()
	// The snap is the point of the rounding, so it has to survive the fix.
	if bx != 17 || by != 20 {
		t.Fatalf("container = (%v,%v), want (17,20) — the 1mm snap is gone", bx, by)
	}
	cx, cy := child.Pos()
	if !nearly(cx-bx, offsetX) || !nearly(cy-by, offsetY) {
		t.Errorf("child sits at (%v,%v) inside a container at (%v,%v): offset (%v,%v), want (%v,%v) — the snap moved it inside its own parent",
			cx, cy, bx, by, cx-bx, cy-by, offsetX, offsetY)
	}
}

// TestMoveCommandSnapRoundsIndependentTargets: the align and distribute paths
// put several items on one command, each with its OWN computed target, and each
// of those still has to land on the grid. A fix that made the whole command
// rigid — snapping the first record and shifting everyone by that — would drop
// every later item off the grid, which is the opposite of what align is for.
func TestMoveCommandSnapRoundsIndependentTargets(t *testing.T) {
	a, b := NewRectItem(), NewRectItem()
	a.SetBounds(0.4, 1.4, 10, 10)
	b.SetBounds(20.6, 30.6, 10, 10)

	// Shaped like AlignSelection's multi-item branch: one AddMoveSubtree call
	// per item, each with the delta onto that item's own aligned target.
	cmd := NewMoveCommand()
	AddMoveSubtree(cmd, a, 4.2-0.4, 8.7-1.4)
	AddMoveSubtree(cmd, b, 9.4-20.6, 12.3-30.6)
	cmd.Redo()

	if x, y := a.Pos(); x != 4 || y != 9 {
		t.Errorf("first aligned item = (%v,%v), want (4,9) — its own target stopped being snapped", x, y)
	}
	if x, y := b.Pos(); x != 9 || y != 12 {
		t.Errorf("second aligned item = (%v,%v), want (9,12) — it followed the first item's snap instead of its own", x, y)
	}
}

// TestMoveCommandSnapReplaysExactly: the command has to survive being replayed
// off the undo stack. Undo restores the untouched fractional positions, and a
// second Redo must land the subtree exactly where the first one did — a snap
// that re-applied itself on every replay would walk the child further out of
// its container each time round.
func TestMoveCommandSnapReplaysExactly(t *testing.T) {
	v := NewView()
	box := NewRectItem()
	box.SetBounds(10.4, 20.4, 60, 60)

	child := NewRectItem()
	child.SetBounds(15.6, 25.6, 10, 10)
	child.SetParent(box)

	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7.5, 7.5)
	if cmd == nil {
		t.Fatal("command is nil, want the container and its child")
	}

	cmd.Redo()
	bx1, by1 := box.Pos()
	cx1, cy1 := child.Pos()

	cmd.Undo()
	if x, y := box.Pos(); x != 10.4 || y != 20.4 {
		t.Fatalf("container after undo = (%v,%v), want (10.4,20.4) — undo must restore the unrounded position", x, y)
	}
	if x, y := child.Pos(); x != 15.6 || y != 25.6 {
		t.Fatalf("child after undo = (%v,%v), want (15.6,25.6) — undo must restore the unrounded position", x, y)
	}

	cmd.Redo()
	bx2, by2 := box.Pos()
	cx2, cy2 := child.Pos()
	if bx2 != bx1 || by2 != by1 {
		t.Errorf("container after replay = (%v,%v), want (%v,%v)", bx2, by2, bx1, by1)
	}
	if cx2 != cx1 || cy2 != cy1 {
		t.Errorf("child after replay = (%v,%v), want (%v,%v) — the snap drifted on the second redo", cx2, cy2, cx1, cy1)
	}
	if !nearly(cx2-bx2, 15.6-10.4) || !nearly(cy2-by2, 25.6-20.4) {
		t.Errorf("child offset after replay = (%v,%v), want (%v,%v)", cx2-bx2, cy2-by2, 15.6-10.4, 25.6-20.4)
	}

	cmd.Undo()
	if x, y := child.Pos(); x != 15.6 || y != 25.6 {
		t.Errorf("child after second undo = (%v,%v), want (15.6,25.6)", x, y)
	}
}
