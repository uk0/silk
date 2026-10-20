package gui

// Document motion has one destination; merge secondary carets there rather
// than leaving them behind to insert text on unrelated lines after Ctrl+End.
func (editor *CodeEditor) moveToDocumentBoundary(end, extend bool) {
	editor.clampCursor()
	if extend && !editor.hasSelection {
		editor.setSelectionStart()
	}
	editor.additionalCursors = nil
	editor.cursorLine, editor.cursorCol = 0, 0
	if end {
		editor.cursorLine = len(editor.lines) - 1
		editor.cursorCol = len([]rune(editor.lines[editor.cursorLine]))
	}
	if extend {
		editor.updateSelectionEnd()
	} else {
		editor.clearSelection()
	}
	// A file ending in a collapsed brace region must reveal its last line.
	for _, region := range editor.FoldRegions() {
		if region.startLine < editor.cursorLine && editor.cursorLine <= region.endLine && editor.foldedLines[region.startLine] {
			delete(editor.foldedLines, region.startLine)
			editor.invalidateVisLineCache()
		}
	}
	editor.ensureCursorVisible()
	editor.Self().Update()
}
