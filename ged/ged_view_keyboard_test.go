package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// holdKeys swaps the modifier probe so a test can press a chord, and restores
// it when the test ends. Without it every Ctrl/Alt/Shift binding in OnKeyDown is
// unreachable headlessly and only the bare keys can be exercised.
func holdKeys(t *testing.T, held ...int) {
	t.Helper()
	down := make(map[int]bool, len(held))
	for _, k := range held {
		down[k] = true
	}
	prev := keyDown
	keyDown = func(k int) bool { return down[k] }
	t.Cleanup(func() { keyDown = prev })
}

// answerRename swaps the name prompt for a canned answer and reports how many
// times it was opened.
func answerRename(t *testing.T, answer string, ok bool) *int {
	t.Helper()
	calls := 0
	prev := showInputBox
	showInputBox = func(parent gui.IWidget, icon paint.Icon, title, label, defText string) (string, bool) {
		calls++
		return answer, ok
	}
	t.Cleanup(func() { showInputBox = prev })
	return &calls
}

// TestF2RenamesSelectedWidget: F2 is the rename key in every file manager and
// IDE, and renaming is the one edit a keyboard user cannot otherwise reach —
// the name field lives behind a right-click context menu. F2 reuses that same
// prompt rather than growing an in-canvas text editor.
func TestF2RenamesSelectedWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fw := addFakeAt(t, scene, "button1", 10, 10, 30, 8)
	view.Selection().Add(fw)

	calls := answerRename(t, "okButton", true)
	view.OnKeyDown(gui.KeyF2, false)

	if *calls != 1 {
		t.Fatalf("F2 opened the name prompt %d times, want 1", *calls)
	}
	if got := fw.WidgetName(); got != "okButton" {
		t.Errorf("widget name = %q, want okButton", got)
	}
}

// TestF2CancelledKeepsName: dismissing the prompt must leave the widget alone.
func TestF2CancelledKeepsName(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fw := addFakeAt(t, scene, "button1", 10, 10, 30, 8)
	view.Selection().Add(fw)

	answerRename(t, "ignored", false)
	view.OnKeyDown(gui.KeyF2, false)

	if got := fw.WidgetName(); got != "button1" {
		t.Errorf("cancelled rename changed the name to %q", got)
	}
}

// TestF2NeedsExactlyOneWidget: a multi-selection has no single name to edit and
// an empty one has nothing to rename, so F2 must not open a prompt at all —
// a prompt that renamed only the first of five selected widgets would be worse
// than no binding.
func TestF2NeedsExactlyOneWidget(t *testing.T) {
	cases := []struct {
		name  string
		count int
	}{
		{"empty selection", 0},
		{"multi selection", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := NewGedView()
			scene := view.GedScene()
			for i := 0; i < tc.count; i++ {
				view.Selection().Add(addFakeAt(t, scene, "w", 10, float64(10+i*20), 30, 8))
			}

			calls := answerRename(t, "x", true)
			view.OnKeyDown(gui.KeyF2, false)

			if *calls != 0 {
				t.Errorf("F2 opened the name prompt with %d widgets selected", tc.count)
			}
		})
	}
}

// TestShiftTabSelectsPreviousWidget: Shift+Tab walks the cycle backwards. The
// two directions share one OnKeyDown case, so this is the only check that the
// Shift branch is wired to selectPrevWidget and not to selectNextWidget.
func TestShiftTabSelectsPreviousWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	first := addFakeAt(t, scene, "first", 10, 10, 30, 8)
	addFakeAt(t, scene, "middle", 10, 30, 30, 8)
	last := addFakeAt(t, scene, "last", 10, 50, 30, 8)

	view.Selection().Add(first)

	holdKeys(t, gui.KeyShift)
	view.OnKeyDown(gui.KeyTab, false)

	if !view.Selection().Contains(last) {
		t.Errorf("Shift+Tab from the first widget did not wrap to the last one")
	}
}

// TestTabKeySelectsNextWidget is the same check for plain Tab, so a regression
// that swapped the two branches fails in both directions.
func TestTabKeySelectsNextWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	first := addFakeAt(t, scene, "first", 10, 10, 30, 8)
	middle := addFakeAt(t, scene, "middle", 10, 30, 30, 8)
	addFakeAt(t, scene, "last", 10, 50, 30, 8)

	view.Selection().Add(first)

	holdKeys(t)
	view.OnKeyDown(gui.KeyTab, false)

	if !view.Selection().Contains(middle) {
		t.Errorf("Tab from the first widget did not select the second one")
	}
}
