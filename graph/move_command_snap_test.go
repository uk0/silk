package graph

import "testing"

// Former implicit-snap regression cases, now enforcing precise targets while
// retaining the rigid-subtree invariants. Explicit snapping belongs to the view.
func nearly(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	return d < eps && d > -eps
}

func TestMoveCommandPrecisionKeepsSubtreeRigid(t *testing.T) {
	v := NewView()
	box, child := boxWithChild(10.4, 15.6)
	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 0)
	if cmd == nil {
		t.Fatal("missing move")
	}
	cmd.Redo()
	if !nearly(box.X(), 17.4) || box.Y() != 10 || !nearly(child.X()-box.X(), 5.2) || child.Y()-box.Y() != 10 {
		t.Fatal("fractional subtree changed", box.Bounds1(), child.Bounds1())
	}
}

func TestMoveCommandPrecisionCarriesEveryDescendant(t *testing.T) {
	v := NewView()
	box := NewRectItem()
	box.SetBounds(10.4, 20.7, 60, 60)
	first := NewRectItem()
	first.SetBounds(15.6, 25.9, 10, 10)
	first.SetParent(box)
	second := NewRectItem()
	second.SetBounds(30.7, 45.2, 10, 10)
	second.SetParent(box)
	grand := NewRectItem()
	grand.SetBounds(32.3, 46.6, 4, 4)
	grand.SetParent(second)
	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 3)
	if cmd == nil {
		t.Fatal("missing move")
	}
	cmd.Redo()
	for _, tc := range []struct {
		item IItem
		x, y float64
	}{{box, 17.4, 23.7}, {first, 22.6, 28.9}, {second, 37.7, 48.2}, {grand, 39.3, 49.6}} {
		if !nearly(tc.item.X(), tc.x) || !nearly(tc.item.Y(), tc.y) {
			t.Errorf("position=%v want=(%v,%v)", tc.item.Bounds1(), tc.x, tc.y)
		}
	}
}

func TestMoveCommandPrecisionIndependentTargets(t *testing.T) {
	a, b := NewRectItem(), NewRectItem()
	a.SetBounds(.4, 1.4, 10, 10)
	b.SetBounds(20.6, 30.6, 10, 10)
	cmd := NewMoveCommand()
	AddMoveSubtree(cmd, a, 4.2-.4, 8.7-1.4)
	AddMoveSubtree(cmd, b, 9.4-20.6, 12.3-30.6)
	cmd.Redo()
	if !nearly(a.X(), 4.2) || !nearly(a.Y(), 8.7) || !nearly(b.X(), 9.4) || !nearly(b.Y(), 12.3) {
		t.Fatal("independent alignment targets changed", a.Bounds1(), b.Bounds1())
	}
}

func TestMoveCommandPrecisionReplaysExactly(t *testing.T) {
	v := NewView()
	box, child := boxWithChild(10.4, 15.6)
	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7.5, 7.5)
	if cmd == nil {
		t.Fatal("missing move")
	}
	cmd.Redo()
	bx, by := box.Pos()
	cx, cy := child.Pos()
	for i := 0; i < 5; i++ {
		cmd.Undo()
		if box.X() != 10.4 || box.Y() != 10 || child.X() != 15.6 || child.Y() != 20 {
			t.Fatal("undo failed to restore exact originals")
		}
		cmd.Redo()
		if box.X() != bx || box.Y() != by || child.X() != cx || child.Y() != cy {
			t.Fatal("replay drifted")
		}
		if !nearly(child.X()-box.X(), 5.2) || child.Y()-box.Y() != 10 {
			t.Fatal("subtree offset changed")
		}
	}
}

func TestMoveCommandPrecisionKeepsEachSubtreeDelta(t *testing.T) {
	boxA, kidA := boxWithChild(10.4, 15.6)
	boxB, kidB := boxWithChild(30.6, 35.9)
	cmd := NewMoveCommand()
	AddMoveSubtree(cmd, boxA, 7.25, .5)
	AddMoveSubtree(cmd, boxB, -3.75, 2.25)
	cmd.Redo()
	for _, tc := range []struct {
		box, kid     IItem
		x, y, offset float64
	}{{boxA, kidA, 17.65, 10.5, 5.2}, {boxB, kidB, 26.85, 12.25, 5.3}} {
		if !nearly(tc.box.X(), tc.x) || tc.box.Y() != tc.y || !nearly(tc.kid.X()-tc.box.X(), tc.offset) || tc.kid.Y()-tc.box.Y() != 10 {
			t.Fatal("one subtree took another's target", tc.box.Bounds1(), tc.kid.Bounds1())
		}
	}
}

func boxWithChild(x, kidX float64) (box, kid IItem) {
	b := NewRectItem()
	b.SetBounds(x, 10, 60, 40)
	k := NewRectItem()
	k.SetBounds(kidX, 20, 20, 10)
	k.SetParent(b)
	return b, k
}
