package gui

import "fmt"

// CursorCount includes the primary insertion point. UI-thread only.
func (e *CodeEditor) CursorCount() int { return 1 + len(e.additionalCursors) }

// CanAddCursorAbove/Below are pure availability checks. The extreme existing
// caret anchors the command; a short next line clamps its column, as with the
// editor's existing keyboard gesture. No document or line text is scanned.
func (e *CodeEditor) CanAddCursorAbove() bool { _, ok := e.adjacentCaret(-1); return ok }
func (e *CodeEditor) CanAddCursorBelow() bool { _, ok := e.adjacentCaret(1); return ok }

// AddCursorAbove/Below add an insertion point without moving the primary one.
// A successful command clears the primary selection, cancels transient input
// UI/snippets, unfolds and reveals the new caret, and focuses the editor body.
// It never changes document bytes, revisions or undo history. At a boundary it
// returns false and leaves all state unchanged. Call on the UI thread.
func (e *CodeEditor) AddCursorAbove() bool { return e.addAdjacentCaret(-1) }
func (e *CodeEditor) AddCursorBelow() bool { return e.addAdjacentCaret(1) }

func (e *CodeEditor) adjacentCaret(delta int) (cursorPos, bool) {
	if len(e.lines) == 0 {
		return cursorPos{}, false
	}
	anchor := cursorPos{max(0, min(e.cursorLine, len(e.lines)-1)), e.cursorCol}
	for _, c := range e.additionalCursors {
		if c.line < 0 || c.line >= len(e.lines) {
			continue
		}
		if delta < 0 && c.line < anchor.line || delta > 0 && c.line > anchor.line {
			anchor = c
		}
	}
	anchor.line += delta
	return anchor, anchor.line >= 0 && anchor.line < len(e.lines)
}
func (e *CodeEditor) addAdjacentCaret(delta int) bool {
	target, ok := e.adjacentCaret(delta)
	if !ok {
		return false
	}
	e.clampCursor()
	e.AddCursorAtLine(target.line, target.col)
	p, _ := e.TextPositionAt(target.line, target.col)
	e.clearSelection()
	e.prepareCaretCommand()
	e.revealCaretLine(p.Line)
	e.ensureTextPositionVisible(p)
	e.SetFocus()
	e.Self().Update()
	return true
}

// KeepPrimaryCursor is the command variant of ClearAdditionalCursors. It also
// dismisses transient input UI and reveals/focuses the original primary caret.
// Any primary selection is retained. False means there were no extra carets.
func (e *CodeEditor) KeepPrimaryCursor() bool {
	if len(e.additionalCursors) == 0 {
		return false
	}
	e.additionalCursors = nil
	e.prepareCaretCommand()
	p := e.clampCursorTextPosition()
	e.revealCaretLine(p.Line)
	e.ensureTextPositionVisible(p)
	e.SetFocus()
	e.Self().Update()
	return true
}
func (e *CodeEditor) prepareCaretCommand() {
	e.DismissLanguageHints()
	if e.completion != nil {
		e.completion.Dismiss()
	}
	if e.symbolPopup != nil {
		e.symbolPopup.Dismiss()
	}
	e.snippetSess = nil
	e.renameActive = false
	e.hideGoToLine(false)
	if e.findActive {
		e.closeFindBar()
	}
}
func (e *CodeEditor) revealCaretLine(line int) {
	if len(e.foldedLines) == 0 {
		return
	}
	changed := false
	for _, region := range e.FoldRegions() {
		if region.startLine < line && line <= region.endLine && e.foldedLines[region.startLine] {
			delete(e.foldedLines, region.startLine)
			changed = true
		}
	}
	if changed {
		e.invalidateVisLineCache()
	}
}

// Convert only the requested prefix, not the entire line. This path is shared
// by input/navigation and caret reveal, including malformed UTF-8 source bytes.
func (e *CodeEditor) clampCursorTextPosition() EditorTextPosition {
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	e.cursorLine = max(0, min(e.cursorLine, len(e.lines)-1))
	p := textPositionInLine(e.lines[e.cursorLine], e.cursorCol)
	p.Line = e.cursorLine
	e.cursorCol = p.RuneColumn
	return p
}
func (e *CodeEditor) cursorStatusLabel(compact bool) string {
	var label string
	if compact {
		label = fmt.Sprintf("%d:%d", e.cursorLine+1, e.cursorCol+1)
	} else {
		label = fmt.Sprintf("Ln %d, Col %d", e.cursorLine+1, e.cursorCol+1)
	}
	if n := e.CursorCount(); n > 1 {
		label += fmt.Sprintf(" (%d carets)", n)
	}
	return label
}
