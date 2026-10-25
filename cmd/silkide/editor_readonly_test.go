package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestReadOnlyEditorMenusAndStaleCommands(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.json", "{\"a\":1}\n"))
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.DuplicateLines()
	ed.SetFocus()
	stale := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(stale)
	ed.SetReadOnly(true)
	locked := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(locked)
	original, rev := ed.Text(), ed.TextRevision()
	old := startLocalFormat
	defer func() { startLocalFormat = old }()
	startLocalFormat = func(*gui.CodeEditor, gui.EditorFormatOptions, func(error)) error {
		t.Fatal("read-only formatting admitted")
		return nil
	}
	for _, id := range []string{"edit.undo", "edit.redo", "editor.cut", "editor.paste", "editor.moveLinesUp", "editor.moveLinesDown", "editor.duplicateLines", "editor.deleteLines", "editor.indentLines", "editor.outdentLines", "editor.format", "editor.compactJSON"} {
		b := menuCommandButton(locked, id)
		if b == nil || b.IsEnabled() {
			t.Fatal("editable menu item", id)
		}
		menuCommandButton(stale, id).Action().Trigger(nil)
	}
	for _, id := range []string{"editor.copy", "editor.selectAll"} {
		if b := menuCommandButton(locked, id); b == nil || !b.IsEnabled() {
			t.Fatal("reader disabled", id)
		}
	}
	if ed.Text() != original || ed.TextRevision() != rev {
		t.Fatal("stale menu changed locked buffer")
	}
	ed.SetReadOnly(false)
	ed.Undo()
	if ed.Text() == original {
		t.Fatal("lock destroyed undo history")
	}
}
