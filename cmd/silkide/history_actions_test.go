package main

import (
	"fmt"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

func historyButton(t *testing.T, w *workbench, name string) *gui.Button {
	t.Helper()
	for _, item := range w.frame.ToolBar().Items() {
		if b, ok := item.(*gui.Button); ok && b.Text() == name {
			return b
		}
	}
	t.Fatalf("missing history button %s", name)
	return nil
}
func TestHistoryMenusAndToolbarRespectEmptyStacks(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "first.txt", "a\nb\n"))
	w.activate(modeEdit)
	buildToolBar(w.frame, w.tabs, w.canvas)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	if menuCommandButton(menu, "edit.undo").IsEnabled() || menuCommandButton(menu, "edit.redo").IsEnabled() {
		t.Error("empty editor history remains enabled in menu")
	}
	if historyButton(t, w, "Undo").IsEnabled() || historyButton(t, w, "Redo").IsEnabled() {
		t.Error("empty editor history remains enabled in toolbar")
	}
}
func TestHistoryMenuCannotUndoAnotherDocument(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "first.txt", "a\n"))
	w.activate(modeEdit)
	first := activeEditor(w.tabs)
	first.DuplicateLines()
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "second.txt", "b\n"))
	second := activeEditor(w.tabs)
	second.DuplicateLines()
	menuCommandButton(menu, "edit.undo").Action().Trigger(nil)
	if second.Text() != "b\nb\n" {
		t.Fatal("stale menu undid another document")
	}
}

func TestHistoryToolbarFollowsEventsTabsModesAndRetirement(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "first.txt", "a\nb\n"))
	w.activate(modeEdit)
	buildToolBar(w.frame, w.tabs, w.canvas)
	first := activeEditor(w.tabs)
	undo, redo := historyButton(t, w, "Undo"), historyButton(t, w, "Redo")
	check := func(u, r bool) {
		t.Helper()
		if undo.IsEnabled() != u || redo.IsEnabled() != r || undo.Action().IsEnabled() != u || redo.Action().IsEnabled() != r {
			t.Fatalf("states got %v/%v want %v/%v", undo.IsEnabled(), redo.IsEnabled(), u, r)
		}
	}
	first.DuplicateLines()
	check(true, false)
	undo.Action().Trigger(nil)
	check(false, true)
	redo.Action().Trigger(nil)
	check(true, false)
	first.Undo()
	first.DeleteLines()
	check(true, false)
	urev, rrev := undo.Action().Rev(), redo.Action().Rev()
	for i := 0; i < 50; i++ {
		w.syncHistoryActions()
	}
	if undo.Action().Rev() != urev || redo.Action().Rev() != rrev {
		t.Fatal("idle history sync touched actions")
	}
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "second.txt", "b\n"))
	second := activeEditor(w.tabs)
	check(false, false)
	first.Undo()
	check(false, false)
	second.DuplicateLines()
	check(true, false)
	w.tabs.SetCurrentIndex(0)
	check(false, true)
	w.activate(modeWelcome)
	check(false, false)
	undoCurrentDocument(w.tabs, w.canvas, true)
	if first.Text() != "a\nb\n" {
		t.Fatal("Welcome history mutated hidden editor")
	}
	w.center.SetActiveIndex(w.center.IndexOfView(w.canvas))
	check(false, false)
	w.canvas.GedScene().PushCommand(gui.NewCompoundCommand())
	check(true, false)
	undoCurrentDocument(w.tabs, w.canvas, false)
	check(false, true)
	w.activate(modeEdit)
	check(false, true)
	first.Redo()
	check(true, false)
	// Rebuilding the toolbar retires old subscriptions; old widgets stay still.
	buildToolBar(w.frame, w.tabs, w.canvas)
	urev = undo.Action().Rev()
	first.SetText("reset")
	if undo.Action().Rev() != urev {
		t.Fatal("retired toolbar still subscribed")
	}
	undo, redo = historyButton(t, w, "Undo"), historyButton(t, w, "Redo")
	check(false, false)
	w.closeHistoryActions()
	urev = undo.Action().Rev()
	first.DuplicateLines()
	if undo.Action().Rev() != urev {
		t.Fatal("closed controller still updated")
	}
}

func TestHistoryPaletteAvailabilityAndExecutionGuards(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "one\n"))
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	registerPaletteCommands(w.tabs, w.canvas)
	var undo, redo paletteCommand
	for _, c := range paletteCommands {
		if c.Name == "Undo" {
			undo = c
		}
		if c.Name == "Redo" {
			redo = c
		}
	}
	if undo.Available == nil || redo.Available == nil || undo.available() || redo.available() {
		t.Fatal("initial palette availability")
	}
	if !strings.Contains(undo.displayText(), "Unavailable") {
		t.Fatal("missing unavailable hint")
	}
	s := &paletteSelection{}
	s.choose(nil, undo)
	if s.command != nil {
		t.Fatal("disabled command dismissed palette")
	}
	e.DuplicateLines()
	if !undo.available() || redo.available() {
		t.Fatal("stale palette availability")
	}
	s.choose(nil, undo)
	e.SetText("reset")
	s.run(gui.DialogOK)
	if e.Text() != "reset" {
		t.Fatal("command ran after availability changed")
	}
	e.DuplicateLines()
	s.choose(nil, undo)
	s.run(gui.DialogOK)
	if e.Text() != "reset" || !redo.available() {
		t.Fatal("palette Undo")
	}
	redo.Fn()
	if e.Text() != "reset\nreset" {
		t.Fatal("palette Redo")
	}
	w.activate(modeWelcome)
	if undo.available() || redo.available() {
		t.Fatal("Welcome palette availability")
	}
}

type historyPendingDraft struct {
	scene   *ged.GedScene
	pending bool
	fail    bool
}

func (d *historyPendingDraft) HasPendingChanges() bool { return d.pending }
func (d *historyPendingDraft) CommitPendingChanges() error {
	if d.fail {
		return fmt.Errorf("draft rejected")
	}
	d.pending = false
	d.scene.PushCommand(gui.NewCompoundCommand())
	return nil
}
func TestHistoryTargetIncludesPendingDesignDrafts(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	s := w.canvas.GedScene()
	d := &historyPendingDraft{scene: s, pending: true}
	id := s.AddPendingEditor(d)
	defer s.RemovePendingEditor(id)
	target := currentDocumentHistory(w.tabs, w.canvas)
	if !target.available(false) || target.available(true) || s.UndoStack().CanUndo() {
		t.Fatal("pending availability")
	}
	undoCurrentDocument(w.tabs, w.canvas, false)
	if d.pending || s.UndoStack().CanUndo() || !s.UndoStack().CanRedo() {
		t.Fatal("draft was not committed and undone")
	}
	d.pending = true
	d.fail = true
	if target.available(true) {
		t.Fatal("Redo would discard pending draft")
	}
	undoCurrentDocument(w.tabs, w.canvas, false)
	if !d.pending || s.UndoStack().CanUndo() || !s.UndoStack().CanRedo() {
		t.Fatal("failed draft changed history")
	}
}
