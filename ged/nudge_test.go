package ged

import (
	"testing"
)

// TestNudgeSelectionMovesItem: a 1mm nudge to the right shifts the
// selected item's X by exactly 1mm and pushes one MoveCommand onto
// the scene's UndoStack. Locks in the arrow-key contract that
// designer muscle-memory users depend on.
func TestNudgeSelectionMovesItem(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	fake.SetPos(10, 20)

	view.Selection().Add(fake)

	stackBefore := view.Scene().UndoStack().Count()
	view.nudgeSelection(1, 0)
	stackAfter := view.Scene().UndoStack().Count()

	if stackAfter != stackBefore+1 {
		t.Errorf("UndoCount delta = %d, want 1", stackAfter-stackBefore)
	}

	gotX, gotY := fake.Pos()
	if gotX != 11 || gotY != 20 {
		t.Errorf("after nudgeSelection(1, 0): pos = (%g, %g), want (11, 20)", gotX, gotY)
	}
}

// TestNudgeSelectionEmpty: nudging with no selection is a quiet
// no-op — UndoStack stays empty so the user doesn't accumulate
// "phantom" undo levels by tapping arrow keys on bare canvas.
func TestNudgeSelectionEmpty(t *testing.T) {
	view := NewGedView()
	stackBefore := view.Scene().UndoStack().Count()
	view.nudgeSelection(1, 0)
	if got := view.Scene().UndoStack().Count(); got != stackBefore {
		t.Errorf("nudge with empty selection pushed %d cmds, want 0", got-stackBefore)
	}
}

// TestNudgeSelectionUndoable: undoing a nudge restores the item's
// original position, proving the MoveCommand wired through correctly.
func TestNudgeSelectionUndoable(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	fake.SetPos(50, 60)

	view.Selection().Add(fake)
	view.nudgeSelection(0, 1)

	if _, y := fake.Pos(); y != 61 {
		t.Fatalf("after nudge: y = %g, want 61", y)
	}

	view.Scene().UndoStack().Undo()
	if x, y := fake.Pos(); x != 50 || y != 60 {
		t.Errorf("after undo: pos = (%g, %g), want (50, 60)", x, y)
	}
}
