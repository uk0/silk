package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type editorCaretCommand struct {
	id, label, icon, hint string
	enabled               func(*gui.CodeEditor) bool
	run                   func(*gui.CodeEditor)
}

func editorCaretCommandSpecs() []editorCaretCommand {
	key := actionKeyHint()
	return []editorCaretCommand{
		{"editor.addCursorAbove", "Add Cursor Above", "cursor-add-above", key + "+Alt+Up", (*gui.CodeEditor).CanAddCursorAbove, func(e *gui.CodeEditor) { e.AddCursorAbove() }},
		{"editor.addCursorBelow", "Add Cursor Below", "cursor-add-below", key + "+Alt+Down", (*gui.CodeEditor).CanAddCursorBelow, func(e *gui.CodeEditor) { e.AddCursorBelow() }},
		{"editor.clearCursors", "Keep Primary Cursor", "cursor-primary", "Esc", func(e *gui.CodeEditor) bool { return e.CursorCount() > 1 }, func(e *gui.CodeEditor) { e.KeepPrimaryCursor() }},
	}
}
func visibleCaretEditor(tabs *gui.TabWidget, canvas *ged.GedView) *gui.CodeEditor {
	if documentMode(tabs, canvas) != modeEdit {
		return nil
	}
	return activeEditor(tabs)
}

// A menu belongs to the document visible when it opened. Palette commands are
// registered once, so nil expected resolves the visible editor at execution.
func runEditorCaretCommand(tabs *gui.TabWidget, canvas *ged.GedView, spec editorCaretCommand, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if ed == nil {
		silkideToast(i18n.T("Open an editor document first"), gui.ToastWarning)
		return
	}
	if expected != nil && ed != expected {
		return
	}
	if spec.enabled(ed) {
		spec.run(ed)
	}
	ed.SetFocus()
}
