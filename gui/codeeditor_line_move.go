package gui

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// editorLineMove rotates [lo, hi] one row. Separators stay in their original
// slots, including mixed LF/CRLF and an unterminated final line. The bitset
// records separator identity (not text); it also disambiguates a literal CR at
// EOF when that body moves before an LF. History never retains document bodies.
type editorLineMove struct {
	lo, hi int
	down   bool
	crlf   []uint64
}

// MoveLinesUp/Down move the selected logical block, or each distinct contiguous
// group of caret lines, in one undoable transaction. Boundary groups stay put;
// other groups may move. The empty sentinel after a final newline is not a row
// to swap with. Selection direction, caret identities and final newline survive.
// Like all CodeEditor mutation APIs, these methods are UI-thread-only.
func (e *CodeEditor) MoveLinesUp()           { e.moveLines(false) }
func (e *CodeEditor) MoveLinesDown()         { e.moveLines(true) }
func (e *CodeEditor) CanMoveLinesUp() bool   { return len(e.lineMovePlan(false)) > 0 }
func (e *CodeEditor) CanMoveLinesDown() bool { return len(e.lineMovePlan(true)) > 0 }

func (e *CodeEditor) lineMovePlan(down bool) []editorLineMove {
	if e.readOnly {
		return nil
	}
	count := len(e.lines)
	if count > 1 && e.lines[count-1] == "" {
		count--
	}
	var result []editorLineMove
	for _, g := range e.editorLineGroups(false) {
		lo, hi := g.lo-1, g.hi
		if down {
			lo, hi = g.lo, g.hi+1
		}
		if lo >= 0 && hi < count {
			result = append(result, editorLineMove{lo: lo, hi: hi, down: down})
		}
	}

	return result
}

func (m *editorLineMove) captureSeparators(lines []string) {
	for i := m.lo; i <= m.hi; i++ {
		if i < len(lines)-1 && strings.HasSuffix(lines[i], "\r") {
			if m.crlf == nil {
				m.crlf = make([]uint64, (m.hi-m.lo+64)/64)
			}
			bit := i - m.lo
			m.crlf[bit/64] |= uint64(1) << uint(bit%64)
		}
	}
}
func (m editorLineMove) hasCR(slot int) bool {
	bit := slot - m.lo
	return len(m.crlf) > 0 && m.crlf[bit/64]&(uint64(1)<<uint(bit%64)) != 0
}
func (m editorLineMove) transfer(text string, from, to int) string {
	source, target := m.hasCR(from), m.hasCR(to)
	if source == target {
		return text
	}
	if source {
		return text[:len(text)-1]
	}
	return text + "\r"
}
func (m editorLineMove) apply(lines []string, reverse bool) {
	if m.down != reverse {
		saved := lines[m.hi]
		for i := m.hi; i > m.lo; i-- {
			lines[i] = m.transfer(lines[i-1], i-1, i)
		}
		lines[m.lo] = m.transfer(saved, m.hi, m.lo)
	} else {
		saved := lines[m.lo]
		for i := m.lo; i < m.hi; i++ {
			lines[i] = m.transfer(lines[i+1], i+1, i)
		}
		lines[m.hi] = m.transfer(saved, m.lo, m.hi)
	}
}
func movedLine(line int, moves []editorLineMove) int {
	i := sort.Search(len(moves), func(i int) bool { return moves[i].hi >= line })
	if i == len(moves) || line < moves[i].lo {
		return line
	}
	m := moves[i]
	if m.down {
		if line == m.hi {
			return m.lo
		}
		return line + 1
	}
	if line == m.lo {
		return m.hi
	}
	return line - 1
}
func (e *CodeEditor) clampMovedPosition(p cursorPos) cursorPos {
	p.line = max(0, min(p.line, len(e.lines)-1))
	p.col = textPositionInLine(e.lines[p.line], p.col).RuneColumn
	return p
}
func (e *CodeEditor) unfoldLineMoves(moves []editorLineMove) {
	if len(e.foldedLines) == 0 {
		return
	}
	for _, region := range e.FoldRegions() {
		i := sort.Search(len(moves), func(i int) bool { return moves[i].hi >= region.startLine })
		if i < len(moves) && moves[i].lo <= region.endLine {
			delete(e.foldedLines, region.startLine)
		}
	}
	e.invalidateVisLineCache()
}
func (e *CodeEditor) moveLines(down bool) {
	moves := e.lineMovePlan(down)
	if len(moves) == 0 {
		return
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	e.prepareCaretCommand()
	e.unfoldLineMoves(moves)
	for i := range moves {
		moves[i].captureSeparators(e.lines)
		moves[i].apply(e.lines, false)
	}
	after := make([]cursorPos, len(before))
	for i, p := range before {
		p.line = movedLine(p.line, moves)
		after[i] = e.clampMovedPosition(p)
	}
	next := selection
	if selection.active {
		delta := -1
		if down {
			delta = 1
		}
		shift := func(p cursorPos) cursorPos {
			p.line += delta
			if p.line >= len(e.lines) {
				p.line = len(e.lines) - 1
				p.col = utf8.RuneCountInString(e.lines[p.line])
			}
			return e.clampMovedPosition(p)
		}
		next.start, next.end = shift(selection.start), shift(selection.end)
		if before[0] == selection.end {
			after[0] = next.end
		} else if before[0] == selection.start {
			after[0] = next.start
		}
	}
	after = dedupCursorsList(after)
	e.restoreEditCursors(after)
	e.restoreSelection(next)
	e.pushUndo(editAction{kind: 5, lineMoves: moves, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, next}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
}
