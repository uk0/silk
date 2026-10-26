package gui

import (
	"slices"
	"sort"
)

// Navigation belongs to the interaction which requested it, not merely the
// document revision. Counts/highlights can still arrive after the user moves,
// but a delayed scan must not overwrite a newer caret, selection or focus.
// Captured only on UI events; no polling or worker access to widget state.
type editorFindNavigation struct {
	cursor        cursorPos
	selection     editorSelection
	carets        []cursorPos
	focus, parent IWidget
	owner         *Window
	revision      uint64
	documentFocus bool
}

func (e *CodeEditor) captureFindNavigation() editorFindNavigation {
	return editorFindNavigation{
		cursor: cursorPos{e.cursorLine, e.cursorCol}, selection: e.selectionSnapshot(),
		carets: slices.Clone(e.additionalCursors), focus: focusWidget, parent: e.Parent(),
		owner: e.OwnerWindow(), revision: e.TextRevision(), documentFocus: e.findState.documentFocus,
	}
}
func (n editorFindNavigation) current(e *CodeEditor) bool {
	return n.cursor == (cursorPos{e.cursorLine, e.cursorCol}) && n.selection == e.selectionSnapshot() &&
		slices.Equal(n.carets, e.additionalCursors) && n.focus == focusWidget && n.parent == e.Parent() &&
		n.owner == e.OwnerWindow() && n.revision == e.TextRevision() && n.documentFocus == e.findState.documentFocus
}
func (e *CodeEditor) queueFindJump(p cursorPos, strict bool) {
	s := &e.findState
	s.jump = &p
	s.step = 0
	s.jumpStrict = strict
	// Replacement callers queue before rebuildText increments the revision.
	// Capture the final interaction when that rebuild submits the scan.
	s.navigationFresh = true
}
func (e *CodeEditor) clearFindNavigation() {
	s := &e.findState
	s.jump = nil
	s.step = 0
	s.jumpStrict = false
	s.navigationFresh = false
	s.navigation = editorFindNavigation{}
}
func (e *CodeEditor) prepareFindNavigation() {
	s := &e.findState
	if s.jump == nil && s.step == 0 {
		return
	}
	if s.navigationFresh {
		s.navigation = e.captureFindNavigation()
		s.navigationFresh = false
	} else if !s.navigation.current(e) {
		e.clearFindNavigation()
	}
}

// Keep the frozen selection from opening Find separate from the match selected
// by navigation. Otherwise enabling "Selection" silently shrinks to one hit.
func (e *CodeEditor) ownsFindSelection() bool {
	return e.findState.matchSelection.active && e.ownsFindMatch()
}
func (e *CodeEditor) ownsFindMatch() bool {
	s := &e.findState
	return s.matchValid && s.matchRevision == e.TextRevision() && s.matchSelection == e.selectionSnapshot() && s.matchCaret == (cursorPos{e.cursorLine, e.cursorCol})
}

// A document click retires the old result index. Resume from the actual caret
// in sorted matches instead of jumping back to an unrelated old result.
func (e *CodeEditor) findNavigationIndex(step int) int {
	n := len(e.findMatches)
	forward := step > 0
	step %= n
	if e.ownsFindMatch() && e.findCurrentIdx >= 0 && e.findCurrentIdx < n {
		start, end := e.findMatchRange(e.findCurrentIdx)
		selected := e.findState.matchSelection
		if end == e.findState.matchCaret && ((!selected.active && start == end) || (selected.active && start == selected.start && end == selected.end)) {
			return (e.findCurrentIdx + step + n) % n
		}
	}
	index := sort.Search(n, func(i int) bool {
		m := e.findMatches[i]
		return m.line > e.cursorLine || m.line == e.cursorLine && m.col >= e.cursorCol
	})
	if forward {
		index--
	}
	return (index + step + n) % n
}
func (e *CodeEditor) activateFindMatch(index int) {
	if index < 0 || index >= len(e.findMatches) {
		return
	}
	e.breakTypingUndoGroup()
	e.findCurrentIdx = index
	start, end := e.findMatchRange(index)
	e.clearSelection()
	if start != end {
		e.hasSelection = true
		e.selStartLine, e.selStartCol = start.line, start.col
		e.selEndLine, e.selEndCol = end.line, end.col
	}
	e.cursorLine, e.cursorCol = end.line, end.col
	e.additionalCursors = nil
	e.findState.matchSelection = e.selectionSnapshot()
	e.findState.matchCaret = end
	e.findState.matchRevision = e.TextRevision()
	e.findState.matchValid = true
	e.ensureCursorVisible()
	e.syncFindBar()
	e.Self().Update()
}

func (e *CodeEditor) findMatchRange(index int) (cursorPos, cursorPos) {
	m := e.findMatches[index]
	start, end := cursorPos{m.line, m.col}, cursorPos{m.line, m.end}
	if len(e.findSpans) == len(e.findMatches) {
		span := e.findSpans[index]
		end = cursorPos{span.endLine, span.endCol}
	}
	return start, end
}
