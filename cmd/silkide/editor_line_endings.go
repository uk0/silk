package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type editorLineEndingCommand struct {
	id, label string
	style     gui.EditorLineEnding
}

func editorLineEndingCommands() []editorLineEndingCommand {
	return []editorLineEndingCommand{
		{"editor.lineEndingLF", "Convert Line Endings to LF", gui.LineEndingLF},
		{"editor.lineEndingCRLF", "Convert Line Endings to CRLF", gui.LineEndingCRLF},
	}
}
func runEditorLineEndingCommand(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor, style gui.EditorLineEnding) {
	ed := visibleCaretEditor(tabs, canvas)
	if expected == nil {
		ed = focusedClipboardEditor(tabs, canvas)
	}
	if ed == nil || expected != nil && ed != expected || !ed.CanConvertLineEndings(style) {
		return
	}
	ed.SetFocus()
	if err := ed.ConvertLineEndings(style); err != nil {
		silkideToast(err.Error(), gui.ToastError)
	}
}
func bindEditorLineEndingCommand(tabs *gui.TabWidget, canvas *ged.GedView, spec editorLineEndingCommand) paletteCommand {
	ed := focusedClipboardEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T(spec.label), Available: func() bool {
		return ed != nil && visibleCaretEditor(tabs, canvas) == ed && ed.CanConvertLineEndings(spec.style)
	}, Fn: func() {
		if ed != nil {
			runEditorLineEndingCommand(tabs, canvas, ed, spec.style)
		}
	}}
}
