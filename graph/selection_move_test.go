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
