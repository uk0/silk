package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestSameSizeSelectionUsesLastSelectedAsReference: 相同宽度 copies the width of
// the widget selected LAST onto every other selected widget, leaving positions
// and heights alone, and lands on the UndoStack as exactly one step so a single
// Ctrl+Z restores the whole selection.
func TestSameSizeSelectionUsesLastSelectedAsReference(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 4)
	b := addFakeAt(t, scene, "b", 20, 10, 15, 6)
	ref := addFakeAt(t, scene, "ref", 50, 50, 30, 8)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(ref)

	before := view.Scene().UndoStack().Count()
	view.SameSizeSelection(graph.SameSizeWidth)

	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Fatalf("相同宽度 pushed %d undo commands, want 1", got)
	}
	if x, y, w, h := a.Bounds(); x != 0 || y != 0 || w != 30 || h != 4 {
		t.Errorf("a after 相同宽度 = (%g, %g, %g, %g), want (0, 0, 30, 4)", x, y, w, h)
	}
	if x, y, w, h := b.Bounds(); x != 20 || y != 10 || w != 30 || h != 6 {
		t.Errorf("b after 相同宽度 = (%g, %g, %g, %g), want (20, 10, 30, 6)", x, y, w, h)
	}
	if x, y, w, h := ref.Bounds(); x != 50 || y != 50 || w != 30 || h != 8 {
		t.Errorf("the reference moved: (%g, %g, %g, %g), want (50, 50, 30, 8)", x, y, w, h)
	}

	view.Scene().UndoStack().Undo()
	if _, _, w, _ := a.Bounds(); w != 10 {
		t.Errorf("a width after undo = %g, want 10", w)
	}
	if _, _, w, _ := b.Bounds(); w != 15 {
		t.Errorf("b width after undo = %g, want 15", w)
	}
}

// TestSameSizeSelectionHeightAndBoth keeps the two axes from being transposed
// by a future edit to the mode mapping: 相同高度 must touch height only, 相同大小
// both extents.
func TestSameSizeSelectionHeightAndBoth(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 4)
	ref := addFakeAt(t, scene, "ref", 50, 50, 30, 8)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(ref)

	view.SameSizeSelection(graph.SameSizeHeight)
	if _, _, w, h := a.Bounds(); w != 10 || h != 8 {
		t.Errorf("a after 相同高度 = (%g, %g), want (10, 8)", w, h)
	}

	a.SetBounds(0, 0, 10, 4)
	view.SameSizeSelection(graph.SameSizeBoth)
	if x, y, w, h := a.Bounds(); x != 0 || y != 0 || w != 30 || h != 8 {
		t.Errorf("a after 相同大小 = (%g, %g, %g, %g), want (0, 0, 30, 8)", x, y, w, h)
	}
}

// TestSameSizeSelectionNilCases: each of these must push nothing, so a menu
// press against a selection with nothing to do does not pile up empty undo
// levels the user then has to Ctrl+Z through.
func TestSameSizeSelectionNilCases(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	if cmd := view.Selection().GenerateSameSizeCommand(graph.SameSizeBoth, minWidgetSize); cmd != nil {
		t.Error("empty selection produced a command")
	}

	only := addFakeAt(t, scene, "only", 0, 0, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(only)
	if cmd := view.Selection().GenerateSameSizeCommand(graph.SameSizeBoth, minWidgetSize); cmd != nil {
		t.Error("a single-item selection produced a command")
	}

	twin := addFakeAt(t, scene, "twin", 40, 40, 10, 4)
	view.Selection().Add(twin)
	if cmd := view.Selection().GenerateSameSizeCommand(graph.SameSizeBoth, minWidgetSize); cmd != nil {
		t.Error("two already equally sized widgets produced a command")
	}
}

// TestSameSizeSelectionSkipsSizeLocked: a size-locked widget must come through
// untouched, exactly as it does on the Ctrl+arrow path — locking a widget's
// size has to hold against every resize route, not just the keyboard one.
func TestSameSizeSelectionSkipsSizeLocked(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	locked := addFakeAt(t, scene, "locked", 10, 10, 20, 8)
	locked.SetLockSize(true)
	ref := addFakeAt(t, scene, "ref", 50, 50, 30, 12)

	view.Selection().Clear()
	view.Selection().Add(locked)
	view.Selection().Add(ref)

	before := view.Scene().UndoStack().Count()
	view.SameSizeSelection(graph.SameSizeBoth)

	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("a same-size press over a size-locked selection pushed %d commands, want 0", got)
	}
	if _, _, w, h := locked.Bounds(); w != 20 || h != 8 {
		t.Errorf("size-locked widget resized to (%g, %g), want (20, 8)", w, h)
	}
}

// TestSameSizeSelectionLeavesSubFloorWidgetsAlone: widgets can already be
// smaller than minWidgetSize (the property sheet accepts any size, and a design
// file can carry one). Clamping the target extent up to the floor would make
// 相同大小 silently inflate a whole selection of tiny widgets, so the floor must
// only ever lower an extent.
func TestSameSizeSelectionLeavesSubFloorWidgetsAlone(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	tiny := addFakeAt(t, scene, "tiny", 10, 10, 0.5, 0.5)
	ref := addFakeAt(t, scene, "ref", 50, 50, 0.5, 0.5)

	view.Selection().Clear()
	view.Selection().Add(tiny)
	view.Selection().Add(ref)

	before := view.Scene().UndoStack().Count()
	view.SameSizeSelection(graph.SameSizeBoth)

	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("same-size over two sub-floor widgets pushed %d commands, want 0", got)
	}
	if x, y, w, h := tiny.Bounds(); x != 10 || y != 10 || w != 0.5 || h != 0.5 {
		t.Errorf("sub-floor widget = (%g, %g, %g, %g), want (10, 10, 0.5, 0.5)", x, y, w, h)
	}
}

// TestSelectionZorderWrappers: the 排列 menu reaches Z-order through the same
// reorderSelection the canvas context menu uses, so both routes must restack
// identically and leave one undoable step behind. A second implementation in
// package main would drift from the index-snapshot undo this one carries.
func TestSelectionZorderWrappers(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	addFake(t, scene, "c")

	view.Selection().Clear()
	view.Selection().Add(a)
	before := view.Scene().UndoStack().Count()
	view.BringSelectionToFront()

	if got := sceneOrder(scene); !eqStrings(got, []string{"b", "c", "a"}) {
		t.Fatalf("after BringSelectionToFront(a): %v, want [b c a]", got)
	}
	if got := view.Scene().UndoStack().Count() - before; got != 1 {
		t.Fatalf("BringSelectionToFront pushed %d undo commands, want 1", got)
	}
	view.Scene().UndoStack().Undo()
	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("after undo: %v, want [a b c]", got)
	}

	view.Selection().Clear()
	view.Selection().Add(b)
	view.SendSelectionToBack()
	if got := sceneOrder(scene); !eqStrings(got, []string{"b", "a", "c"}) {
		t.Errorf("after SendSelectionToBack(b): %v, want [b a c]", got)
	}
}
