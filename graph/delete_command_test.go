package graph

import "testing"

// buildDeleteScene returns a parent holding three children A,B,C in insertion
// order (index 0,1,2) plus a label lookup so orderOf renders the ring as a
// string like "ABC". Reuses the shared orderOf helper from zorder_test.go.
func buildDeleteScene() (parent, a, b, c *RectItem, labels map[IItem]string) {
	parent = NewRectItem()
	a = NewRectItem()
	a.SetParent(parent)
	b = NewRectItem()
	b.SetParent(parent)
	c = NewRectItem()
	c.SetParent(parent)
	labels = map[IItem]string{a.Self(): "A", b.Self(): "B", c.Self(): "C"}
	return
}

// TestDeleteCommandMiddleChildRestoresIndex: deleting the middle child B and
// undoing must put B back at index 1 (between A and C), not append it at the
// end — the whole point of capturing the sibling index.
func TestDeleteCommandMiddleChildRestoresIndex(t *testing.T) {
	parent, _, b, _, labels := buildDeleteScene()

	cmd := NewDeleteCommand("Delete")
	cmd.Add(b)

	cmd.Redo() // Push() would normally call this.
	if got := orderOf(parent, labels); got != "AC" {
		t.Fatalf("after Redo parent = %q, want AC", got)
	}
	if got := len(parent.Children()); got != 2 {
		t.Fatalf("after Redo child count = %d, want 2", got)
	}
	if b.Parent() != nil {
		t.Errorf("after Redo B should be detached (parent nil)")
	}

	cmd.Undo()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("after Undo parent = %q, want ABC (B back between A and C)", got)
	}
	if got := b.IndexInParent(); got != 1 {
		t.Errorf("B index = %d, want 1 (not appended at the end)", got)
	}
	if got := len(parent.Children()); got != 3 {
		t.Errorf("after Undo child count = %d, want 3", got)
	}
}

// TestDeleteCommandMultiRestoresOrder: deleting the two OUTER children A and C
// in one command leaves only B; undo must restore both at their original
// indices so the ring reads A,B,C again — the multi-select restore-order case.
func TestDeleteCommandMultiRestoresOrder(t *testing.T) {
	parent, a, _, c, labels := buildDeleteScene()

	cmd := NewDeleteCommand("Delete")
	cmd.Add(a)
	cmd.Add(c)

	cmd.Redo()
	if got := orderOf(parent, labels); got != "B" {
		t.Fatalf("after Redo parent = %q, want B", got)
	}
	if got := len(parent.Children()); got != 1 {
		t.Fatalf("after Redo child count = %d, want 1", got)
	}

	cmd.Undo()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("after Undo parent = %q, want ABC (both restored in order)", got)
	}
}

// TestDeleteCommandAllChildrenRestoreOrder: emptying a parent entirely and
// undoing must bring every child back in the original order. A naive
// reverse-with-original-index scheme reorders these; live-index capture plus
// reverse replay does not.
func TestDeleteCommandAllChildrenRestoreOrder(t *testing.T) {
	parent, a, b, c, labels := buildDeleteScene()

	cmd := NewDeleteCommand("Delete")
	cmd.Add(a)
	cmd.Add(b)
	cmd.Add(c)

	cmd.Redo()
	if got := len(parent.Children()); got != 0 {
		t.Fatalf("after Redo child count = %d, want 0 (parent emptied)", got)
	}

	cmd.Undo()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("after Undo parent = %q, want ABC (all restored in order)", got)
	}
}

// TestDeleteCommandRedoAfterUndoRoundTrip: a Redo→Undo→Redo cycle re-deletes
// to the identical state, and a final Undo restores everything — the undo
// stack drives exactly this sequence.
func TestDeleteCommandRedoAfterUndoRoundTrip(t *testing.T) {
	parent, a, _, c, labels := buildDeleteScene()

	cmd := NewDeleteCommand("Delete")
	cmd.Add(a)
	cmd.Add(c)

	cmd.Redo()
	afterFirst := orderOf(parent, labels)
	cmd.Undo()
	cmd.Redo()
	if got := orderOf(parent, labels); got != afterFirst {
		t.Errorf("redo not idempotent: %q vs first %q", got, afterFirst)
	}
	if got := orderOf(parent, labels); got != "B" {
		t.Errorf("after re-Redo parent = %q, want B", got)
	}

	cmd.Undo()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("after final Undo parent = %q, want ABC", got)
	}
}

// TestDeleteCommandIllegalSequence: Undo before Redo panics, matching the
// isUndo guard AddCommand/ReparentCommand use to catch double-apply bugs.
func TestDeleteCommandIllegalSequence(t *testing.T) {
	parent := NewRectItem()
	a := NewRectItem()
	a.SetParent(parent)

	cmd := NewDeleteCommand("Delete")
	cmd.Add(a)

	defer func() {
		if recover() == nil {
			t.Error("Undo before Redo should panic (illegal sequence)")
		}
	}()
	cmd.Undo()
}
