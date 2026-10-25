package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestWorkbenchAndPaletteShareSDKEditorShortcutHints(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "keys.txt", "first\nsecond\n"))
	w.activate(modeEdit)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	h := gui.CodeEditorShortcuts()
	want := map[string]string{"editor.addCursorAbove": h.AddCursorAbove, "editor.addCursorBelow": h.AddCursorBelow,
		"editor.clearCursors": h.KeepPrimaryCursor, "editor.moveLinesUp": h.MoveLinesUp, "editor.moveLinesDown": h.MoveLinesDown,
		"editor.duplicateLines": h.DuplicateLines, "editor.deleteLines": h.DeleteLines, "editor.outdentLines": h.OutdentLines,
		"editor.cut": h.Cut, "editor.copy": h.Copy, "editor.paste": h.Paste, "editor.selectAll": h.SelectAll}
	for id, hint := range want {
		b := menuCommandButton(m, id)
		if b == nil || b.ShortcutText() != hint {
			t.Fatalf("menu hint mismatch %s", id)
		}
	}
	for _, spec := range editorCaretCommandSpecs() {
		if hint, ok := want[spec.id]; ok {
			if spec.hint != hint {
				t.Fatal("caret spec drift", spec.id)
			}
			found := false
			for _, c := range paletteCommands {
				if c.Name == spec.label && c.Hint == hint {
					found = true
				}
			}
			if !found {
				t.Fatal("palette hint drift", spec.id)
			}
		}
	}
}
