package ged

import (
	"testing"
)

// containsItem reports whether the GedView's current selection holds fw.
func selectionHas(view *GedView, fw *FakeWidget) bool {
	return view.Selection().Contains(fw)
}

// TestSelectAllSelectsEveryWidget: selectAll() pulls every selectable widget
// on the page into the selection. Three widgets are dropped on the scene; after
// selectAll() the selection count must be exactly 3 and each widget present.
//
// Ctrl detection: OnKeyDown reads the modifier via gui.IsKeyDown(KeyCtrl),
// which queries live GLFW window state with no test hook (the nudge tests note
// the same limitation for Shift). So this drives selectAll() directly — the
// exact method the Cmd/Ctrl+A case in OnKeyDown calls — rather than synthesizing
// a modifier-down key event.
func TestSelectAllSelectsEveryWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	c := addFake(t, scene, "c")

	view.selectAll()

	if got := view.Selection().Count(); got != 3 {
		t.Fatalf("selectAll selected %d items, want 3", got)
	}
	for _, fw := range []*FakeWidget{a, b, c} {
		if !selectionHas(view, fw) {
			t.Errorf("selectAll did not select %q", fw.WidgetName())
		}
	}
}

// TestSelectAllClearsPriorSelection: selectAll() replaces whatever was selected
// rather than appending. Pre-select one widget, then selectAll() and confirm the
// count is the full set (no duplicate of the pre-selected one) — Selection.Add
// dedups, so the real check here is that the prior single-selection didn't leave
// stale state and the result is the complete page.
func TestSelectAllClearsPriorSelection(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	addFake(t, scene, "b")

	view.Selection().Clear()
	view.Selection().Add(a)
	if view.Selection().Count() != 1 {
		t.Fatalf("setup: pre-selection count = %d, want 1", view.Selection().Count())
	}

	view.selectAll()

	if got := view.Selection().Count(); got != 2 {
		t.Errorf("after selectAll: count = %d, want 2", got)
	}
}

// TestSelectAllSkipsNonSelectable: a widget with SetSelectable(false) must be
// left out, mirroring graph's TraversalCond_Selectable (a marquee drag skips it
// too). Two selectable + one non-selectable => selectAll picks exactly the two.
func TestSelectAllSkipsNonSelectable(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	hidden := addFake(t, scene, "noselect")
	hidden.SetSelectable(false)

	view.selectAll()

	if got := view.Selection().Count(); got != 2 {
		t.Fatalf("selectAll selected %d items, want 2 (non-selectable skipped)", got)
	}
	if selectionHas(view, hidden) {
		t.Errorf("selectAll selected the non-selectable widget")
	}
	for _, fw := range []*FakeWidget{a, b} {
		if !selectionHas(view, fw) {
			t.Errorf("selectAll did not select %q", fw.WidgetName())
		}
	}
}

// TestSelectAllExcludesSceneRoot: the scene/page root is never selected.
// selectAll iterates Scene().Children() (the dropped widgets only), so the
// selection must contain none of the scene object itself — asserted by checking
// the count equals the child widget count and the scene is not Contains'd.
func TestSelectAllExcludesSceneRoot(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	addFake(t, scene, "a")
	addFake(t, scene, "b")

	view.selectAll()

	if view.Selection().Contains(scene) {
		t.Errorf("selectAll selected the scene root")
	}
	if got := view.Selection().Count(); got != 2 {
		t.Errorf("after selectAll: count = %d, want 2 (children only)", got)
	}
}

// TestSelectAllIncludesLocked: a locked widget stays IsSelectable (IsLocked
// only pins position/size), so selectAll must still grab it — a designer needs
// to select a locked widget to unlock it. Unlike align/nudge, locked is NOT a
// skip reason for selection.
func TestSelectAllIncludesLocked(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	locked := addFake(t, scene, "locked")
	locked.SetLocked(true)

	view.selectAll()

	if got := view.Selection().Count(); got != 2 {
		t.Fatalf("selectAll selected %d items, want 2 (locked included)", got)
	}
	if !selectionHas(view, locked) {
		t.Errorf("selectAll skipped the locked widget; locked must stay selectable")
	}
	if !selectionHas(view, a) {
		t.Errorf("selectAll did not select %q", a.WidgetName())
	}
}
