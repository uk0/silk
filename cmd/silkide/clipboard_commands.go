package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type editorClipboardCommand struct {
	id, label, icon, hint string
	available             func(*gui.CodeEditor) bool
	run                   func(*gui.CodeEditor) error
}

func editorClipboardCommandSpecs() []editorClipboardCommand {
	keys := gui.CodeEditorShortcuts()
	return []editorClipboardCommand{
		{"editor.cut", "Cut", "edit-cut", keys.Cut, (*gui.CodeEditor).CanCut, (*gui.CodeEditor).Cut},
		{"editor.copy", "Copy", "edit-copy", keys.Copy, (*gui.CodeEditor).CanCopy, (*gui.CodeEditor).Copy},
		{"editor.paste", "Paste", "edit-paste", keys.Paste, (*gui.CodeEditor).CanPaste, func(e *gui.CodeEditor) error { e.Paste(); return nil }},
		{"editor.selectAll", "Select All", "edit-select-all", keys.SelectAll, (*gui.CodeEditor).CanSelectAll, func(e *gui.CodeEditor) error { e.SelectAll(); return nil }},
	}
}

func focusedClipboardEditor(tabs *gui.TabWidget, canvas *ged.GedView) *gui.CodeEditor {
	ed := visibleCaretEditor(tabs, canvas)
	if ed == nil || !ed.HasFocus() {
		return nil
	}
	return ed
}

func bindEditorClipboardCommand(tabs *gui.TabWidget, canvas *ged.GedView, spec editorClipboardCommand) paletteCommand {
	ed := focusedClipboardEditor(tabs, canvas)
	return paletteCommand{
		Name: i18n.T(spec.label), Hint: spec.hint,
		Available: func() bool { return ed != nil && visibleCaretEditor(tabs, canvas) == ed && spec.available(ed) },
		Fn: func() {
			if ed != nil {
				runEditorClipboardCommand(tabs, canvas, spec, ed)
			}
		},
	}
}

func runEditorClipboardCommand(tabs *gui.TabWidget, canvas *ged.GedView, spec editorClipboardCommand, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if expected == nil {
		ed = focusedClipboardEditor(tabs, canvas)
	}
	if ed == nil || expected != nil && ed != expected || !spec.available(ed) {
		return
	}
	// Popup/palette callbacks run after dismissal. Restore document focus BEFORE
	// starting a paste: asynchronous delivery requires that same focused owner.
	ed.SetFocus()
	if err := spec.run(ed); err != nil {
		silkideToast(i18n.Tf("Clipboard operation failed: %v", err), gui.ToastError)
	}
}
