package gui

import "unicode/utf8"

// EditorTextPosition is a value snapshot of one zero-based position. RuneColumn
// counts Unicode code points (not grapheme clusters), ByteColumn counts UTF-8
// bytes, and UTF16Column uses LSP code units. Invalid UTF-8 bytes count as U+FFFD.
// PreviousRune is meaningful only when HasPreviousRune is true (NUL is valid).
type EditorTextPosition struct {
	Line, RuneColumn, ByteColumn, UTF16Column int
	PreviousRune                              rune
	HasPreviousRune                           bool
}

// LineText reads one current line without copying the document's line slice.
// An empty valid line returns ("", true); invalid indices return ("", false).
// The returned string is immutable and remains valid after subsequent edits.
// Like all editor accessors, call on the UI thread, not from an LSP worker.
func (e *CodeEditor) LineText(line int) (string, bool) {
	if line < 0 || line >= len(e.lines) {
		return "", false
	}
	return e.lines[line], true
}

// TextPositionAt converts a rune position in the current unsaved buffer. Invalid
// lines return false; columns clamp to [0, line rune count]. It does not move the
// caret, publish revisions, allocate, scan other lines or scan past the requested
// column. Time is O(prefix bytes), not O(document size). UI-thread only.
func (e *CodeEditor) TextPositionAt(line, column int) (EditorTextPosition, bool) {
	text, ok := e.LineText(line)
	if !ok {
		return EditorTextPosition{}, false
	}
	p := textPositionInLine(text, column)
	p.Line = line
	return p, true
}

// CursorTextPosition snapshots the primary caret in all three coordinate units
// with one prefix traversal. In particular, LSP consumers can inspect the
// preceding code point without splitting Text() or allocating []rune(line).
func (e *CodeEditor) CursorTextPosition() (EditorTextPosition, bool) {
	return e.TextPositionAt(e.cursorLine, e.cursorCol)
}

func textPositionInLine(line string, column int) (p EditorTextPosition) {
	for p.ByteColumn < len(line) && p.RuneColumn < column {
		r, width := utf8.DecodeRuneInString(line[p.ByteColumn:])
		p.ByteColumn += width
		p.RuneColumn++
		p.UTF16Column++
		if r > 0xffff {
			p.UTF16Column++
		}
		p.PreviousRune, p.HasPreviousRune = r, true
	}
	return
}

// UTF16ColumnAt converts a zero-based rune column in the current buffer without
// copying every line. Invalid lines return zero; columns clamp to the line.
func (this *CodeEditor) UTF16ColumnAt(line, column int) int {
	if line < 0 || line >= len(this.lines) {
		return 0
	}
	return utf16ColumnOf(this.lines[line], max(0, column))
}

// RuneColumnFromUTF16 converts a zero-based UTF-16 code-unit offset (as used by
// LSP) into a zero-based Unicode code-point column (as used by CodeEditor).
// Negative offsets clamp to zero; offsets beyond the line clamp to its end.
// An offset in the middle of a surrogate pair rounds down to that code point.
// Invalid UTF-8 bytes count as replacement characters, matching Go's range.
// The conversion does not allocate, including for very long source lines.
func RuneColumnFromUTF16(line string, column int) int {
	if column <= 0 {
		return 0
	}
	units, runes := 0, 0
	for _, r := range line {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if column-units < n {
			return runes
		}
		units += n
		runes++
		if units == column {
			return runes
		}
	}
	return runes
}

// ScrollToUTF16Position navigates to a zero-based line and UTF-16 column, using
// the current buffer (including unsaved edits), not the file on disk. Like all
// CodeEditor methods, call it on the UI thread. Out-of-range lines clamp to the
// nearest line, and columns clamp as documented by RuneColumnFromUTF16.
func (this *CodeEditor) ScrollToUTF16Position(line, column int) {
	if len(this.lines) == 0 {
		return
	}
	if line < 0 {
		line = 0
	} else if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	this.ScrollToLineCol(line, RuneColumnFromUTF16(this.lines[line], column))
}

// NavigateToUTF16Position is the explicit-jump variant of ScrollToUTF16Position:
// it reveals folds and clears stale selections/carets before focusing the target.
func (this *CodeEditor) NavigateToUTF16Position(line, column int) {
	if len(this.lines) == 0 {
		return
	}
	line = max(0, min(len(this.lines)-1, line))
	this.NavigateToLineCol(line, RuneColumnFromUTF16(this.lines[line], column))
}

// NavigateToLineCol activates a single insertion point for an explicit user jump
// (for example an outline row). Coordinates are zero-based Unicode rune columns,
// not LSP UTF-16 units. It reveals collapsed ancestors, clears old selections and
// secondary carets, dismisses completion, and returns keyboard focus to the editor.
// It never edits text or creates undo history. Use ScrollToLineCol instead when
// the caller intentionally wants to retain selection/focus. Call on the UI thread.
func (this *CodeEditor) NavigateToLineCol(line, column int) {
	this.DismissLanguageHints()
	if len(this.lines) == 0 {
		return
	}
	line = max(0, min(len(this.lines)-1, line))
	this.revealCaretLine(line)
	this.clearSelection()
	this.additionalCursors = nil
	if this.completion != nil {
		this.completion.visible = false
	}
	this.ScrollToLineCol(line, column)
	this.ensureCursorVisible()
	this.SetFocus()
	this.Self().Update()
}
