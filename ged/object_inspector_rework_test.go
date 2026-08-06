package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestParkInsideParentRescuesAWidgetTheClipWouldHide is the regression guard
// for a tree re-parent that made the widget disappear. FakeWidget has no local
// coordinates, so a child keeps its scene rect while Item.DrawAll clips it to
// the intersection with its new parent — and returns early when that is empty.
// A canvas drop cannot hit this (the pointer was already over the container);
// a tree drop has no pointer at all.
func TestParkInsideParentRescuesAWidgetTheClipWouldHide(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 10, 60, 40, 8) // below the box, no overlap
	btn.SetParent(box)

	parkInsideParent([]graph.IItem{btn}, box)

	if !rectsOverlap(btn, box) {
		t.Errorf("button at (%.1f,%.1f) still has no intersection with its parent box (%.1f,%.1f %.1fx%.1f); DrawAll clips it away",
			btn.X(), btn.Y(), box.X(), box.Y(), box.Width(), box.Height())
	}
}

// TestParkInsideParentLeavesAnOverlappingWidgetAlone keeps the rescue from
// becoming a layout stomp: a tree drop states intent about ownership, not about
// position, so a widget that is already visible must not be moved.
func TestParkInsideParentLeavesAnOverlappingWidgetAlone(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 20, 20, 30, 8)
	btn.SetParent(box)
	x, y := btn.X(), btn.Y()

	parkInsideParent([]graph.IItem{btn}, box)

	if btn.X() != x || btn.Y() != y {
		t.Errorf("an already-visible widget was moved from (%.1f,%.1f) to (%.1f,%.1f)", x, y, btn.X(), btn.Y())
	}
}

// TestParkInsideParentHonoursAPositionLock: a lock means the user asked for the
// position to stay put, which outranks the rescue.
func TestParkInsideParentHonoursAPositionLock(t *testing.T) {
	box := newTestFake(t, "gui.VBox", 10, 10, 60, 40)
	btn := newTestFake(t, "gui.Button", 10, 60, 40, 8)
	btn.SetParent(box)
	btn.SetLockPos(true)

	parkInsideParent([]graph.IItem{btn}, box)

	if btn.Y() != 60 {
		t.Errorf("a position-locked widget was moved to y=%.1f", btn.Y())
	}
}

// TestParkInsideParentIgnoresTheFormRoot — the root clips nothing, so parking
// against it would move widgets for no reason.
func TestParkInsideParentIgnoresTheFormRoot(t *testing.T) {
	btn := newTestFake(t, "gui.Button", 200, 200, 40, 8)
	parkInsideParent([]graph.IItem{btn}, nil)
	if btn.X() != 200 || btn.Y() != 200 {
		t.Errorf("re-parenting to the form root moved the widget to (%.1f,%.1f)", btn.X(), btn.Y())
	}
}

func rectsOverlap(a, b graph.IItem) bool {
	return a.X() < b.X()+b.Width() && a.X()+a.Width() > b.X() &&
		a.Y() < b.Y()+b.Height() && a.Y()+a.Height() > b.Y()
}

func newTestFake(t *testing.T, factory string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	it, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("NewFakeWidgetFromFactory(%q): %v", factory, err)
	}
	it.SetBounds(x, y, w, h)
	return it
}
