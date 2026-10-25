package gui

import "errors"

// ErrEditorReadOnly is returned by editing operations with an error result
// when a host has put the editor into read-only mode.
var ErrEditorReadOnly = errors.New("editor is read-only")

// IsReadOnly reports whether document edit commands are locked. Selection,
// navigation, Find, Copy, folding and scrolling remain available.
func (e *CodeEditor) IsReadOnly() bool { return e.readOnly }

// SetReadOnly changes edit permission without changing text, revision, dirty
// state, selection or stored history. Unlocking restores Undo/Redo availability.
// Pending editor-owned paste, formatting, completion and replacement operations
// are retired when locking, even if the editor is subsequently unlocked.
// SetText is the explicit host/model-loading exception and still replaces the
// buffer and resets history. All access is UI-thread-only, like CodeEditor.
func (e *CodeEditor) SetReadOnly(value bool) {
	if e.readOnly == value {
		return
	}
	e.readOnly = value
	if value {
		e.CancelPendingPaste()
		e.CancelFormat()
		e.CancelCompletion()
		e.ClearExternalCompletions()
		e.DismissLanguageHints()
		e.snippetSess = nil
		if e.renameActive {
			e.renameCancel()
		}
		if e.findState.pending && e.findState.request.replace {
			e.findUpdateMatches()
		}
	}
	e.syncFindBar()
	e.notifyHistoryChanged()
	e.Update()
}
