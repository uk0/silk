package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func showEditorGoToLine(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if expected == nil {
		ed = focusedClipboardEditor(tabs, canvas)
	}
	if ed == nil || expected != nil && ed != expected {
		return
	}
	ed.ShowGoToLine()
}
func bindEditorGoToLine(tabs *gui.TabWidget, canvas *ged.GedView) paletteCommand {
	ed := focusedClipboardEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T("Go to Line / Column"), Hint: gui.CodeEditorShortcuts().GoToLine,
		Available: func() bool { return ed != nil && visibleCaretEditor(tabs, canvas) == ed },
		Fn: func() {
			if ed != nil {
				showEditorGoToLine(tabs, canvas, ed)
			}
		},
	}
}
