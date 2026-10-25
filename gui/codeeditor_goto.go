package gui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrEditorLocation identifies invalid one-based line[:column] destinations.
var ErrEditorLocation = errors.New("invalid editor location")

// ResolveGoToLocation resolves a one-based line or line:column in the current
// unsaved buffer. Columns count Unicode code points, not bytes, graphemes, visual
// tab cells or LSP UTF-16 units. End-of-line is valid; the CR of CRLF is excluded.
// Invalid destinations are rejected, never silently clamped. A valid query scans
// only the requested line prefix and allocates nothing. UI-thread only.
func (e *CodeEditor) ResolveGoToLocation(location string) (EditorTextPosition, error) {
	bad := func(message string) (EditorTextPosition, error) {
		return EditorTextPosition{}, fmt.Errorf("%w: %s", ErrEditorLocation, message)
	}
	if len(location) > 64 {
		return bad("use line or line:column")
	}
	value := strings.TrimSpace(location)
	lineText, columnText, hasColumn := strings.Cut(value, ":")
	positive := func(s string) (int, bool) {
		if s == "" {
			return 0, false
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return 0, false
			}
		}
		n, err := strconv.Atoi(s)
		return n, err == nil && n > 0
	}
	line, ok := positive(lineText)
	column := 1
	if !ok {
		return bad("use line or line:column")
	}
	if hasColumn {
		column, ok = positive(columnText)
		if !ok {
			return bad("use line or line:column")
		}
	}
	if line > len(e.lines) {
		return bad(fmt.Sprintf("line must be 1-%d", len(e.lines)))
	}
	source := e.lines[line-1]
	if e.lineHasCRLF(line - 1) {
		source = source[:len(source)-1]
	}
	p := textPositionInLine(source, column-1)
	if p.RuneColumn != column-1 {
		return bad(fmt.Sprintf("column must be 1-%d", p.RuneColumn+1))
	}
	p.Line = line - 1
	return p, nil
}

// NavigateToLocation is the checked, one-based user-input counterpart of
// NavigateToLineCol. On success it reveals folds, retires selection/extra carets
// and focuses the document. Failure preserves text, history and navigation state.
// It is available for read-only documents; all access is UI-thread-only.
func (e *CodeEditor) NavigateToLocation(location string) error {
	p, err := e.ResolveGoToLocation(location)
	if err != nil {
		return err
	}
	e.hideGoToLine(false)
	e.NavigateToLineCol(p.Line, p.RuneColumn)
	return nil
}

// ShowGoToLine opens an editor-owned child input. Reopening selects the existing
// query; a fresh bar is seeded with the current one-based line and rune column.
// Opening/cancelling never changes selection, text or undo history.
func (e *CodeEditor) ShowGoToLine() {
	e.DismissLanguageHints()
	e.CancelPendingPaste()
	e.CancelCompletion()
	if e.symbolPopup != nil {
		e.symbolPopup.Dismiss()
	}
	e.renameActive = false
	e.snippetSess = nil
	fresh := !e.gotoLineActive
	e.gotoLineActive = true
	e.ensureGoToBar()
	if fresh {
		source, _ := e.LineText(e.cursorLine)
		if e.lineHasCRLF(e.cursorLine) {
			source = source[:len(source)-1]
		}
		column := textPositionInLine(source, e.cursorCol).RuneColumn
		e.gotoBar.query.SetText(fmt.Sprintf("%d:%d", e.cursorLine+1, column+1))
	}
	e.syncGoToBar()
	e.Layout()
	e.gotoBar.query.SetFocus()
	e.gotoBar.query.SelectAll()
	e.Update()
}
func (e *CodeEditor) GoToLineVisible() bool { return e.gotoLineActive }

// HideGoToLine cancels pending input and returns focus only if this bar owned it.
// Call when disposing an editor; hiding an ancestor also retires the bar.
func (e *CodeEditor) HideGoToLine() { e.hideGoToLine(true) }
func (e *CodeEditor) hideGoToLine(restoreFocus bool) {
	if !e.gotoLineActive {
		return
	}
	owned := false
	if b := e.gotoBar; b != nil {
		owned = b.query.HasFocus() || b.accept.HasFocus() || b.close.HasFocus()
		b.query.CancelPendingPaste()
		b.tooltips(false)
		b.Hide()
	}
	e.gotoLineActive = false
	e.gotoLineText = ""
	e.gotoBarHeight = 0
	if restoreFocus && owned {
		e.SetFocus()
	}
	e.Layout()
}
func (e *CodeEditor) gotoLineAccept() {
	if !e.gotoLineActive || e.gotoBar == nil {
		return
	}
	if err := e.NavigateToLocation(e.gotoBar.query.Text()); err != nil {
		e.syncGoToBar()
		e.gotoBar.query.SetFocus()
	}
}
