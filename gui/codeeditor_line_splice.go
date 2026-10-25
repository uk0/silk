package gui

import (
	"sort"
	"strings"
)

// Line splices are sorted and disjoint in the pre-edit document. History owns
// only the affected strings, never a slice backed by the full document's lines.
// Applying any number of splices rebuilds the line index once, not once per caret.
type editorLineSplice struct {
	at            int
	before, after []string
}
type editorLineGroup struct{ lo, hi int }

func (e *CodeEditor) editorLineGroups(includeSentinel bool) []editorLineGroup {
	count := len(e.lines)
	if !includeSentinel && count > 1 && e.lines[count-1] == "" {
		count--
	}
	if e.hasSelection {
		lo, _, hi, col := e.selectionRange()
		if hi > lo && col == 0 {
			hi--
		}
		if lo < 0 || hi >= count || lo > hi {
			return nil
		}
		return []editorLineGroup{{lo, hi}}
	}
	rows := make([]int, 0, len(e.additionalCursors)+1)
	add := func(line int) {
		if line >= 0 && line < count {
			rows = append(rows, line)
		}
	}
	add(e.cursorLine)
	for _, p := range e.additionalCursors {
		add(p.line)
	}
	sort.Ints(rows)
	var groups []editorLineGroup
	for _, row := range rows {
		if len(groups) > 0 && row <= groups[len(groups)-1].hi+1 {
			groups[len(groups)-1].hi = row
		} else {
			groups = append(groups, editorLineGroup{row, row})
		}
	}
	return groups
}
func cloneLineBodies(lines []string) []string {
	out := make([]string, len(lines))
	for i, s := range lines {
		out[i] = strings.Clone(s)
	}
	return out
}

type editorLineSpliceStep struct {
	at, remove, shift int
	put               []string
}

func lineSpliceSteps(changes []editorLineSplice, reverse bool) ([]editorLineSpliceStep, int) {
	steps := make([]editorLineSpliceStep, 0, len(changes))
	shift := 0
	for _, c := range changes {
		at, remove, put := c.at, len(c.before), c.after
		if reverse {
			// Inverse deltas negate the original prefix shift.
			at -= shift
			remove, put = len(c.after), c.before
		}
		steps = append(steps, editorLineSpliceStep{at, remove, shift, put})
		shift += len(put) - remove
	}
	return steps, shift
}
func (e *CodeEditor) applyLineSplices(changes []editorLineSplice, reverse bool) {
	steps, delta := lineSpliceSteps(changes, reverse)
	if len(e.foldedLines) > 0 {
		folded := make(map[int]bool, len(e.foldedLines))
		for _, r := range e.FoldRegions() {
			if !e.foldedLines[r.startLine] {
				continue
			}
			i := sort.Search(len(steps), func(i int) bool { return steps[i].at+steps[i].remove > r.startLine })
			if i < len(steps) && steps[i].at <= r.endLine {
				continue
			} // touched fold expands
			shift := delta
			if i < len(steps) {
				shift = steps[i].shift
			}
			folded[r.startLine+shift] = true
		}
		e.foldedLines = folded
		e.invalidateVisLineCache()
	}
	lines := make([]string, 0, len(e.lines)+delta)
	start := 0
	for _, s := range steps {
		lines = append(lines, e.lines[start:s.at]...)
		lines = append(lines, s.put...)
		start = s.at + s.remove
	}
	e.lines = append(lines, e.lines[start:]...)
	e.textDirty = true
}
func (e *CodeEditor) commitLineSplices(changes []editorLineSplice, before, after []cursorPos, selection, next editorSelection) {
	if len(changes) == 0 {
		return
	}
	e.prepareCaretCommand()
	e.applyLineSplices(changes, false)
	for i, p := range after {
		after[i] = e.clampMovedPosition(p)
	}
	after = dedupCursorsList(after)
	e.restoreEditCursors(after)
	if next.active {
		next.start = e.clampMovedPosition(next.start)
		next.end = e.clampMovedPosition(next.end)
	}
	e.restoreSelection(next)
	e.pushUndo(editAction{kind: 6, lineSplices: changes, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, next}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
}

// DuplicateLines copies the selected logical block or each contiguous group of
// caret lines below itself. An endpoint at the next line's column zero excludes
// that line. Carets and selection direction follow the copies; Undo restores
// everything in one transaction. Empty lines (including the EOF row) can be
// duplicated. New EOF separators follow the preceding line, or LF for one line.
// All CodeEditor command and availability APIs are UI-thread-only.
func (e *CodeEditor) DuplicateLines() {
	if e.readOnly {
		return
	}
	groups := e.editorLineGroups(true)
	if len(groups) == 0 {
		return
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	changes := make([]editorLineSplice, 0, len(groups))
	offsets := make([]int, len(groups))
	added := 0
	for i, g := range groups {
		count := g.hi - g.lo + 1
		offsets[i] = added
		added += count
		block := cloneLineBodies(e.lines[g.lo : g.hi+1])
		if g.hi == len(e.lines)-1 {
			suffix := ""
			if g.hi > 0 && strings.HasSuffix(e.lines[g.hi-1], "\r") {
				suffix = "\r"
			}
			// The old EOF body needs a separator; the copied EOF stays unterminated.
			changes = append(changes, editorLineSplice{at: g.hi, before: []string{block[len(block)-1]}, after: append([]string{block[len(block)-1] + suffix}, block...)})
		} else {
			changes = append(changes, editorLineSplice{at: g.hi + 1, after: block})
		}
	}
	after := make([]cursorPos, len(before))
	for j, p := range before {
		i := sort.Search(len(groups), func(i int) bool { return groups[i].hi >= p.line })
		shift := added
		if i < len(groups) {
			shift = offsets[i]
			if p.line >= groups[i].lo {
				shift += groups[i].hi - groups[i].lo + 1
			}
		}
		p.line += shift
		after[j] = p
	}
	next := selection
	if next.active {
		span := groups[0].hi - groups[0].lo + 1
		next.start.line += span
		next.end.line += span
		if before[0] == selection.start {
			after[0] = next.start
		}
		if before[0] == selection.end {
			after[0] = next.end
		}
	}
	e.commitLineSplices(changes, before, after, selection, next)
}
func (e *CodeEditor) CanDuplicateLines() bool {
	return !e.readOnly && len(e.editorLineGroups(true)) > 0
}

// DeleteLines removes the selected logical block or every distinct caret row.
// Carets on removed rows collapse to the next surviving row (or the last row at
// EOF), column zero; duplicate resulting carets merge. Selection is cleared.
// Undo restores original bytes, all carets and selection direction. Deleting the
// last row removes its preceding separator; deleting all rows leaves one empty
// row. An already empty document is a no-op, including its history and revision.
func (e *CodeEditor) DeleteLines() {
	if !e.readOnly {
		e.deleteLineGroups(e.editorLineGroups(true))
	}
}
func (e *CodeEditor) CanDeleteLines() bool {
	return !e.readOnly && !(len(e.lines) == 1 && e.lines[0] == "") && len(e.editorLineGroups(true)) > 0
}
func (e *CodeEditor) deleteLineGroups(groups []editorLineGroup) {
	if len(groups) == 0 || len(e.lines) == 1 && e.lines[0] == "" {
		return
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	changes := make([]editorLineSplice, 0, len(groups))
	offsets := make([]int, len(groups))
	removed := 0
	for i, g := range groups {
		offsets[i] = removed
		removed += g.hi - g.lo + 1
		c := editorLineSplice{at: g.lo}
		if g.hi == len(e.lines)-1 {
			if g.lo == 0 {
				c.after = []string{""}
			} else {
				c.at = g.lo - 1
				c.after = []string{strings.Clone(strings.TrimSuffix(e.lines[c.at], "\r"))}
			}
		}
		c.before = cloneLineBodies(e.lines[c.at : g.hi+1])
		changes = append(changes, c)
	}
	after := make([]cursorPos, len(before))
	for j, p := range before {
		i := sort.Search(len(groups), func(i int) bool { return groups[i].hi >= p.line })
		shift := removed
		if i < len(groups) {
			shift = offsets[i]
			if p.line >= groups[i].lo {
				p.line = groups[i].lo
				p.col = 0
			}
		}
		p.line -= shift
		after[j] = p
	}
	if selection.active {
		after[0] = cursorPos{groups[0].lo, 0}
	}
	e.commitLineSplices(changes, before, after, selection, editorSelection{})
}
