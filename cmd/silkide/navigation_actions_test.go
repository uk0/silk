package main

import (
	"fmt"
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

func TestNavigationToolbarHasDiscoverableActions(t *testing.T) {
	w := testWorkbench(t)
	buildToolBar(w.frame, w.tabs, w.canvas)
	for _, name := range []string{"Go Back", "Go Forward"} {
		b := historyButton(t, w, name)
		hint := gui.CodeEditorShortcuts().GoBack
		if name == "Go Forward" {
			hint = gui.CodeEditorShortcuts().GoForward
		}
		if !strings.Contains(gui.GetToolTip(b), hint) {
			t.Fatal("missing navigation shortcut hint", name)
		}
		if b.IsEnabled() || b.Action().IsEnabled() {
			t.Fatalf("empty history enabled %s", name)
		}
	}
}

func TestNavigationToolbarEventsClearAndRetirement(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "first\nsecond\n")
	b := writeFile(t, dir, "b.txt", "alpha\nbeta\n")
	openFileInEditor(w.tabs, a)
	w.activate(modeEdit)
	buildToolBar(w.frame, w.tabs, w.canvas)
	back, forward := historyButton(t, w, "Go Back"), historyButton(t, w, "Go Forward")
	check := func(wantBack, wantForward bool) {
		t.Helper()
		if back.IsEnabled() != wantBack || back.Action().IsEnabled() != wantBack || forward.IsEnabled() != wantForward || forward.Action().IsEnabled() != wantForward {
			t.Fatalf("got %v/%v want %v/%v", back.IsEnabled(), forward.IsEnabled(), wantBack, wantForward)
		}
	}
	check(false, false)
	if !openFileFinderSelection(w.tabs, b, "2:2") {
		t.Fatal("location jump")
	}
	check(true, false)
	target := activeEditor(w.tabs)
	back.Action().Trigger(nil)
	check(false, true)
	if activeEditor(w.tabs).FilePath() != a {
		t.Fatal("back wrong document")
	}
	forward.Action().Trigger(nil)
	check(true, false)
	if activeEditor(w.tabs) != target || target.CursorLine() != 1 || target.CursorCol() != 1 {
		t.Fatal("forward wrong caret")
	}
	rev := back.Action().Rev()
	for i := 0; i < 100; i++ {
		w.syncNavigationActions()
	}
	if back.Action().Rev() != rev {
		t.Fatal("idle invalidation")
	}
	w.activate(modeWelcome)
	back.Action().Trigger(nil)
	check(false, true)
	if documentMode(w.tabs, w.canvas) != modeEdit || activeEditor(w.tabs).FilePath() != a {
		t.Fatal("history did not reveal editor from Welcome")
	}
	// Clear is workspace history only: no text/history/caret/dirty changes.
	e := activeEditor(w.tabs)
	e.DuplicateLines()
	text, pos, dirty := e.Text(), e.NavigationPosition(), e.Modified()
	clearEditorNavigation(w.tabs)
	check(false, false)
	if e.Text() != text || e.NavigationPosition() != pos || e.Modified() != dirty || !e.CanUndo() {
		t.Fatal("clear changed document")
	}
	openFileFinderSelection(w.tabs, b, "1:1")
	check(true, false)
	buildToolBar(w.frame, w.tabs, w.canvas)
	oldBack, oldForward := back, forward
	br, fr := oldBack.Action().Rev(), oldForward.Action().Rev()
	back, forward = historyButton(t, w, "Go Back"), historyButton(t, w, "Go Forward")
	clearEditorNavigation(w.tabs)
	check(false, false)
	if gui.GetToolTip(oldBack) != "" || gui.GetToolTip(oldForward) != "" {
		t.Fatal("retired toolbar retained tooltips")
	}
	if oldBack.Action().Rev() != br || oldForward.Action().Rev() != fr {
		t.Fatal("retired toolbar still updates")
	}
	w.closeNavigationActions()
	br = back.Action().Rev()
	openFileFinderSelection(w.tabs, a, "2:2")
	if back.Action().Rev() != br {
		t.Fatal("closed subscription still updates")
	}
}

func TestNavigationMenuPaletteAvailabilityAndWorkspaceScope(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	commands := map[string]paletteCommand{}
	for _, c := range paletteCommands {
		if c.Name == "Go Back" || c.Name == "Go Forward" || c.Name == "Clear Navigation History" {
			commands[c.Name] = c
		}
	}
	if len(commands) != 3 {
		t.Fatal("missing navigation palette commands")
	}
	for name, c := range commands {
		if c.Available == nil || c.available() {
			t.Fatal("empty palette command enabled", name)
		}
	}
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	for _, id := range []string{"navigate.back", "navigate.forward", "navigate.clear"} {
		b := menuCommandButton(menu, id)
		if b.Icon() == nil || b.Icon().IsAir() || fmt.Sprint(b.Icon()) == "image-missing" {
			t.Fatal("missing navigation icon", id)
		}
		if b.IsEnabled() {
			t.Fatal("empty menu enabled", id)
		}
	}
	releaseWorkbenchMenu(menu)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "first\n")
	b := writeFile(t, dir, "b.txt", "second\n")
	openFileInEditor(w.tabs, a)
	openFileFinderSelection(w.tabs, b, "1:2")
	w.activate(modeWelcome)
	if !commands["Go Back"].available() || commands["Go Forward"].available() || !commands["Clear Navigation History"].available() {
		t.Fatal("palette stale availability")
	}
	commands["Go Back"].Fn()
	if activeEditor(w.tabs).FilePath() != a || documentMode(w.tabs, w.canvas) != modeEdit {
		t.Fatal("palette blocked workspace navigation")
	}
	menu = buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	menuCommandButton(menu, "navigate.clear").Action().Trigger(nil)
	releaseWorkbenchMenu(menu)
	for name, c := range commands {
		if c.available() {
			t.Fatal("clear left command enabled", name)
		}
	}
}
