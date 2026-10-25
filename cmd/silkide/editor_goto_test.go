package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestGoToMenuPaletteReadonlyAndFocusIdentity(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "one.txt", "a\n中🙂xyz"))
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetReadOnly(true)
	ed.SetFocus()
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	b := menuCommandButton(menu, "navigate.line")
	if b == nil || !b.IsEnabled() || b.Icon() == nil {
		t.Fatal("missing readonly navigation command")
	}
	palette := bindEditorGoToLine(w.tabs, w.canvas)
	if !palette.available() || palette.Hint != gui.CodeEditorShortcuts().GoToLine {
		t.Fatal("missing palette binding")
	}
	b.Action().Trigger(nil)
	if !ed.GoToLineVisible() {
		t.Fatal("menu did not open child input")
	}
	ed.HideGoToLine()
	palette.Fn()
	if !ed.GoToLineVisible() {
		t.Fatal("palette did not open child input")
	}
	ed.HideGoToLine()
	if err := ed.NavigateToLocation("2:3"); err != nil || ed.CursorLine() != 1 || ed.CursorCol() != 2 || ed.Modified() {
		t.Fatal("readonly location navigation failed", err)
	}
	// A modal command is attached to a document identity, not a reused tab index.
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "two.txt", "other"))
	other := activeEditor(w.tabs)
	b.Action().Trigger(nil)
	palette.Fn()
	if other.GoToLineVisible() || ed.GoToLineVisible() {
		t.Fatal("stale command opened another editor")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	blocked := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(blocked)
	if menuCommandButton(blocked, "navigate.line").IsEnabled() {
		t.Fatal("menu stole focused field")
	}
	showEditorGoToLine(w.tabs, w.canvas, nil)
	if other.GoToLineVisible() {
		t.Fatal("command stole field focus")
	}
}
func TestGoToClosedEditorRetiresChildInput(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "a\nb")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.ShowGoToLine()
	if !detachEditorTab(w.tabs, w.tabs.Stack().Page(w.tabs.CurrentIndex()), ed, path) {
		t.Fatal("detach")
	}
	if ed.GoToLineVisible() {
		t.Fatal("closed editor retained child input")
	}
}
