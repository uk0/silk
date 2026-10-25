package gui

import (
	"slices"
	"sort"
	"strings"
	"time"
)

type editorBookmarkObserver struct{ notify func() }
type editorBookmarkHistory struct {
	before, after []int
	epoch         uint64
}
type editorBookmarkState struct {
	published       []int
	epoch           uint64
	mapped, pending bool
	observers       []*editorBookmarkObserver
}

// BookmarkLines returns a sorted, independent copy of the zero-based bookmarked
// rows. All bookmark APIs are UI-thread-only; annotations do not modify text,
// dirty state or text history. Text edits move marks with their original rows.
func (e *CodeEditor) BookmarkLines() []int      { return slices.Clone(e.bookmarkState.published) }
func (e *CodeEditor) HasBookmark(line int) bool { return e.bookmarks[line] }
func (e *CodeEditor) BookmarkCount() int        { return len(e.bookmarks) }

// SetBookmark sets or removes a mark. Invalid rows are rejected without changes;
// the result reports whether a valid request changed the set. Read-only buffers
// allow annotations and navigation. Removing a mark prevents text Undo from
// later resurrecting it (explicit annotation changes start a new epoch).
func (e *CodeEditor) SetBookmark(line int, marked bool) bool {
	if line < 0 || line >= len(e.lines) || e.HasBookmark(line) == marked {
		return false
	}
	if marked {
		if e.bookmarks == nil {
			e.bookmarks = make(map[int]bool)
		}
		e.bookmarks[line] = true
	} else {
		delete(e.bookmarks, line)
	}
	e.explicitBookmarksChanged()
	return true
}

// SetBookmarkLines replaces the marks atomically. Invalid rows reject the whole
// request; duplicates are merged. nil clears the set. Returns false on invalid
// input, true on success (including an unchanged set).
func (e *CodeEditor) SetBookmarkLines(lines []int) bool {
	for _, line := range lines {
		if line < 0 || line >= len(e.lines) {
			return false
		}
	}
	marks := make(map[int]bool, len(lines))
	for _, line := range lines {
		marks[line] = true
	}
	if bookmarkSetEqual(marks, e.bookmarkState.published) {
		return true
	}
	e.bookmarks = marks
	e.explicitBookmarksChanged()
	return true
}
func (e *CodeEditor) ClearBookmarks() { e.SetBookmarkLines(nil) }
func (e *CodeEditor) ToggleBookmark() {
	e.clampCursor()
	e.SetBookmark(e.cursorLine, !e.HasBookmark(e.cursorLine))
}
func (e *CodeEditor) explicitBookmarksChanged() {
	e.bookmarkState.epoch++
	// Do not coalesce typing across a user's annotation edit.
	if n := len(e.undoStack); n > 0 {
		e.undoStack[n-1].stamp = time.Time{}
	}
	e.publishBookmarks(false)
	e.Update()
}

// NextBookmark / PrevBookmark wrap within this buffer, reveal folded targets,
// retire secondary carets and selection, and focus the destination. They never
// edit the buffer. Sorted indices are cached at mutation time, not per jump.
func (e *CodeEditor) NextBookmark() {
	lines := e.bookmarkState.published
	if len(lines) == 0 {
		return
	}
	i := sort.Search(len(lines), func(i int) bool { return lines[i] > e.cursorLine })
	if i == len(lines) {
		i = 0
	}
	e.NavigateToLineCol(lines[i], 0)
}
func (e *CodeEditor) PrevBookmark() {
	lines := e.bookmarkState.published
	if len(lines) == 0 {
		return
	}
	i := sort.SearchInts(lines, e.cursorLine) - 1
	if i < 0 {
		i = len(lines) - 1
	}
	e.NavigateToLineCol(lines[i], 0)
}

// SubscribeBookmarksChanged observes explicit changes and complete text edits
// while marks exist (so hosts can refresh row context). No initial event, no idle
// polling. Callbacks run on the UI thread, must be short and must not mutate this
// editor synchronously. Unsubscribe is idempotent; detached observers are skipped
// even during delivery. SigChanged is not replaced.
func (e *CodeEditor) SubscribeBookmarksChanged(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	o := &editorBookmarkObserver{fn}
	e.bookmarkState.observers = append(e.bookmarkState.observers, o)
	return func() {
		if o.notify == nil {
			return
		}
		o.notify = nil
		for i, v := range e.bookmarkState.observers {
			if v == o {
				e.bookmarkState.observers = slices.Delete(e.bookmarkState.observers, i, i+1)
				return
			}
		}
	}
}
func bookmarkSetEqual(marks map[int]bool, lines []int) bool {
	if len(marks) != len(lines) {
		return false
	}
	for _, line := range lines {
		if !marks[line] {
			return false
		}
	}
	return true
}
func (e *CodeEditor) publishBookmarks(textChanged bool) {
	s := &e.bookmarkState
	changed := !bookmarkSetEqual(e.bookmarks, s.published)
	if changed {
		next := make([]int, 0, len(e.bookmarks))
		for line := range e.bookmarks {
			next = append(next, line)
		}
		sort.Ints(next)
		if s.pending && len(e.undoStack) > 0 {
			a := &e.undoStack[len(e.undoStack)-1]
			if a.bookmarks == nil || a.bookmarks.epoch != s.epoch {
				a.bookmarks = &editorBookmarkHistory{before: s.published, epoch: s.epoch}
			}
			a.bookmarks.after = next
		}
		s.published = next
	}
	s.pending, s.mapped = false, false
	if !changed && (!textChanged || len(e.bookmarks) == 0) {
		return
	}
	for _, o := range slices.Clone(s.observers) {
		if o.notify != nil {
			o.notify()
		}
	}
}
func (e *CodeEditor) restoreBookmarkHistory(h *editorBookmarkHistory, redo bool) {
	if h == nil || h.epoch != e.bookmarkState.epoch {
		return
	}
	rows := h.before
	if redo {
		rows = h.after
	}
	e.bookmarks = make(map[int]bool, len(rows))
	for _, row := range rows {
		e.bookmarks[row] = true
	}
	e.bookmarkState.mapped = true
}

// Map only annotations, not the document. No work/allocation on the common path
// with no marks or an inline edit. Deleted rows disappear; Undo restores them
// only while the annotation epoch still belongs to that edit.
func (e *CodeEditor) mapBookmarks(mapLine func(int) int) {
	e.bookmarkState.mapped = true
	if len(e.bookmarks) == 0 {
		return
	}
	next := make(map[int]bool, len(e.bookmarks))
	for line := range e.bookmarks {
		if target := mapLine(line); target >= 0 {
			next[target] = true
		}
	}
	e.bookmarks = next
}
func (e *CodeEditor) mapByteBookmarks(line, column, endLine, end, insertedLines int) {
	e.bookmarkState.mapped = true
	if line == endLine && insertedLines == 0 {
		return
	}
	e.mapBookmarks(func(row int) int {
		if row < line {
			return row
		}
		if row > endLine {
			return row + insertedLines - (endLine - line)
		}
		if row == line && column > 0 {
			return row
		}
		if row == endLine && (row == line || end < len(e.lines[row]) || end == 0) {
			return line + insertedLines
		}
		return -1
	})
}
func (e *CodeEditor) mapMoveBookmarks(m editorLineMove, reverse bool) {
	e.mapBookmarks(func(row int) int {
		if row < m.lo || row > m.hi {
			return row
		}
		if m.down != reverse {
			if row == m.hi {
				return m.lo
			}
			return row + 1
		}
		if row == m.lo {
			return m.hi
		}
		return row - 1
	})
}
func (e *CodeEditor) mapSpliceBookmarks(steps []editorLineSpliceStep, delta int) {
	e.mapBookmarks(func(row int) int {
		i := sort.Search(len(steps), func(i int) bool { return steps[i].at+steps[i].remove > row })
		if i == len(steps) {
			return row + delta
		}
		s := steps[i]
		if row < s.at {
			return row + s.shift
		}
		// EOF duplicate/delete rewrites an unchanged prefix to adjust its separator.
		for j := 0; j <= row-s.at && j < len(s.put); j++ {
			if strings.TrimSuffix(e.lines[s.at+j], "\r") != strings.TrimSuffix(s.put[j], "\r") {
				return -1
			}
			if j == row-s.at {
				return row + s.shift
			}
		}
		return -1
	})
}

// A host reload / whole-buffer provider replacement has no edit map. Preserve
// equal prefix/suffix rows exactly, then search nearest equal trimmed context
// within 200 rows; otherwise keep the old row, clamped into the new document.
// This fallback is deliberately heuristic, not semantic identity matching.
func (e *CodeEditor) reanchorBookmarks(oldText string) {
	if len(e.bookmarks) == 0 {
		return
	}
	old := strings.Split(oldText, "\n")
	prefix := 0
	for prefix < len(old) && prefix < len(e.lines) && old[prefix] == e.lines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(e.lines)-prefix && old[len(old)-1-suffix] == e.lines[len(e.lines)-1-suffix] {
		suffix++
	}
	e.mapBookmarks(func(row int) int {
		if row < prefix {
			return row
		}
		if row >= len(old)-suffix {
			return row + len(e.lines) - len(old)
		}
		target := max(0, min(row, len(e.lines)-1))
		if row < 0 || row >= len(old) {
			return target
		}
		want := strings.TrimSpace(old[row])
		if want == "" {
			return target
		}
		for d := 0; d <= 200; d++ {
			if up := target - d; up >= 0 && strings.TrimSpace(e.lines[up]) == want {
				return up
			}
			if down := target + d; down < len(e.lines) && strings.TrimSpace(e.lines[down]) == want {
				return down
			}
		}
		return target
	})
}
