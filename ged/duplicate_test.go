package ged

import (
	"testing"
)

// TestDuplicateAddsNewItemOffsetFromOriginal: the Cmd+D handler
// (CopySelected → PasteItems) duplicates the selected widget and
// places the copy at a small offset so it doesn't sit directly on
// top of the original. After the duplicate the scene has two
// children and the selection has moved to the copy.
func TestDuplicateAddsNewItemOffsetFromOriginal(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create fake: %v", err)
	}
	fake.SetParent(scene)
	fake.SetPos(10, 20)
	fake.SetSize(80, 40)

	view.Selection().Add(fake)

	view.CopySelected()
	view.PasteItems()

	children := scene.Children()
	if len(children) != 2 {
		t.Fatalf("after duplicate: scene has %d children, want 2", len(children))
	}

	// The selection should now hold the duplicate (not the original).
	selItems := view.Selection().ItemList()
	if len(selItems) != 1 {
		t.Fatalf("after duplicate: %d selected, want 1", len(selItems))
	}
	dup := selItems[0]
	if dup == fake {
		t.Errorf("duplicate selection should not be the original item")
	}

	// Duplicate sits offset from the original (PasteItems adds (2, 2)
	// before snap-to-grid). Exact offset isn't pinned — the snap may
	// round it — but it must not coincide with the original.
	dx, dy := dup.Pos()
	ox, oy := fake.Pos()
	if dx == ox && dy == oy {
		t.Errorf("duplicate is at the original's position (%g, %g)", dx, dy)
	}
}

// TestDuplicateSelectionOffsetsByOneGridPitch drives the Ctrl+D entry point
// itself (the keystroke and the 编辑 menu share it): the copy lands one grid
// pitch down-right of the original and the selection follows it, so a second
// Ctrl+D walks down the canvas instead of stacking copies in place.
func TestDuplicateSelectionOffsetsByOneGridPitch(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)

	fake := addFakeAt(t, scene, "btn", 10, 20, 30, 12)
	view.Selection().Add(fake)

	view.DuplicateSelection()

	if n := len(scene.Children()); n != 2 {
		t.Fatalf("after duplicate: scene has %d children, want 2", n)
	}
	sel := view.Selection().ItemList()
	if len(sel) != 1 {
		t.Fatalf("after duplicate: %d selected, want 1 (the copy)", len(sel))
	}
	if sel[0] == fake {
		t.Fatal("selection stayed on the original")
	}
	if x, y := sel[0].Pos(); x != 15 || y != 25 {
		t.Errorf("copy at (%g, %g), want (15, 25) — one 5 mm pitch off the original", x, y)
	}
}
