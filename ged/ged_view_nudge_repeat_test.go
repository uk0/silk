package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// TestHeldArrowKeyIsOneUndoStep: a held arrow key auto-repeats, and OnKeyDown
// used to ignore its repeat flag entirely — one press plus 20 repeats pushed
// 21 separate MoveCommands, burying whatever the designer did before it 21
// undo levels deep. The whole burst must land as a single undo step that still
// travels the full 21 mm, and one undo must return the widget to where the
// burst started.
func TestHeldArrowKeyIsOneUndoStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	before := view.Scene().UndoStack().Count()
	view.OnKeyDown(gui.KeyRight, false)
	for i := 0; i < 20; i++ {
		view.OnKeyDown(gui.KeyRight, true)
	}

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Errorf("1 press + 20 repeats pushed %d undo commands, want 1", got)
	}
	if x, y := fake.Pos(); x != 61 || y != 30 {
		t.Errorf("after the burst: pos = (%g, %g), want (61, 30)", x, y)
	}

	view.Scene().UndoStack().Undo()
	if x, y := fake.Pos(); x != 40 || y != 30 {
		t.Errorf("after one undo: pos = (%g, %g), want (40, 30) — the whole burst reverses at once", x, y)
	}

	// Redo has to replay the whole burst. A merge that rewrote the surviving
	// command's records instead of leaving them alone would replay one step.
	view.Scene().UndoStack().Redo()
	if x, y := fake.Pos(); x != 61 || y != 30 {
		t.Errorf("after redo: pos = (%g, %g), want (61, 30)", x, y)
	}
}

// TestSeparateArrowPressesAreSeparateUndoSteps: releasing the key and pressing
// again is a new gesture, so two discrete taps must stay two undo steps. Guards
// the coalescing fix against over-reaching and swallowing deliberate presses
// into the previous one.
func TestSeparateArrowPressesAreSeparateUndoSteps(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	before := view.Scene().UndoStack().Count()
	view.OnKeyDown(gui.KeyRight, false)
	view.OnKeyDown(gui.KeyRight, false)

	if got := view.Scene().UndoStack().Count() - before; got != 2 {
		t.Errorf("two discrete presses pushed %d undo commands, want 2", got)
	}
	if x, _ := fake.Pos(); x != 42 {
		t.Fatalf("after two presses: x = %g, want 42", x)
	}

	view.Scene().UndoStack().Undo()
	if x, _ := fake.Pos(); x != 41 {
		t.Errorf("after one undo: x = %g, want 41 — only the second press should reverse", x)
	}
}

// TestArrowBurstDoesNotMergeIntoEarlierMove: a mouse drag pushes an untagged
// MoveCommand. A following arrow burst carries a gesture token, but the drag
// below it carries none, so the burst must open its own undo step instead of
// being absorbed into the drag — otherwise undoing the arrows would silently
// take the drag back too.
func TestArrowBurstDoesNotMergeIntoEarlierMove(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	// Stand in for a drag: the same command MovePart commits, with no token.
	drag := view.Selection().GenerateMoveCommand(5, 0)
	if drag == nil {
		t.Fatal("GenerateMoveCommand returned nil for a non-empty selection")
	}
	view.Scene().UndoStack().Push(drag)
	if x, _ := fake.Pos(); x != 45 {
		t.Fatalf("after the stand-in drag: x = %g, want 45", x)
	}

	before := view.Scene().UndoStack().Count()
	view.OnKeyDown(gui.KeyRight, false)
	view.OnKeyDown(gui.KeyRight, true)
	view.OnKeyDown(gui.KeyRight, true)

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Errorf("the arrow burst pushed %d undo commands, want 1", got)
	}

	view.Scene().UndoStack().Undo()
	if x, _ := fake.Pos(); x != 45 {
		t.Errorf("after undoing the burst: x = %g, want 45 — the drag must survive", x)
	}
}
