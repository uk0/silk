package main

import (
	"errors"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type editorCleanupCommand struct {
	id, label, icon string
	run             func(*gui.CodeEditor) error
}

func editorCleanupCommands() []editorCleanupCommand {
	return []editorCleanupCommand{
		{"editor.trimTrailing", "Trim Trailing Whitespace", "whitespace-trim", (*gui.CodeEditor).TrimTrailingWhitespaceChecked},
		{"editor.finalNewline", "Ensure Final Newline", "newline-insert", (*gui.CodeEditor).EnsureFinalNewline},
	}
}
func (s editorCleanupCommand) available(e *gui.CodeEditor) bool {
	if e == nil || e.IsReadOnly() {
		return false
	}
	// Never scan all rows just to open the menu/palette. Trim can be a no-op.
	return s.id != "editor.finalNewline" || !e.HasFinalNewline() && e.Text() != ""
}
func runEditorCleanupCommand(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor, spec editorCleanupCommand) {
	e := visibleCaretEditor(tabs, canvas)
	if expected == nil {
		e = focusedClipboardEditor(tabs, canvas)
	}
	if expected != nil && e != expected || !spec.available(e) {
		return
	}
	e.SetFocus()
	if err := spec.run(e); err != nil {
		text := err.Error()
		if errors.Is(err, gui.ErrEditorCleanupCR) {
			text = i18n.T("Cleanup would change a standalone CR; no text was modified")
		}
		silkideToast(text, gui.ToastWarning)
	}
}
func bindEditorCleanupCommand(tabs *gui.TabWidget, canvas *ged.GedView, spec editorCleanupCommand) paletteCommand {
	e := focusedClipboardEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T(spec.label), Available: func() bool {
		return e != nil && visibleCaretEditor(tabs, canvas) == e && spec.available(e)
	}, Fn: func() {
		if e != nil {
			runEditorCleanupCommand(tabs, canvas, e, spec)
		}
	}}
}
