package graph

import "testing"

// TestGenerateMoveCommandCarriesSceneSpaceChildren: in a tree that keeps every
// item in scene coordinates — the designer's model, where no container turns on
// local coords — a container carries nothing on its own, so the command has to
// record the whole subtree at the same delta.
func TestGenerateMoveCommandCarriesSceneSpaceChildren(t *testing.T) {
	v := NewView()
	box := NewRectItem()
	box.SetBounds(10, 20, 60, 60)

	child := NewRectItem()
	child.SetBounds(15, 25, 10, 10)
	child.SetParent(box)

	grandchild := NewRectItem()
	grandchild.SetBounds(17, 27, 4, 4)
	grandchild.SetParent(child)

	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 3)
	if cmd == nil {
		t.Fatal("command is nil, want the container and its subtree")
	}
	cmd.Redo()

	if x, y := box.Pos(); x != 17 || y != 23 {
		t.Fatalf("container = (%v,%v), want (17,23)", x, y)
	}
	if x, y := child.Pos(); x != 22 || y != 28 {
		t.Errorf("child = (%v,%v), want (22,28) — it was left behind", x, y)
	}
	if x, y := grandchild.Pos(); x != 24 || y != 30 {
		t.Errorf("grandchild = (%v,%v), want (24,30) — the walk stopped one level down", x, y)
	}
}

// TestGenerateMoveCommandCarriesLocalCoordChildOnce: the coordinate boundary
// can sit partway down the tree, not only at the item that was selected. A
// local-coord container parented to a scene-space one keeps its own scene
// position, so nothing carries it and the walk has to record it — exactly once,
// and without descending: skipping it strands it and its whole subtree, while
// stepping into it would move its children a second time. The plain sibling
// after it is there to show the walk steps over the boundary rather than ending
// at it.
func TestGenerateMoveCommandCarriesLocalCoordChildOnce(t *testing.T) {
	v := NewView()
	box := NewRectItem()
	box.SetBounds(10, 20, 60, 60)

	panel := NewRectItem()
	panel.SetLocalCoord(true)
	panel.SetBounds(15, 25, 30, 30)
	panel.SetParent(box)

	inner := NewRectItem()
	inner.SetBounds(5, 5, 10, 10)
	inner.SetParent(panel)

	sibling := NewRectItem()
	sibling.SetBounds(50, 60, 10, 10)
	sibling.SetParent(box)

	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 3)
	if cmd == nil {
		t.Fatal("command is nil, want the container and its subtree")
	}
	cmd.Redo()

	if x, y := box.Pos(); x != 17 || y != 23 {
		t.Fatalf("container = (%v,%v), want (17,23)", x, y)
	}
	if x, y := panel.Pos(); x != 22 || y != 28 {
		t.Errorf("local-coord child = (%v,%v), want (22,28) — its parent does not carry it, so it was left behind", x, y)
	}
	if x, y := inner.Pos(); x != 5 || y != 5 {
		t.Errorf("child below the boundary = (%v,%v), want (5,5) — it is stored relative, so recording it too moves it twice", x, y)
	}
	if x, y := inner.MapToScene(inner.Pos()); x != 27 || y != 33 {
		t.Errorf("child below the boundary on screen = (%v,%v), want (27,33) — it rides along with the container it sits in", x, y)
	}
	if x, y := sibling.Pos(); x != 57 || y != 63 {
		t.Errorf("sibling after the boundary = (%v,%v), want (57,63) — the walk gave up at the local-coord child", x, y)
	}
}

// TestGenerateMoveCommandStopsAtLocalCoordBoundary: a local-coordinate item
// stores its children relative to itself, so they follow it for free. Recording
// them as well would apply the delta a second time and throw them out of the
// container they sit in.
func TestGenerateMoveCommandStopsAtLocalCoordBoundary(t *testing.T) {
	v := NewView()
	root := NewRectItem()
	root.SetLocalCoord(true)

	box := NewRectItem()
	box.SetLocalCoord(true)
	box.SetBounds(10, 20, 60, 60)
	box.SetParent(root)

	child := NewRectItem()
	child.SetBounds(5, 5, 10, 10)
	child.SetParent(box)

	v.Selection().Add(box)
	cmd := v.Selection().GenerateMoveCommand(7, 3)
	if cmd == nil {
		t.Fatal("command is nil, want the container")
	}
	cmd.Redo()

	if x, y := box.Pos(); x != 17 || y != 23 {
		t.Fatalf("container = (%v,%v), want (17,23)", x, y)
	}
	if x, y := child.Pos(); x != 5 || y != 5 {
		t.Errorf("child = (%v,%v), want (5,5) — its position is relative to the container, so moving it too moves it twice", x, y)
	}
	if x, y := child.MapToScene(child.Pos()); x != 22 || y != 28 {
		t.Errorf("child on screen = (%v,%v), want (22,28) — it must ride along with the container", x, y)
	}
}
