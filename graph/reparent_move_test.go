package graph

import "testing"

// TestParentOriginMatchesChildCoordOffset pins ParentOrigin against the
// recurrence it has to mirror: whatever it reports for a parent is exactly what
// CoordOffset() reports for that parent's children. A local-coordinate parent
// shifts its children by its own position; a plain one passes its space through
// untouched.
func TestParentOriginMatchesChildCoordOffset(t *testing.T) {
	root := NewRectItem()
	root.SetLocalCoord(true)
	root.SetBounds(10, 20, 200, 200)

	inner := NewRectItem() // plain parent: adds nothing of its own
	inner.SetBounds(30, 40, 50, 50)
	inner.SetParent(root)

	leaf := NewRectItem()
	leaf.SetParent(inner)

	ox, oy := ParentOrigin(root)
	cx, cy := inner.CoordOffset()
	if ox != cx || oy != cy {
		t.Errorf("ParentOrigin(root) = (%v,%v), child CoordOffset = (%v,%v)", ox, oy, cx, cy)
	}
	if ox != 10 || oy != 20 {
		t.Errorf("ParentOrigin(local root) = (%v,%v), want (10,20)", ox, oy)
	}

	ox, oy = ParentOrigin(inner)
	cx, cy = leaf.CoordOffset()
	if ox != cx || oy != cy {
		t.Errorf("ParentOrigin(inner) = (%v,%v), child CoordOffset = (%v,%v)", ox, oy, cx, cy)
	}
	if ox != 10 || oy != 20 {
		t.Errorf("ParentOrigin(plain inner) = (%v,%v), want (10,20) — it must not add its own position", ox, oy)
	}

	if ox, oy := ParentOrigin(nil); ox != 0 || oy != 0 {
		t.Errorf("ParentOrigin(nil) = (%v,%v), want the scene origin (0,0)", ox, oy)
	}
}

// TestTranslateBetweenParentsKeepsScenePosition: moving a point from one
// local-coordinate container to another shifts it by the difference of the two
// origins, so the point stays where it was in scene space.
func TestTranslateBetweenParentsKeepsScenePosition(t *testing.T) {
	root := NewRectItem()
	root.SetLocalCoord(true)

	boxA := NewRectItem()
	boxA.SetLocalCoord(true)
	boxA.SetBounds(10, 20, 60, 60)
	boxA.SetParent(root)

	boxB := NewRectItem()
	boxB.SetLocalCoord(true)
	boxB.SetBounds(40, 50, 60, 60)
	boxB.SetParent(root)

	item := NewRectItem()
	item.SetBounds(5, 5, 10, 10)
	item.SetParent(boxA)
	wantX, wantY := item.MapToScene(item.Pos()) // (15,25)

	x1, y1 := TranslateBetweenParents(boxA, boxB, 5, 5)
	if x1 != -25 || y1 != -25 {
		t.Errorf("TranslateBetweenParents(A→B, 5,5) = (%v,%v), want (-25,-25)", x1, y1)
	}

	item.SetParent(boxB)
	item.SetPos(x1, y1)
	if gotX, gotY := item.MapToScene(item.Pos()); gotX != wantX || gotY != wantY {
		t.Errorf("scene position after the re-parent = (%v,%v), want (%v,%v)", gotX, gotY, wantX, wantY)
	}
}

// TestTranslateBetweenParentsIsIdentityInOneSpace covers the model the designer
// actually uses: no container opts into local coordinates, so every item in a
// scene shares one space and a re-parent must leave x,y alone. A translation
// here would be the bug, not the feature.
func TestTranslateBetweenParentsIsIdentityInOneSpace(t *testing.T) {
	root := NewRectItem()
	root.SetLocalCoord(true) // the scene root, the only local-coordinate item

	boxA := NewRectItem()
	boxA.SetBounds(10, 10, 50, 50)
	boxA.SetParent(root)

	boxB := NewRectItem()
	boxB.SetBounds(80, 10, 50, 50)
	boxB.SetParent(root)

	if x, y := TranslateBetweenParents(boxA, boxB, 20, 20); x != 20 || y != 20 {
		t.Errorf("container→container = (%v,%v), want (20,20)", x, y)
	}
	if x, y := TranslateBetweenParents(boxA, root, 20, 20); x != 20 || y != 20 {
		t.Errorf("container→root = (%v,%v), want (20,20)", x, y)
	}
	if x, y := TranslateBetweenParents(root, boxB, 20, 20); x != 20 || y != 20 {
		t.Errorf("root→container = (%v,%v), want (20,20)", x, y)
	}
}

// TestReparentCommandMovedRestoresPosition: AddMoved carries the position, so
// Redo lands the item at the translated spot and Undo returns it to its old
// parent, its old slot AND its old position — a plain Add would leave it at the
// translated coordinates inside the old parent.
func TestReparentCommandMovedRestoresPosition(t *testing.T) {
	root := NewRectItem()
	root.SetLocalCoord(true)

	boxA := NewRectItem()
	boxA.SetLocalCoord(true)
	boxA.SetBounds(10, 20, 60, 60)
	boxA.SetParent(root)

	boxB := NewRectItem()
	boxB.SetLocalCoord(true)
	boxB.SetBounds(40, 50, 60, 60)
	boxB.SetParent(root)

	item := NewRectItem()
	item.SetBounds(5, 5, 10, 10)
	item.SetParent(boxA)
	sibling := NewRectItem()
	sibling.SetParent(boxA) // item keeps index 0, sibling takes 1

	x1, y1 := TranslateBetweenParents(boxA, boxB, 5, 5)
	cmd := NewReparentCommand("")
	cmd.AddMoved(item, boxA, boxB, x1, y1)

	cmd.Redo()
	if item.Parent() != IItem(boxB) {
		t.Fatalf("after Redo: parent = %v, want boxB", item.Parent())
	}
	if x, y := item.Pos(); x != -25 || y != -25 {
		t.Errorf("after Redo: pos = (%v,%v), want (-25,-25)", x, y)
	}
	if x, y := item.MapToScene(item.Pos()); x != 15 || y != 25 {
		t.Errorf("after Redo: scene pos = (%v,%v), want (15,25) — the item moved on screen", x, y)
	}

	cmd.Undo()
	if item.Parent() != IItem(boxA) {
		t.Fatalf("after Undo: parent = %v, want boxA", item.Parent())
	}
	if got := item.IndexInParent(); got != 0 {
		t.Errorf("after Undo: index = %d, want 0", got)
	}
	if x, y := item.Pos(); x != 5 || y != 5 {
		t.Errorf("after Undo: pos = (%v,%v), want (5,5)", x, y)
	}
}

// TestGenerateReparentCommandRecordsOneMovePerTopLevelItem: a selection holding
// a container and its own child produces one record — the child rides along
// inside the container, exactly as GenerateMoveCommand treats it.
func TestGenerateReparentCommandRecordsOneMovePerTopLevelItem(t *testing.T) {
	v := NewView()
	root := NewRectItem()
	root.SetLocalCoord(true)

	box := NewRectItem()
	box.SetParent(root)
	child := NewRectItem()
	child.SetParent(box)

	target := NewRectItem()
	target.SetParent(root)

	sel := v.Selection()
	sel.Add(box)
	sel.Add(child)

	cmd := sel.GenerateReparentCommand(target)
	if cmd == nil {
		t.Fatal("command is nil, want one record for the container")
	}
	if got := cmd.Count(); got != 1 {
		t.Errorf("records = %d, want 1 (the child rides along inside the container)", got)
	}
}

// TestGenerateReparentCommandSkips covers the three refusals that must return
// no command at all: an item already owned by the target, a position-locked
// item (a drag leaves it behind, so it must not change owner either), and a
// target inside the moved item's own subtree, which would build a parent cycle
// that hangs every later traversal.
func TestGenerateReparentCommandSkips(t *testing.T) {
	newTree := func() (root, box, child IItem) {
		r := NewRectItem()
		r.SetLocalCoord(true)
		b := NewRectItem()
		b.SetParent(r)
		c := NewRectItem()
		c.SetParent(b)
		return r, b, c
	}

	t.Run("already in the target", func(t *testing.T) {
		v := NewView()
		_, box, child := newTree()
		v.Selection().Add(child)
		if cmd := v.Selection().GenerateReparentCommand(box); cmd != nil {
			t.Errorf("command = %d record(s), want nil for an item already in the target", cmd.Count())
		}
	})

	t.Run("position locked", func(t *testing.T) {
		v := NewView()
		root, _, child := newTree()
		child.SetLockPos(true)
		v.Selection().Add(child)
		if cmd := v.Selection().GenerateReparentCommand(root); cmd != nil {
			t.Errorf("command = %d record(s), want nil for a position-locked item", cmd.Count())
		}
	})

	t.Run("target inside the moved subtree", func(t *testing.T) {
		v := NewView()
		_, box, child := newTree()
		v.Selection().Add(box)
		if cmd := v.Selection().GenerateReparentCommand(child); cmd != nil {
			t.Errorf("command = %d record(s), want nil — that move would build a cycle", cmd.Count())
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		v := NewView()
		root, _, _ := newTree()
		if cmd := v.Selection().GenerateReparentCommand(root); cmd != nil {
			t.Errorf("command = %d record(s), want nil for an empty selection", cmd.Count())
		}
	})
}
