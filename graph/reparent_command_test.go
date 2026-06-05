package graph

import "testing"

// TestReparentCommandRedoUndo exercises the command in isolation: it
// models a "Lay Out" (attach a new container under the root, then move
// two existing children into it) and verifies Redo applies the whole
// move and Undo unwinds it exactly — children back under the root, the
// container detached.
func TestReparentCommandRedoUndo(t *testing.T) {
	root := NewRectItem()
	a := NewRectItem()
	a.SetParent(root)
	b := NewRectItem()
	b.SetParent(root)
	container := NewRectItem() // freshly created, no parent yet

	if got := len(root.Children()); got != 2 {
		t.Fatalf("setup: root children = %d, want 2", got)
	}

	cmd := NewReparentCommand("Lay Out")
	cmd.Add(container, nil, root) // container: nil -> root
	cmd.Add(a, root, container)   // a: root -> container
	cmd.Add(b, root, container)   // b: root -> container

	// Redo applies the move (Push normally calls this).
	cmd.Redo()
	if got := len(root.Children()); got != 1 {
		t.Fatalf("after Redo: root children = %d, want 1 (the container)", got)
	}
	if root.Children()[0] != IItem(container) {
		t.Errorf("after Redo: root's child should be the container")
	}
	if got := len(container.Children()); got != 2 {
		t.Fatalf("after Redo: container children = %d, want 2", got)
	}

	// Undo unwinds in reverse: a,b return to root, container detaches.
	cmd.Undo()
	if got := len(root.Children()); got != 2 {
		t.Fatalf("after Undo: root children = %d, want 2 (flat restored)", got)
	}
	if container.Parent() != nil {
		t.Errorf("after Undo: container should be detached (parent nil)")
	}
	if got := len(container.Children()); got != 0 {
		t.Errorf("after Undo: container should be empty, has %d", got)
	}

	// Redo again is idempotent with the first.
	cmd.Redo()
	if got := len(root.Children()); got != 1 {
		t.Errorf("after second Redo: root children = %d, want 1", got)
	}
}

// TestReparentCommandIllegalSequence: Redo on an already-applied command
// (or Undo on an un-applied one) panics — the same guard AddCommand uses
// to catch double-apply bugs in the undo stack.
func TestReparentCommandIllegalSequence(t *testing.T) {
	root := NewRectItem()
	a := NewRectItem()
	a.SetParent(root)
	cmd := NewReparentCommand("x")
	cmd.Add(a, root, nil)

	defer func() {
		if recover() == nil {
			t.Error("Undo before Redo should panic (illegal sequence)")
		}
	}()
	cmd.Undo() // not yet applied → must panic
}
