package gui

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
	for _, region := range this.FoldRegions() {
		if region.startLine < line && line <= region.endLine && this.foldedLines[region.startLine] {
			delete(this.foldedLines, region.startLine)
			this.invalidateVisLineCache()
		}
	}
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
