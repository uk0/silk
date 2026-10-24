package gui

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

var ErrEditorFindSelection = errors.New("selection search needs a current selection; select text and reopen Find")

// Scope boundaries are byte positions in one immutable revision. The selected
// fragment is its own search subject, including regex anchors and word edges.
// Invalid scopes never silently fall back to searching the whole document.
type editorFindScope struct {
	enabled, valid bool
	start, end     int
	first, last    cursorPos
	revision, id   uint64
}
type editorFindScopeHistory struct{ before, after editorFindScope }

func (e *CodeEditor) captureFindSelection() editorFindScope {
	if !e.hasSelection {
		return editorFindScope{}
	}
	sl, sc, el, ec := e.selectionRange()
	if sl < 0 || el >= len(e.lines) || sl > el {
		return editorFindScope{}
	}
	a, b := textPositionInLine(e.lines[sl], sc), textPositionInLine(e.lines[el], ec)
	start := byteOffsetForCursor(e.lines, sl, a.RuneColumn)
	end := byteOffsetForCursor(e.lines, el, b.RuneColumn)
	if start >= end {
		return editorFindScope{}
	}
	return editorFindScope{valid: true, start: start, end: end, first: cursorPos{sl, a.RuneColumn}, last: cursorPos{el, b.RuneColumn}, revision: e.textRevision}
}

// SetFindInSelection freezes the current document selection (or the selection
// captured when ShowFind opened). Query edits and navigation keep this scope.
// Unrelated edits invalidate it rather than widening it. Find-owned replacements
// and their Undo/Redo track its extent. Call on the UI thread; false clears scope.
func (e *CodeEditor) SetFindInSelection(enabled bool) error {
	if !enabled {
		e.findState.scope = editorFindScope{}
	} else {
		if e.findState.scope.enabled && e.findState.scope.valid && e.findState.scope.revision == e.textRevision {
			return nil
		}
		s := e.captureFindSelection()
		if !s.valid {
			s = e.findState.selectionCandidate
		}
		if s.revision != e.textRevision {
			s.valid = false
		}
		e.findState.scopeSerial++
		s.id = e.findState.scopeSerial
		s.enabled = true
		e.findState.scope = s
	}
	e.findState.jump = &cursorPos{}
	e.findUpdateMatches()
	e.syncFindBar()
	e.Update()
	if enabled && !e.findState.scope.valid {
		return ErrEditorFindSelection
	}
	return nil
}
func (e *CodeEditor) findScopeTextChanged() {
	s := &e.findState
	s.selectionCandidate = editorFindScope{}
	if s.scope.enabled {
		if s.pendingScope != nil && s.pendingScope.id == s.scope.id && s.pendingScope.revision == e.textRevision {
			s.scope = *s.pendingScope
		} else {
			s.scope.valid = false
		}
	}
	s.pendingScope = nil
	s.scopeTransition = nil
}
func (e *CodeEditor) prepareFindScopeChange(r editorFindResult) {
	if !r.request.scope.enabled {
		return
	}
	h := &editorFindScopeHistory{before: r.request.scope, after: r.scopeAfter}
	e.findState.scopeTransition = h
	e.findState.pendingScope = &h.after
}
func (e *CodeEditor) restoreFindScopeHistory(h *editorFindScopeHistory, redo bool) {
	if h == nil || !e.findState.scope.enabled || e.findState.scope.id != h.before.id {
		return
	}
	s := h.before
	if redo {
		s = h.after
	}
	s.revision = e.textRevision + 1
	e.findState.pendingScope = &s
}
func (e *CodeEditor) findScopeStatus() string {
	s := e.findState.scope
	if !s.enabled {
		return ""
	}
	if !s.valid {
		return ErrEditorFindSelection.Error()
	}
	return fmt.Sprintf("Selection %d:%d–%d:%d (local anchors / word boundaries)", s.first.line+1, s.first.col+1, s.last.line+1, s.last.col+1)
}
func computeEditorScopedFind(ctx context.Context, r editorFindRequest) (result editorFindResult) {
	if r.options.Regex {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, editorRegexTimeLimit)
		defer cancel()
	}
	result.request = r
	result.request.text = ""
	scope := r.scope
	if !scope.valid || scope.revision != r.revision || scope.start < 0 || scope.end < scope.start || scope.end > len(r.text) {
		result.err = ErrEditorFindSelection
		return
	}
	inner := r
	inner.scope = editorFindScope{}
	inner.text = r.text[scope.start:scope.end]
	inner.spans = true
	if r.replace {
		inner.budgetSet = true
		inner.budget = editorReplacementBudget(r) - (len(r.text) - len(inner.text))
		if inner.budget < 0 {
			result.err = ErrEditorFindReplacementTooLarge
			return
		}
	}
	if r.one {
		inner.target.start -= scope.start
		inner.target.end -= scope.start
	}
	result = computeEditorFind(ctx, inner)
	result.request = r
	result.request.text = ""
	if result.err != nil {
		return
	}
	project := func(p cursorPos) cursorPos {
		if p.line == 0 {
			p.col += scope.first.col
		}
		p.line += scope.first.line
		return p
	}
	for i := range result.matches {
		if i&1023 == 0 && ctx.Err() != nil {
			result.err = ctx.Err()
			result.matches, result.spans, result.count = nil, nil, 0
			return
		}
		m := &result.matches[i]
		end := project(cursorPos{result.spans[i].endLine, result.spans[i].endCol})
		start := project(cursorPos{m.line, m.col})
		m.line, m.col, m.end = start.line, start.col, end.col
		span := &result.spans[i]
		span.start += scope.start
		span.end += scope.start
		span.endLine, span.endCol = end.line, end.col
	}
	if !r.replace || result.count == 0 {
		return
	}
	after := scope
	after.revision = r.revision + 1
	if r.one {
		after.end += len(result.text) - (r.target.end - r.target.start)
		p := editorRegexPosition{line: r.anchor.line, col: r.anchor.col}
		result.err = p.advance(ctx, result.text, len(result.text))
		after.last.line += p.line - r.target.endLine
		if scope.last.line == r.target.endLine {
			after.last.col += p.col - r.target.endCol
		}
		// Malformed bytes can join into a valid rune across an edit seam. The
		// byte edit is still exact, but old rune coordinates are no longer safe.
		pieces := []string{r.text[:r.target.start], result.text, r.text[r.target.end:]}
		after.valid = editorFindPieceBoundary(r.target.start, pieces...) && editorFindPieceBoundary(r.target.start+len(result.text), pieces...)
	} else {
		after.end = scope.start + len(result.text)
		p := editorRegexPosition{}
		result.err = p.advance(ctx, result.text, len(result.text))
		after.last = project(cursorPos{p.line, p.col})
		if result.err == nil {
			result.text = r.text[:scope.start] + result.text + r.text[scope.end:]
			after.valid = editorFindPieceBoundary(after.start, result.text) && editorFindPieceBoundary(after.end, result.text)
		}
	}
	if result.err != nil {
		result.text = ""
		result.count = 0
		return
	}
	result.scopeAfter = after
	return
}

// Check a boundary in a virtual concatenation without copying a large buffer.
// A valid UTF-8 rune can start at most three bytes before the boundary.
func editorFindPieceBoundary(pos int, pieces ...string) bool {
	byteAt := func(index int) (byte, bool) {
		for _, s := range pieces {
			if index < len(s) {
				return s[index], true
			}
			index -= len(s)
		}
		return 0, false
	}
	for start := max(0, pos-3); start < pos; start++ {
		var buf [utf8.UTFMax]byte
		n := 0
		for ; n < len(buf); n++ {
			b, ok := byteAt(start + n)
			if !ok {
				break
			}
			buf[n] = b
		}
		_, width := utf8.DecodeRune(buf[:n])
		if start+width > pos {
			return false
		}
	}
	return true
}
func editorReplacementBudget(r editorFindRequest) int {
	if r.budgetSet {
		return r.budget
	}
	return editorFindReplacementLimit
}

var errEditorFindOneDone = errors.New("single match located")

func computeEditorLiteralOne(ctx context.Context, r editorFindRequest) (result editorFindResult) {
	result.request = r
	result.request.text = ""
	result.err = ErrEditorFindStaleMatch
	err := scanEditorFind(ctx, r.text, r.query, r.options, func(_ findMatch, a, b int) error {
		if a < r.target.start {
			return nil
		}
		if a == r.target.start && b == r.target.end {
			if len(r.replacement) > editorReplacementBudget(r)-(len(r.text)-(b-a)) {
				result.err = ErrEditorFindReplacementTooLarge
			} else {
				result.err = nil
				result.text = r.replacement
				result.count = 1
			}
		}
		return errEditorFindOneDone
	})
	if err != nil && !errors.Is(err, errEditorFindOneDone) {
		result.err = err
		result.text = ""
		result.count = 0
	}
	return
}
