package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// TestEscapeClearsSelection: pressing ESC on a non-empty selection
// removes every selected item from the selection set. The ESC handler
// goes through Selection().Clear() so any decorations attached to
// the selected items are dropped at the same time.
func TestEscapeClearsSelection(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	view.Selection().Add(fake)

	if view.Selection().IsEmpty() {
		t.Fatal("selection should be non-empty before ESC")
	}

	view.OnKeyDown(gui.KeyEsc, false)

	if !view.Selection().IsEmpty() {
		t.Errorf("after ESC: selection still has %d items, want 0",
			len(view.Selection().ItemList()))
	}
}

// TestEscapeOnEmptySelectionIsNoOp: ESC with nothing selected
// shouldn't trigger an Update or any state change. The shortcut
// still consumes the key but the selection-already-empty branch
// short-circuits.
func TestEscapeOnEmptySelectionIsNoOp(t *testing.T) {
	view := NewGedView()
	if !view.Selection().IsEmpty() {
		t.Fatal("fresh view should have empty selection")
	}
	view.OnKeyDown(gui.KeyEsc, false)
	if !view.Selection().IsEmpty() {
		t.Errorf("ESC mutated empty selection")
	}
}
