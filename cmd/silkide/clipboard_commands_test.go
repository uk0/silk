package main

import (
	"testing"

	"github.com/uk0/silk/gui"
)

func TestWorkbenchClipboardMenuInventory(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "clipboard.txt", "hello\n"))
	w.activate(modeEdit)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, id := range []string{"editor.cut", "editor.copy", "editor.paste", "editor.selectAll"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil || b.ShortcutText() == "" || !b.IsEnabled() {
			t.Errorf("missing or incomplete clipboard action: %s", id)
		}
	}
}

func TestClipboardMenusCannotSelectAnotherOrHiddenDocument(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "first.txt", "first"))
	w.activate(modeEdit)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "second.txt", "second"))
	e := activeEditor(w.tabs)
	menuCommandButton(m, "editor.selectAll").Action().Trigger(nil)
	if e.HasSelection() {
		t.Fatal("stale menu selected a different document")
	}
	for _, mode := range []workspaceMode{modeWelcome, modeDesign} {
		w.activate(mode)
		hidden := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
		for _, spec := range editorClipboardCommandSpecs() {
			b := menuCommandButton(hidden, spec.id)
			if b.IsEnabled() {
				t.Fatal("hidden editor clipboard command enabled", spec.id)
			}
			b.Action().Trigger(nil)
		}
		releaseWorkbenchMenu(hidden)
	}
	if e.Text() != "second" || e.HasSelection() {
		t.Fatal("hidden document changed")
	}
}

func TestClipboardCommandsDoNotStealFocusedTextFields(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "first.txt", "first"))
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetText("query")
	field.SetReadOnly(true)
	field.SetFocus()
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, spec := range editorClipboardCommandSpecs() {
		if menuCommandButton(m, spec.id).IsEnabled() {
			t.Fatal("editor command stole a field's focus", spec.id)
		}
		c := bindEditorClipboardCommand(w.tabs, w.canvas, spec)
		if c.available() {
			t.Fatal("palette bound editor behind a field")
		}
		c.Fn()
		runEditorClipboardCommand(w.tabs, w.canvas, spec, nil)
	}
	if e.Text() != "first" || e.HasSelection() || field.Text() != "query" || !field.HasFocus() {
		t.Fatal("text field protection")
	}
	e.SetFocus()
	e.ShowFind(false)
	if focusedClipboardEditor(w.tabs, w.canvas) != nil {
		t.Fatal("find query treated as document")
	}
	e.CancelFind()
}

func TestClipboardPaletteBindsBeforeModalFocusAndRechecksDocument(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "中🙂\n"))
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	registerPaletteCommands(w.tabs, w.canvas)
	commands := bindPaletteCommands(paletteCommands)
	var all paletteCommand
	for _, c := range commands {
		if c.Name == "Select All" {
			all = c
		}
	}
	if all.Fn == nil || !all.available() {
		t.Fatal("missing bound palette command")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	if !all.available() {
		t.Fatal("modal input invalidated captured scope")
	}
	selection := &paletteSelection{}
	selection.choose(nil, all)
	selection.run(gui.DialogOK)
	if !e.HasFocus() || e.SelectedText() != e.Text() {
		t.Fatal("palette did not restore document focus")
	}
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "b"))
	if all.available() {
		t.Fatal("bound command followed a different editor")
	}
	all.Fn()
	if activeEditor(w.tabs).HasSelection() {
		t.Fatal("stale palette selected a different editor")
	}
}
