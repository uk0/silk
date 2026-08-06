package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// Ctrl+arrow cannot be driven through OnKeyDown here: the modifier comes from
// gui.IsKeyDown, which needs a live window and reports false in tests. These
// cases therefore call beginGesture + resizeSelection directly — exactly the
// pair OnKeyDown invokes once it has decided the Ctrl+arrow branch applies —
// the same workaround ged_view_nudge_test.go uses for Shift.

// TestResizeSelectionGrowsFromTopLeft: Ctrl+Right must change only the width,
// leaving X and Y pinned. A resize that also shifted the corner would drift the
// widget across the canvas as the key repeated.
func TestResizeSelectionGrowsFromTopLeft(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	dw, dh, handled := nudgeDelta(gui.KeyRight, false, 1, view.coarseNudgeStep())
	if !handled || dw != 1 || dh != 0 {
		t.Fatalf("nudgeDelta(Right) = (%g, %g, %v), want (1, 0, true)", dw, dh, handled)
	}
	view.resizeSelection(dw, dh)

	x, y, w, h := fake.Bounds()
	if x != 40 || y != 30 || w != 11 || h != 4 {
		t.Errorf("after Ctrl+Right: bounds = (%g, %g, %g, %g), want (40, 30, 11, 4)", x, y, w, h)
	}

	view.Scene().UndoStack().Undo()
	x, y, w, h = fake.Bounds()
	if x != 40 || y != 30 || w != 10 || h != 4 {
		t.Errorf("after undo: bounds = (%g, %g, %g, %g), want (40, 30, 10, 4)", x, y, w, h)
	}
}

// TestResizeSelectionGrowsHeight: Ctrl+Down grows height only, so the two axes
// cannot be transposed by a future edit to the delta mapping. The coarse step
// is the scene's grid pitch, set here to a non-default value so a resize wired
// back to a fixed constant cannot pass.
func TestResizeSelectionGrowsHeight(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(8)

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	step := view.coarseNudgeStep()
	dw, dh, _ := nudgeDelta(gui.KeyDown, true, 1, step)
	view.resizeSelection(dw, dh)

	_, _, w, h := fake.Bounds()
	if w != 10 || h != 4+step {
		t.Errorf("after Shift+Ctrl+Down: size = (%g, %g), want (10, %g)", w, h, 4+step)
	}
}

// TestResizeSelectionFloorsAtMinWidgetSize: a keyboard resize repeats while the
// key is held, so shrinking must stop at minWidgetSize. Without the floor the
// width walks through zero into a negative extent, which NormalizeCopy flips
// into a mirrored rectangle sitting left of the widget's own left edge.
func TestResizeSelectionFloorsAtMinWidgetSize(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 5, 5)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.beginGesture(false)
	for i := 0; i < 50; i++ {
		view.resizeSelection(-1, -1)
		view.beginGesture(true)
	}

	x, y, w, h := fake.Bounds()
	if w != minWidgetSize || h != minWidgetSize {
		t.Errorf("after 50 shrink steps: size = (%g, %g), want (%g, %g)",
			w, h, minWidgetSize, minWidgetSize)
	}
	if x != 40 || y != 30 {
		t.Errorf("shrinking moved the corner to (%g, %g), want (40, 30)", x, y)
	}
}

// TestResizeSelectionLeavesSubFloorWidgetAlone: a widget can already be smaller
// than minWidgetSize, and one Ctrl+Left used to clamp it up to the floor — the
// shrink key made it bigger. Shrinking below the floor is a no-op instead.
func TestResizeSelectionLeavesSubFloorWidgetAlone(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "tiny", 40, 30, 0.5, 0.5)
	view.Selection().Clear()
	view.Selection().Add(fake)

	before := view.Scene().UndoStack().Count()
	view.beginGesture(false)
	view.resizeSelection(-1, -1)

	if x, y, w, h := fake.Bounds(); x != 40 || y != 30 || w != 0.5 || h != 0.5 {
		t.Errorf("Ctrl+Left/Up on a 0.5x0.5 widget gave (%g, %g, %g, %g), want (40, 30, 0.5, 0.5)",
			x, y, w, h)
	}
	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("a shrink that changed nothing pushed %d undo commands, want 0", got)
	}

	// Growing out of the sub-floor range must still work.
	view.resizeSelection(1, 1)
	if _, _, w, h := fake.Bounds(); w != 1.5 || h != 1.5 {
		t.Errorf("Ctrl+Right/Down on the same widget gave (%g, %g), want (1.5, 1.5)", w, h)
	}
}

// TestResizeSelectionSkipsSizeLocked: a size-locked widget must come through a
// resize untouched, and a selection holding nothing else must not push an undo
// command at all — otherwise Ctrl+arrow on a locked widget accumulates undo
// levels that do nothing.
func TestResizeSelectionSkipsSizeLocked(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	locked := addFakeAt(t, scene, "locked", 10, 10, 20, 8)
	locked.SetLockSize(true)
	view.Selection().Clear()
	view.Selection().Add(locked)

	before := view.Scene().UndoStack().Count()
	view.resizeSelection(5, 5)

	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("resizing a size-locked selection pushed %d commands, want 0", got)
	}
	if _, _, w, h := locked.Bounds(); w != 20 || h != 8 {
		t.Errorf("size-locked widget resized to (%g, %g), want (20, 8)", w, h)
	}
}

// TestResizeKeepsPositionLockedWidget: the drag-handle path in ResizeDecor
// skips IsLockPos items because it can reposition them. A keyboard resize
// anchors at the top-left and never moves anything, so a position lock must not
// block it — locking where a widget sits should not also freeze its size.
func TestResizeKeepsPositionLockedWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	posLocked := addFakeAt(t, scene, "pinned", 10, 10, 20, 20)
	posLocked.SetLockPos(true)
	view.Selection().Clear()
	view.Selection().Add(posLocked)

	view.resizeSelection(5, 5)

	x, y, w, h := posLocked.Bounds()
	if x != 10 || y != 10 || w != 25 || h != 25 {
		t.Errorf("position-locked widget bounds = (%g, %g, %g, %g), want (10, 10, 25, 25)", x, y, w, h)
	}
}

// TestGenerateResizeCommandNilCases: each of these must yield nil so
// resizeSelection forwards a quiet no-op rather than pushing a command that
// does nothing — otherwise Ctrl+arrow against an immovable selection quietly
// piles up empty undo levels.
func TestGenerateResizeCommandNilCases(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	if cmd := view.Selection().GenerateResizeCommand(5, 5, minWidgetSize); cmd != nil {
		t.Error("empty selection produced a command")
	}

	fake := addFakeAt(t, scene, "btn", 0, 0, minWidgetSize, minWidgetSize)
	view.Selection().Clear()
	view.Selection().Add(fake)

	if cmd := view.Selection().GenerateResizeCommand(0, 0, minWidgetSize); cmd != nil {
		t.Error("a zero delta produced a command")
	}
	if cmd := view.Selection().GenerateResizeCommand(-5, -5, minWidgetSize); cmd != nil {
		t.Error("shrinking a widget already at the floor produced a command")
	}
}

// TestHeldResizeKeyIsOneUndoStep: the same auto-repeat flood that afflicted the
// arrow keys applies to Ctrl+arrow, so a held resize must also collapse into a
// single undo step that reverses the whole burst at once.
func TestHeldResizeKeyIsOneUndoStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake := addFakeAt(t, scene, "btn", 40, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	before := view.Scene().UndoStack().Count()
	view.beginGesture(false)
	view.resizeSelection(1, 0)
	for i := 0; i < 9; i++ {
		view.beginGesture(true)
		view.resizeSelection(1, 0)
	}

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Errorf("1 press + 9 repeats pushed %d undo commands, want 1", got)
	}
	if _, _, w, _ := fake.Bounds(); w != 20 {
		t.Errorf("after the burst: width = %g, want 20", w)
	}

	view.Scene().UndoStack().Undo()
	if _, _, w, _ := fake.Bounds(); w != 10 {
		t.Errorf("after one undo: width = %g, want 10 — the whole burst reverses at once", w)
	}

	view.Scene().UndoStack().Redo()
	if _, _, w, _ := fake.Bounds(); w != 20 {
		t.Errorf("after redo: width = %g, want 20 — redo replays the whole burst", w)
	}
}

// TestHeldShrinkOverMixedSizesIsOneUndoStep: coalescing must survive a
// selection whose widgets reach the floor at different times. The small widget
// bottoms out on the first repeat and leaves the command's record list from
// there on, which used to break the merge and split one held key into two undo
// steps — the second of which reversed only part of the burst.
func TestHeldShrinkOverMixedSizesIsOneUndoStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	big := addFakeAt(t, scene, "big", 0, 0, 100, 10)
	small := addFakeAt(t, scene, "small", 0, 40, 2, 10)
	view.Selection().Clear()
	view.Selection().Add(big)
	view.Selection().Add(small)

	before := view.Scene().UndoStack().Count()
	view.beginGesture(false)
	view.resizeSelection(-1, 0)
	for i := 0; i < 9; i++ {
		view.beginGesture(true)
		view.resizeSelection(-1, 0)
	}

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Errorf("1 press + 9 repeats pushed %d undo commands, want 1", got)
	}
	if _, _, w, _ := big.Bounds(); w != 90 {
		t.Errorf("after the burst: big width = %g, want 90", w)
	}
	if _, _, w, _ := small.Bounds(); w != minWidgetSize {
		t.Errorf("after the burst: small width = %g, want %g", w, minWidgetSize)
	}

	view.Scene().UndoStack().Undo()
	bw := widthOf(big)
	sw := widthOf(small)
	if bw != 100 || sw != 2 {
		t.Errorf("after one undo: widths = (%g, %g), want (100, 2) — one undo reverses the whole burst", bw, sw)
	}
}

func widthOf(fw *FakeWidget) float64 {
	_, _, w, _ := fw.Bounds()
	return w
}
