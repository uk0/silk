package gui

import (
	"fmt"
	"sort"
	"strings"
)

// InsertSpaces selects soft tabs for future Tab/Enter/line-indent operations.
// It never rewrites existing text, pasted content, snippets or formatter output.
// Like the rest of CodeEditor, these accessors are UI-thread-only.
func (e *CodeEditor) InsertSpaces() bool { return e.insertSpaces }
func (e *CodeEditor) SetInsertSpaces(value bool) {
	if e.insertSpaces != value {
		e.insertSpaces = value
		e.Update()
	}
}
func (e *CodeEditor) indentUnit() string {
	if e.insertSpaces {
		return strings.Repeat(" ", e.TabSize())
	}
	return "\t"
}
func (e *CodeEditor) indentationLabel() string {
	mode := "Tabs"
	if e.insertSpaces {
		mode = "Spaces"
	}
	return fmt.Sprintf("%s: %d", mode, e.TabSize())
}

// tabText aligns soft Tab with the next logical tab stop. Each non-tab code
// point (including an invalid UTF-8 byte) occupies one logical column, matching
// editor caret/minimap columns, not font pixel width or terminal wcwidth.
func (e *CodeEditor) tabText(line string, col int) string {
	if !e.insertSpaces {
		return "\t"
	}
	width, logical, runes := e.TabSize(), 0, 0
	for _, r := range line {
		if runes >= col {
			break
		}
		if r == '\t' {
			logical += width - logical%width
		} else {
			logical++
		}
		runes++
	}
	return strings.Repeat(" ", width-logical%width)
}

// Selection snapshots are optional: older edit kinds keep their existing
// selection-clearing contract. Line indentation restores selection direction.
type editorSelectionHistory struct{ before, after editorSelection }
type editorSelection struct {
	active     bool
	start, end cursorPos
}

func (e *CodeEditor) selectionSnapshot() editorSelection {
	return editorSelection{e.hasSelection, cursorPos{e.selStartLine, e.selStartCol}, cursorPos{e.selEndLine, e.selEndCol}}
}
func (e *CodeEditor) restoreSelection(s editorSelection) {
	e.hasSelection = s.active
	e.selStartLine, e.selStartCol = s.start.line, s.start.col
	e.selEndLine, e.selEndCol = s.end.line, s.end.col
}

// IndentLines/OutdentLines change the selected logical lines, or each distinct
// caret line, in one undoable transaction. A selection ending at column zero
// excludes that last line. Only changed prefixes are retained in edit history.
func (e *CodeEditor) IndentLines()  { e.changeLineIndent(false) }
func (e *CodeEditor) OutdentLines() { e.changeLineIndent(true) }
func (e *CodeEditor) changeLineIndent(remove bool) {
	if e.readOnly {
		return
	}
	if len(e.lines) == 0 {
		return
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	var lines []int
	if e.hasSelection {
		sl, _, el, ec := e.selectionRange()
		if el > sl && ec == 0 {
			el--
		}
		// A selection is contiguous: build descending anchors directly, without
		// an O(n log n) sort or a hash entry for every selected line.
		for i := min(el, len(e.lines)-1); i >= max(0, sl); i-- {
			lines = append(lines, i)
		}
	} else {
		seen := make(map[int]bool, len(before))
		for _, p := range before {
			if p.line >= 0 && p.line < len(e.lines) && !seen[p.line] {
				lines = append(lines, p.line)
				seen[p.line] = true
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(lines)))
	}
	edits := make([]editorByteEdit, 0, len(lines))
	// Only caret/selection anchors need a delta; history holds other prefixes.
	deltas := make(map[int]int, len(before)+2)
	for _, p := range before {
		deltas[p.line] = 0
	}
	if selection.active {
		deltas[selection.start.line] = 0
		deltas[selection.end.line] = 0
	}
	unit, width := e.indentUnit(), e.TabSize()
	for _, line := range lines {
		edit := editorByteEdit{line: line, after: unit}
		if remove {
			n := indentRemoval(e.lines[line], width)
			if n == 0 {
				continue
			}
			edit.before, edit.after = strings.Clone(e.lines[line][:n]), ""
		}
		edits = append(edits, edit)
		if _, needed := deltas[line]; needed {
			deltas[line] = len(edit.after) - len(edit.before)
		}
	}
	if len(edits) == 0 {
		return
	}
	e.DismissLanguageHints()
	if e.completion != nil {
		e.completion.Dismiss()
	}
	e.snippetSess = nil
	for _, edit := range edits {
		e.replaceByteRange(edit.line, 0, len(edit.before), edit.after)
	}
	shift := func(p cursorPos) cursorPos { p.col = max(0, p.col+deltas[p.line]); return p }
	after := make([]cursorPos, len(before))
	for i, p := range before {
		after[i] = shift(p)
	}
	after = dedupCursorsList(after)
	e.restoreEditCursors(after)
	next := selection
	next.start, next.end = shift(next.start), shift(next.end)
	if next.active {
		e.restoreSelection(next)
	}
	e.pushUndo(editAction{kind: 4, group: edits, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, e.selectionSnapshot()}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
}
func indentRemoval(line string, width int) int {
	if strings.HasPrefix(line, "\t") {
		return 1
	}
	n := 0
	for n < len(line) && n < width && line[n] == ' ' {
		n++
	}
	return n
}

func (e *CodeEditor) insertIndentTab() {
	if e.readOnly {
		return
	}
	if len(e.additionalCursors) > 0 && !e.hasSelection {
		e.editAtAllCursors(byteCursorTab, "")
		e.rebuildText()
		e.ensureCursorVisible()
		e.Update()
		return
	}
	line, col := e.cursorLine, e.cursorCol
	if e.hasSelection {
		line, col, _, _ = e.selectionRange()
	}
	if line < 0 || line >= len(e.lines) {
		return
	}
	text := e.tabText(e.lines[line], col)
	// Keep ordinary Tab semantics (replace a single-line selection), but make
	// the key a transaction boundary rather than coalescing it into older typing.
	if e.hasSelection {
		e.replaceSelectionRecorded(text)
	} else {
		before := []cursorPos{{line, col}}
		column := textPositionInLine(e.lines[line], col).ByteColumn
		p := e.replaceByteRange(line, column, 0, text)
		e.cursorLine, e.cursorCol = p.line, p.col
		e.pushUndo(editAction{kind: 4, group: []editorByteEdit{{line: line, column: column, after: text}}, beforeCursors: before, afterCursors: []cursorPos{p}})
	}
	e.clearSelection()
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
}
