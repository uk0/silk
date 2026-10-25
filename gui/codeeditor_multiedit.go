package gui

import (
	"sort"
	"strings"
	"unicode/utf8"
)

type editorByteEdit struct {
	line, column, offset int
	before, after        string
}

const (
	byteCursorInsert = iota
	byteCursorBackspace
	byteCursorDelete
	byteCursorEnter
	byteCursorTab
)

func lineByteStarts(lines []string) []int {
	starts := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		starts[i] = starts[i-1] + len(lines[i-1]) + 1
	}
	return starts
}

// cursorAtByteOffset rounds an interior UTF-8 offset forward to the containing
// code point's end. Splicing malformed byte sequences can create such an offset.
func cursorAtByteOffset(lines []string, starts []int, offset int) cursorPos {
	line := max(0, sort.Search(len(starts), func(i int) bool { return starts[i] > offset })-1)
	target := max(0, min(offset-starts[line], len(lines[line])))
	return cursorPos{line, runeColumnAtByte(lines[line], target)}
}

// editAtAllCursors records a bounded list of byte edits, not a whole-document
// snapshot per key. All anchors are computed BEFORE any mutation. Right-to-left
// application and reverse replay preserve positions even when lines are joined
// or malformed sequences become valid across an edit boundary.
func (e *CodeEditor) editAtAllCursors(mode int, text string) {
	if mode == byteCursorInsert && text == "" {
		return
	}
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	for i, p := range before {
		p.line = max(0, min(p.line, len(e.lines)-1))
		p.col = textPositionInLine(e.lines[p.line], p.col).RuneColumn
		before[i] = p
	}
	before = dedupCursorsList(before)
	starts := lineByteStarts(e.lines)
	offsets := make([]int, len(before))
	edits := make([]editorByteEdit, 0, len(before))
	for i, p := range before {
		line := e.lines[p.line]
		column := textPositionInLine(line, p.col).ByteColumn
		offsets[i] = starts[p.line] + column
		edit := editorByteEdit{line: p.line, column: column, offset: offsets[i]}
		switch mode {
		case byteCursorInsert:
			edit.after = text
		case byteCursorEnter:
			edit.after = editorNewlineWithIndent(line, p.col, e.indentUnit(), e.TabSize())
		case byteCursorTab:
			edit.after = e.tabText(line, p.col)
		case byteCursorBackspace:
			if column > 0 {
				_, width := utf8.DecodeLastRuneInString(line[:column])
				edit.column -= width
				edit.offset -= width
				edit.before = strings.Clone(line[edit.column:column])
			} else if p.line > 0 {
				edit.line--
				edit.column = len(e.lines[edit.line])
				edit.offset--
				edit.before = "\n"
			} else {
				continue
			}
		case byteCursorDelete:
			if column < len(line) {
				_, width := utf8.DecodeRuneInString(line[column:])
				edit.before = strings.Clone(line[column : column+width])
			} else if p.line < len(e.lines)-1 {
				edit.before = "\n"
			} else {
				continue
			}
		}
		edits = append(edits, edit)
	}
	if len(edits) == 0 {
		return
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].offset > edits[j].offset })
	// Normalized/deduplicated carets produce disjoint rune/newline ranges.
	for _, edit := range edits {
		e.replaceByteRange(edit.line, edit.column, len(edit.before), edit.after)
	}
	afterStarts := lineByteStarts(e.lines)
	after := make([]cursorPos, 0, len(before))
	for _, offset := range offsets {
		shifted := offset
		for _, edit := range edits {
			if offset < edit.offset {
				continue
			}
			if offset <= edit.offset+len(edit.before) {
				shifted += len(edit.after) - (offset - edit.offset)
			} else {
				shifted += len(edit.after) - len(edit.before)
			}
		}
		after = append(after, cursorAtByteOffset(e.lines, afterStarts, shifted))
	}
	after = dedupCursorsList(after)
	e.restoreEditCursors(after)
	e.pushUndo(editAction{kind: 4, group: edits, beforeCursors: before, afterCursors: after})
}

func (e *CodeEditor) restoreEditCursors(cursors []cursorPos) {
	if len(cursors) == 0 {
		return
	}
	e.cursorLine, e.cursorCol = cursors[0].line, cursors[0].col
	e.additionalCursors = append([]cursorPos(nil), cursors[1:]...)
}

func editorNewlineWithIndent(line string, col int, unit string, width int) string {
	indent := nextLineIndent(line, col, unit)
	column := textPositionInLine(line, col).ByteColumn
	after := strings.TrimLeft(line[column:], " \t")
	if strings.HasPrefix(after, "}") && len(indent) > 0 {
		if indent[len(indent)-1] == '\t' {
			indent = indent[:len(indent)-1]
		} else {
			for n := 0; n < width && len(indent) > 0 && indent[len(indent)-1] == ' '; n++ {
				indent = indent[:len(indent)-1]
			}
		}
	}
	return "\n" + indent
}
