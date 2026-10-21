package main

import (
	"errors"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// UI-owned, only explicit user refreshes enter this map. Closing or rebinding
// an editor retires the request; background failures never create toast storms.
var editorGitManualRefresh = map[*gui.CodeEditor]string{}

type editorGitFeedback struct{ last string }

func (f *editorGitFeedback) complete(ed *gui.CodeEditor, err error) {
	if openEditors[ed.FilePath()] != ed {
		return
	}
	requested, manual := editorGitManualRefresh[ed]
	delete(editorGitManualRefresh, ed)
	manual = manual && requested == ed.FilePath()
	if err == nil {
		f.last = ""
		if manual {
			silkideToast(i18n.T("Git markers refreshed"), gui.ToastSuccess)
		}
		return
	}
	message := err.Error()
	if message != f.last {
		f.last = message
		var detail *diagnostic.Error
		if errors.As(err, &detail) {
			message += "\n" + detail.Hint
		}
		logEvent(ged.LogWarn, message)
	}
	if manual {
		silkideToast(err.Error(), gui.ToastWarning)
	}
}
func configureEditorGitFeedback(ed *gui.CodeEditor) {
	f := &editorGitFeedback{}
	ed.SigGitStatusChanged(func() { f.complete(ed, ed.GitStatusError()) })
}
func refreshActiveEditorGit(tabs *gui.TabWidget) {
	ed := activeEditor(tabs)
	if ed == nil || ed.FilePath() == "" || rejectBusyFile(ed.FilePath()) {
		return
	}
	editorGitManualRefresh[ed] = ed.FilePath()
	ed.RefreshGitStatus()
}
