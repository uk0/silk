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

// TestReparentUndoRestoresSiblingOrder is the P1 regression: Break Layout on
// a VBox reverses its children on undo when the command only remembers the
// parent, not the index. scene [Other, VBox([C1,C2])]; lift C1,C2 to the
// scene and drop the VBox, then Undo. C1,C2 must come back to the VBox IN
// ORDER (not [C2,C1]) and at their original indices, and the VBox must
// return to its original slot in the scene.
func TestReparentUndoRestoresSiblingOrder(t *testing.T) {
	scene := NewRectItem()
	other := NewRectItem()
	other.SetParent(scene)
	vbox := NewRectItem()
	vbox.SetParent(scene)
	c1 := NewRectItem()
	c1.SetParent(vbox)
	c2 := NewRectItem()
	c2.SetParent(vbox)

	labels := map[IItem]string{
		other.Self(): "O", vbox.Self(): "V", c1.Self(): "1", c2.Self(): "2",
	}
	if got := orderOf(scene, labels); got != "OV" {
		t.Fatalf("setup scene = %q, want OV", got)
	}
	if got := orderOf(vbox, labels); got != "12" {
		t.Fatalf("setup vbox = %q, want 12", got)
	}

	// Break Layout: children up to the scene (recording their VBox indices
	// 0,1), then the now-empty VBox detaches.
	cmd := NewReparentCommand("Break Layout")
	cmd.Add(c1, vbox, scene)
	cmd.Add(c2, vbox, scene)
	cmd.Add(vbox, scene, nil)

	cmd.Redo()
	if got := orderOf(scene, labels); got != "O12" {
		t.Fatalf("after Redo scene = %q, want O12", got)
	}
	if vbox.Parent() != nil {
		t.Fatalf("after Redo vbox should be detached")
	}

	cmd.Undo()
	if got := orderOf(vbox, labels); got != "12" {
		t.Errorf("after Undo vbox = %q, want 12 (order, not reversed)", got)
	}
	if got := orderOf(scene, labels); got != "OV" {
		t.Errorf("after Undo scene = %q, want OV", got)
	}
	if c1.IndexInParent() != 0 {
		t.Errorf("c1 index = %d, want 0", c1.IndexInParent())
	}
	if c2.IndexInParent() != 1 {
		t.Errorf("c2 index = %d, want 1", c2.IndexInParent())
	}
	if vbox.IndexInParent() != 1 {
		t.Errorf("vbox index = %d, want 1", vbox.IndexInParent())
	}
}

// TestReparentUndoThreeChildrenOrder guards the general case a naive
// reverse-order (or record-order) undo gets wrong: with three children a
// simple reversal yields [1,3,2]. Restoring by increasing original index
// must reproduce [1,2,3].
func TestReparentUndoThreeChildrenOrder(t *testing.T) {
	scene := NewRectItem()
	vbox := NewRectItem()
	vbox.SetParent(scene)
	c1 := NewRectItem()
	c1.SetParent(vbox)
	c2 := NewRectItem()
	c2.SetParent(vbox)
	c3 := NewRectItem()
	c3.SetParent(vbox)

	labels := map[IItem]string{
		vbox.Self(): "V", c1.Self(): "1", c2.Self(): "2", c3.Self(): "3",
	}

	cmd := NewReparentCommand("Break Layout")
	cmd.Add(c1, vbox, scene)
	cmd.Add(c2, vbox, scene)
	cmd.Add(c3, vbox, scene)
	cmd.Add(vbox, scene, nil)

	cmd.Redo()
	cmd.Undo()

	if got := orderOf(vbox, labels); got != "123" {
		t.Errorf("after Undo vbox = %q, want 123", got)
	}
}

// TestReparentUndoOutOfOrderRecords proves the undo restores by original
// index, not by record order. "Lay Out" records selected items in
// canvas-position order, which can differ from their child index. Here the
// records are added in DECREASING index order (c,b,a); undo must still put
// the parent back to [A,B,C].
func TestReparentUndoOutOfOrderRecords(t *testing.T) {
	scene := NewRectItem()
	p := NewRectItem()
	p.SetParent(scene)
	a := NewRectItem()
	a.SetParent(p) // index 0
	b := NewRectItem()
	b.SetParent(p) // index 1
	c := NewRectItem()
	c.SetParent(p) // index 2
	container := NewRectItem()

	labels := map[IItem]string{a.Self(): "A", b.Self(): "B", c.Self(): "C"}

	cmd := NewReparentCommand("Lay Out")
	cmd.Add(container, nil, scene)
	cmd.Add(c, p, container) // fromIndex 2
	cmd.Add(b, p, container) // fromIndex 1
	cmd.Add(a, p, container) // fromIndex 0

	cmd.Redo()
	cmd.Undo()

	if got := orderOf(p, labels); got != "ABC" {
		t.Errorf("after Undo p = %q, want ABC (restore by index, not record order)", got)
	}
	if container.Parent() != nil {
		t.Errorf("after Undo container should be detached")
	}
}

// TestReparentUndoMiddleChild moves a middle sibling out and expects it back
// at index 1 on undo.
func TestReparentUndoMiddleChild(t *testing.T) {
	root := NewRectItem()
	parent := NewRectItem()
	parent.SetParent(root)
	elsewhere := NewRectItem()
	elsewhere.SetParent(root)
	x := NewRectItem()
	x.SetParent(parent)
	y := NewRectItem()
	y.SetParent(parent)
	z := NewRectItem()
	z.SetParent(parent)

	labels := map[IItem]string{x.Self(): "X", y.Self(): "Y", z.Self(): "Z"}

	cmd := NewReparentCommand("move")
	cmd.Add(y, parent, elsewhere) // fromIndex 1
	cmd.Redo()
	if got := orderOf(parent, labels); got != "XZ" {
		t.Fatalf("after Redo parent = %q, want XZ", got)
	}
	cmd.Undo()
	if got := orderOf(parent, labels); got != "XYZ" {
		t.Errorf("after Undo parent = %q, want XYZ", got)
	}
	if y.IndexInParent() != 1 {
		t.Errorf("y index = %d, want 1", y.IndexInParent())
	}
}

// TestReparentRedoUndoRedoIdempotent confirms sibling order is identical
// across a Redo→Undo→Redo cycle (and a final Undo restores the original).
func TestReparentRedoUndoRedoIdempotent(t *testing.T) {
	scene := NewRectItem()
	other := NewRectItem()
	other.SetParent(scene)
	vbox := NewRectItem()
	vbox.SetParent(scene)
	c1 := NewRectItem()
	c1.SetParent(vbox)
	c2 := NewRectItem()
	c2.SetParent(vbox)
	c3 := NewRectItem()
	c3.SetParent(vbox)

	labels := map[IItem]string{
		other.Self(): "O", vbox.Self(): "V",
		c1.Self(): "1", c2.Self(): "2", c3.Self(): "3",
	}

	cmd := NewReparentCommand("Break Layout")
	cmd.Add(c1, vbox, scene)
	cmd.Add(c2, vbox, scene)
	cmd.Add(c3, vbox, scene)
	cmd.Add(vbox, scene, nil)

	cmd.Redo()
	redo1 := orderOf(scene, labels)
	cmd.Undo()
	cmd.Redo()
	redo2 := orderOf(scene, labels)
	if redo1 != redo2 {
		t.Errorf("redo not idempotent: %q vs %q", redo1, redo2)
	}

	cmd.Undo()
	if got := orderOf(scene, labels); got != "OV" {
		t.Errorf("final Undo scene = %q, want OV", got)
	}
	if got := orderOf(vbox, labels); got != "123" {
		t.Errorf("final Undo vbox = %q, want 123", got)
	}
}

// TestSetParentAtPositions unit-tests the insertion primitive directly:
// head, middle, tail, the -1 append sentinel, out-of-range clamps, an empty
// parent, and detach.
func TestSetParentAtPositions(t *testing.T) {
	build := func() (*RectItem, map[IItem]string) {
		p := NewRectItem()
		a := NewRectItem()
		a.SetParent(p)
		b := NewRectItem()
		b.SetParent(p)
		c := NewRectItem()
		c.SetParent(p)
		return p, map[IItem]string{a.Self(): "A", b.Self(): "B", c.Self(): "C"}
	}

	cases := []struct {
		name  string
		index int
		want  string
	}{
		{"head", 0, "XABC"},
		{"middle", 2, "ABXC"},
		{"tail", 3, "ABCX"},
		{"append-sentinel", -1, "ABCX"},
		{"clamp-high", 999, "ABCX"},
		{"clamp-low", -5, "XABC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, labels := build()
			x := NewRectItem()
			labels[x.Self()] = "X"
			x.SetParentAt(p, tc.index)
			if got := orderOf(p, labels); got != tc.want {
				t.Errorf("SetParentAt(p, %d) = %q, want %q", tc.index, got, tc.want)
			}
		})
	}

	t.Run("empty-parent", func(t *testing.T) {
		p := NewRectItem()
		x := NewRectItem()
		x.SetParentAt(p, 7) // index irrelevant for an empty ring
		kids := p.Children()
		if len(kids) != 1 || kids[0] != x.Self() {
			t.Errorf("insert into empty parent failed: %v", kids)
		}
	})

	t.Run("detach", func(t *testing.T) {
		p, _ := build()
		b := p.Children()[1]
		b.SetParentAt(nil, 0)
		if b.Parent() != nil {
			t.Errorf("SetParentAt(nil, ...) should detach")
		}
		if got := len(p.Children()); got != 2 {
			t.Errorf("parent children after detach = %d, want 2", got)
		}
	})
}
