package ged

import (
	"silk/graph"
	"testing"
)

// TestAlignSelectionUndoRedo confirms that alignSelection pushes a single
// undoable MoveCommand: every selected widget snaps to the min-left on
// AlignLeft, UndoStack().Undo() restores the originals exactly, and
// Redo() re-applies the alignment. Mirrors the nudge/MoveCommand round-
// trip — align now lives on the same UndoStack as drag-moves.
func TestAlignSelectionUndoRedo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 4)
	b := addFakeAt(t, scene, "b", 20, 10, 30, 8)
	c := addFakeAt(t, scene, "c", 100, 50, 20, 6)

	// Snapshot starting positions for the round-trip assertion.
	type xy struct{ x, y float64 }
	start := map[*FakeWidget]xy{
		a: {0, 0}, b: {20, 10}, c: {100, 50},
	}

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.alignSelection(AlignLeft)

	// All widgets aligned to the min-left = 0; Y untouched.
	for w, p := range start {
		gotX, gotY := w.Pos()
		if gotX != 0 || gotY != p.y {
			t.Errorf("%s after AlignLeft = (%g,%g), want (0,%g)", w.WidgetName(), gotX, gotY, p.y)
		}
	}

	// Undo restores the originals exactly.
	scene.UndoStack().Undo()
	for w, p := range start {
		gotX, gotY := w.Pos()
		if gotX != p.x || gotY != p.y {
			t.Errorf("%s after Undo = (%g,%g), want (%g,%g)", w.WidgetName(), gotX, gotY, p.x, p.y)
		}
	}

	// Redo re-applies the alignment.
	scene.UndoStack().Redo()
	for w, p := range start {
		gotX, gotY := w.Pos()
		if gotX != 0 || gotY != p.y {
			t.Errorf("%s after Redo = (%g,%g), want (0,%g)", w.WidgetName(), gotX, gotY, p.y)
		}
	}
}

// TestReorderSelectionUndoRedo confirms that reorderSelection pushes an
// undoable zorderCommand: BringToFront on a middle child reorders the
// scene children, Undo restores the original stacking, Redo brings it
// back. Sibling-position only, no parent change.
func TestReorderSelectionUndoRedo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	addFake(t, scene, "c")

	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("initial order = %v, want [a b c]", got)
	}

	view.Selection().Clear()
	view.Selection().Add(b)
	view.reorderSelection(graph.IItem.BringToFront)

	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "c", "b"}) {
		t.Fatalf("after reorderSelection BringToFront(b): %v, want [a c b]", got)
	}

	scene.UndoStack().Undo()
	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Errorf("after Undo: %v, want [a b c]", got)
	}

	scene.UndoStack().Redo()
	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "c", "b"}) {
		t.Errorf("after Redo: %v, want [a c b]", got)
	}
}

// TestZorderCommandInverseRaiseLower locks in the Raise↔Lower inverse
// pairing on the zorderCommand struct directly. Built in the pre-apply
// state with op=Raise/inverse=Lower; Redo raises the middle child to
// the head, Undo lowers it back. Single-record so reverse-order Undo
// doesn't matter — the next test covers ordering.
func TestZorderCommandInverseRaiseLower(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	addFake(t, scene, "c")
	// a b c

	cmd := newZorderCommand("Raise")
	cmd.Add(b, graph.IItem.Raise, graph.IItem.Lower)
	cmd.Redo()

	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "c", "b"}) {
		t.Fatalf("after Redo (Raise b): %v, want [a c b]", got)
	}

	cmd.Undo()
	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Errorf("after Undo (Lower b): %v, want [a b c]", got)
	}
}

// TestZorderCommandInverseFrontBack locks in the BringToFront↔SendToBack
// inverse pairing. Mirrors the Raise/Lower test but for the absolute-
// position ops.
func TestZorderCommandInverseFrontBack(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	addFake(t, scene, "b")
	addFake(t, scene, "c")
	// a b c

	cmd := newZorderCommand("Bring to Front")
	cmd.Add(a, graph.IItem.BringToFront, graph.IItem.SendToBack)
	cmd.Redo()

	if got := sceneOrder(scene); !eqStrings(got, []string{"b", "c", "a"}) {
		t.Fatalf("after Redo (BringToFront a): %v, want [b c a]", got)
	}

	cmd.Undo()
	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Errorf("after Undo (SendToBack a): %v, want [a b c]", got)
	}
}

// TestZorderCommandRedoUndoGuards mirrors graph.ReparentCommand's
// invariant: Redo on a post-Redo command panics, Undo on a pre-Redo
// command panics. The Push() contract relies on this to catch double-
// applies in tests / debug builds.
func TestZorderCommandRedoUndoGuards(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := addFake(t, scene, "a")

	t.Run("double Redo panics", func(t *testing.T) {
		cmd := newZorderCommand("Raise")
		cmd.Add(a, graph.IItem.Raise, graph.IItem.Lower)
		cmd.Redo()
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on double Redo")
			}
		}()
		cmd.Redo()
	})

	t.Run("Undo before Redo panics", func(t *testing.T) {
		cmd := newZorderCommand("Raise")
		cmd.Add(a, graph.IItem.Raise, graph.IItem.Lower)
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on Undo before Redo")
			}
		}()
		cmd.Undo()
	})
}
