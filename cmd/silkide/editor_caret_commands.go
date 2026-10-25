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
	keys := gui.CodeEditorShortcuts()
	return []editorCaretCommand{
		{"editor.addCursorAbove", "Add Cursor Above", "cursor-add-above", keys.AddCursorAbove, (*gui.CodeEditor).CanAddCursorAbove, func(e *gui.CodeEditor) { e.AddCursorAbove() }},
		{"editor.addCursorBelow", "Add Cursor Below", "cursor-add-below", keys.AddCursorBelow, (*gui.CodeEditor).CanAddCursorBelow, func(e *gui.CodeEditor) { e.AddCursorBelow() }},
		{"editor.moveLinesUp", "Move Lines Up", "line-move-up", keys.MoveLinesUp, (*gui.CodeEditor).CanMoveLinesUp, (*gui.CodeEditor).MoveLinesUp},
		{"editor.moveLinesDown", "Move Lines Down", "line-move-down", keys.MoveLinesDown, (*gui.CodeEditor).CanMoveLinesDown, (*gui.CodeEditor).MoveLinesDown},
		{"editor.duplicateLines", "Duplicate Lines", "line-duplicate", keys.DuplicateLines, (*gui.CodeEditor).CanDuplicateLines, (*gui.CodeEditor).DuplicateLines},
		{"editor.deleteLines", "Delete Lines", "line-delete", keys.DeleteLines, (*gui.CodeEditor).CanDeleteLines, (*gui.CodeEditor).DeleteLines},
		{"editor.indentLines", "Indent Lines", "navigate-forward", "", func(e *gui.CodeEditor) bool { return true }, (*gui.CodeEditor).IndentLines},
		{"editor.outdentLines", "Outdent Lines", "navigate-back", keys.OutdentLines, func(e *gui.CodeEditor) bool { return true }, (*gui.CodeEditor).OutdentLines},
		{"editor.clearCursors", "Keep Primary Cursor", "cursor-primary", keys.KeepPrimaryCursor, func(e *gui.CodeEditor) bool { return e.CursorCount() > 1 }, func(e *gui.CodeEditor) { e.KeepPrimaryCursor() }},
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
