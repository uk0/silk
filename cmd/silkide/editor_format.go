package main

import (
	"context"
	"errors"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

var startLocalFormat = (*gui.CodeEditor).FormatDocumentAsync

func registerEditorFormatShortcut(tabs *gui.TabWidget, canvas *ged.GedView) {
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'I', func() { formatActiveDocument(tabs, canvas, nil, false) })
}

// expected binds a popup action to its original document, not a reused index.
// JSON is local; Go retains gopls when available, with an offline local fallback.
func formatActiveDocument(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor, compact bool) {
	ed := visibleCaretEditor(tabs, canvas)
	if ed == nil || ed.IsReadOnly() || expected != nil && ed != expected {
		return
	}
	if !compact && ed.Language() == "go" && globalLSP != nil {
		formatDocumentViaLSP(tabs)
		return
	}
	path := ed.FilePath()
	report := func(err error) {
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		message := i18n.T("Format Document") + ": " + err.Error()
		logEvent(ged.LogError, path+": "+message)
		silkideToast(message, gui.ToastError)
	}
	report(startLocalFormat(ed, gui.EditorFormatOptions{Compact: compact}, report))
}

func cancelActiveFormatting(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if ed != nil && (expected == nil || expected == ed) {
		ed.CancelFormat()
	}
}
