package gui

import (
	"strings"
	"unicode/utf8"
)

// replaceByteRange is the storage primitive. Carets address decoded runes, but
// history addresses bytes: deleting a separator can join malformed UTF-8 into
// a valid rune, so replaying a rune count would delete unrelated source bytes.
// line/column are a byte position in the pre-edit buffer; count includes LF.
// Untouched bytes are copied verbatim. No callbacks/revisions are published.
func (e *CodeEditor) replaceByteRange(line, column, count int, text string) cursorPos {
	if line < 0 || line >= len(e.lines) {
		return cursorPos{e.cursorLine, e.cursorCol}
	}
	column = max(0, min(column, len(e.lines[line])))
	count = max(0, count)
	endLine, end := line, column
	for count > len(e.lines[endLine])-end {
		count -= len(e.lines[endLine]) - end
		if endLine == len(e.lines)-1 {
			count = 0
			end = len(e.lines[endLine])
			break
		}
		count-- // newline
		endLine++
		end = 0
	}
	end += count
	e.mapByteBookmarks(line, column, endLine, end, strings.Count(text, "\n"))
	before, after := e.lines[line][:column], e.lines[endLine][end:]
	e.textDirty = true
	if endLine == line && !strings.Contains(text, "\n") {
		updated := before + text + after
		e.lines[line] = updated
		return cursorPos{line, runeColumnAtByte(updated, len(before)+len(text))}
	}
	parts := strings.Split(text, "\n")
	replacement := make([]string, 0, len(e.lines)-(endLine-line)+len(parts)-1)
	replacement = append(replacement, e.lines[:line]...)
	for i, part := range parts {
		if i == 0 {
			part = before + part
		}
		if i == len(parts)-1 {
			part += after
		}
		replacement = append(replacement, part)
	}
	replacement = append(replacement, e.lines[endLine+1:]...)
	e.lines = replacement
	last := line + len(parts) - 1
	insertedEnd := len(e.lines[last]) - len(after)
	return cursorPos{last, runeColumnAtByte(e.lines[last], insertedEnd)}
}

func (e *CodeEditor) insertRawTextAtByte(line, column int, text string) {
	p := e.replaceByteRange(line, column, 0, text)
	e.cursorLine, e.cursorCol = p.line, p.col
}

// replaceSelectionRecorded keeps both the original bytes and the pre-edit byte
// anchor. It publishes only when its caller rebuilds the text, avoiding a
// transient deleted selection reaching LSP/outline consumers.
func (e *CodeEditor) replaceSelectionRecorded(text string) {
	e.replaceSelectionEdit(text, false)
}

func (e *CodeEditor) replaceSelectionEdit(text string, typing bool) {
	sl, sc, _, _ := e.selectionRange()
	original := e.SelectedText()
	column := textPositionInLine(e.lines[sl], sc).ByteColumn
	e.pushUndo(editAction{kind: 2, line: sl, col: sc, text: text, oldText: original, typing: typing && editorTypingInput(text)})
	p := e.replaceByteRange(sl, column, len(original), text)
	e.cursorLine, e.cursorCol = p.line, p.col
	e.clearSelection()
}

// Byte endpoints can lie inside a code point newly formed across a splice.
// Round to its end instead of counting a truncated prefix as invalid runes.
func runeColumnAtByte(line string, target int) int {
	target = max(0, min(target, len(line)))
	col, pos := 0, 0
	for pos < target {
		_, width := utf8.DecodeRuneInString(line[pos:])
		pos += width
		col++
	}
	return col
}
