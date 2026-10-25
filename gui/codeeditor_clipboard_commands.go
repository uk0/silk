package gui

import (
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
)

// CanCopy reports whether the selection, or the primary caret's current line,
// contains text. Queries do not allocate selected text or access the OS clipboard.
// Like the other editor commands, these APIs must be called on the UI thread.
func (e *CodeEditor) CanCopy() bool {
	if !e.IsEnabled() {
		return false
	}
	if e.hasSelection {
		sl, sc, el, ec := e.selectionRange()
		return sl != el || sc != ec
	}
	return e.cursorLine >= 0 && e.cursorLine < len(e.lines) && e.lines[e.cursorLine] != ""
}

// CanCut also permits removing an empty line from a multi-line buffer. As with
// the existing keyboard command, a no-selection cut affects only the primary
// caret's line, not secondary caret rows.
func (e *CodeEditor) CanCut() bool {
	return !e.readOnly && (e.CanCopy() || e.IsEnabled() && !e.hasSelection && len(e.lines) > 1)
}

// CanPaste is permission to request a paste, not a clipboard-content probe.
// Menus must not materialize promised/large native clipboard data. An empty
// clipboard remains a no-op when Paste executes.
func (e *CodeEditor) CanPaste() bool { return !e.readOnly && e.IsEnabled() }
func (e *CodeEditor) CanSelectAll() bool {
	return e.IsEnabled() && (len(e.lines) > 1 || len(e.lines) == 1 && e.lines[0] != "")
}

func writeEditorClipboard(text string) error {
	// SetData replaces text itself on every backend. Do not Clear first: invalid
	// payloads or a busy clipboard must not erase the previous clipboard value.
	_, err := Clipboard.SetData(text)
	return err
}

// Copy writes selected text, or the primary caret's line without an added LF.
// It never edits text/history. Disabled/empty commands are no-ops; native write
// failures are returned to the caller rather than silently reported as success.
// Copy/Cut reject text over 16 MiB, matching the paste limit, before writing or
// deleting anything. Large multi-line selections are sized before allocation.
func (e *CodeEditor) Copy() error { return e.copyOrCut(false, writeEditorClipboard) }

// Cut removes text only AFTER a successful clipboard write, in one Undo step.
// An empty-line cut leaves the clipboard alone, matching the keyboard behavior.
func (e *CodeEditor) Cut() error { return e.copyOrCut(true, writeEditorClipboard) }

// copyOrCut's explicit writer is a deterministic test seam, not a global hook.
func (e *CodeEditor) copyOrCut(cut bool, write func(string) error) error {
	if cut && !e.CanCut() || !cut && !e.CanCopy() {
		return nil
	}
	e.clampCursor()
	text, err := e.clipboardCommandText()
	if err != nil {
		return err
	}
	if text != "" {
		if err := clipboardTextErr(text); err != nil {
			return err
		}
		if err := write(text); err != nil {
			return err
		}
	}
	if !cut || e.readOnly {
		return nil
	}
	e.CancelPendingPaste()
	if e.hasSelection {
		e.replaceSelectionRecorded("")
		e.rebuildText()
		e.ensureCursorVisible()
		e.Self().Update()
	} else {
		e.deleteCurrentLine()
	}
	return nil
}

func (e *CodeEditor) clipboardCommandText() (string, error) {
	if !e.hasSelection {
		text := e.lines[e.cursorLine]
		if len(text) > maxClipboardTextBytes {
			return "", ErrClipboardTooLarge
		}
		return text, nil
	}
	sl, sc, el, ec := e.selectionRange()
	start := textPositionInLine(e.lines[sl], sc).ByteColumn
	end := textPositionInLine(e.lines[el], ec).ByteColumn
	size := end - start
	if sl != el {
		size = len(e.lines[sl]) - start + 1 + end
		for i := sl + 1; i < el && size <= maxClipboardTextBytes; i++ {
			size += len(e.lines[i]) + 1
		}
	}
	if size > maxClipboardTextBytes {
		return "", ErrClipboardTooLarge
	}
	return e.SelectedText(), nil
}

// Paste uses the same bounded, stale-target-checked path as the keyboard.
// macOS delivery is asynchronous; callers should focus a live editor before
// requesting a paste. Other backends retain their native synchronous reads.
func (e *CodeEditor) Paste() {
	if e.CanPaste() {
		e.pasteClipboard()
	}
}

// SelectAll selects the entire buffer and retires secondary carets. Empty text
// has no selection; this operation does not modify the document or edit history.
func (e *CodeEditor) SelectAll() {
	if !e.IsEnabled() {
		return
	}
	e.CancelPendingPaste()
	e.additionalCursors = nil
	e.snippetSess = nil
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine = len(e.lines) - 1
	e.selEndCol = utf8.RuneCountInString(e.lines[e.selEndLine])
	e.hasSelection = e.CanSelectAll()
	e.cursorLine, e.cursorCol = e.selEndLine, e.selEndCol
	e.Self().Update()
}

func (e *CodeEditor) clipboardCommandFailed(operation string, err error) {
	if err == nil {
		return
	}
	core.Warn("clipboard ", operation, ": ", err)
	if e.OwnerWindow() != nil {
		ShowToast(e, i18n.Tf("Clipboard operation failed: %v", err), 3500, ToastError)
	}
}
