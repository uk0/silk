package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type editorFindState struct {
	options                       EditorFindOptions
	request                       editorFindRequest
	generation                    uint64
	valid, pending, documentFocus bool
	count                         int
	err                           error
	worker                        *editorFindWorker
	post                          func(func()) // test dispatcher; production uses Post
	jump                          *cursorPos
	step                          int
	jumpStrict                    bool
	navigationFresh               bool
	navigation                    editorFindNavigation
	matchSelection                editorSelection
	matchCaret                    cursorPos
	matchRevision                 uint64
	matchValid                    bool
	scope, selectionCandidate     editorFindScope
	scopeSerial                   uint64
	pendingScope                  *editorFindScope
	scopeTransition               *editorFindScopeHistory
}

// EditorFindSnapshot describes search without exposing mutable ranges.
// Count includes all matches; Retained is the bounded navigation/highlight set.
type EditorFindSnapshot struct {
	Query                        string
	Options                      EditorFindOptions
	Count, Retained, Current     int
	Pending, Ready               bool
	Revision                     uint64
	Err                          error
	InSelection, SelectionValid  bool
	SelectionStart, SelectionEnd int // UTF-8 byte offsets in Revision
}

// FindSnapshot and the other find APIs must be called on the UI thread.
func (e *CodeEditor) FindSnapshot() EditorFindSnapshot {
	s := EditorFindSnapshot{Query: e.findText, Options: e.findState.options, Pending: e.findState.pending, Err: e.findState.err, Revision: e.textRevision, Ready: e.findRangesCurrent()}
	s.InSelection = e.findState.scope.enabled
	s.SelectionValid = s.InSelection && e.findState.scope.valid && e.findState.scope.revision == e.textRevision
	if s.SelectionValid {
		s.SelectionStart = e.findState.scope.start
		s.SelectionEnd = e.findState.scope.end
	}
	if s.Ready {
		s.Count, s.Retained = e.findState.count, len(e.findMatches)
		if s.Retained > 0 {
			s.Current = e.findCurrentIdx + 1
		}
	}
	return s
}

// Walk every ancestor: headless hosts and nested hidden pages have no Window.
func (e *CodeEditor) findVisible() bool {
	for w := e.Self(); w != nil; w = w.Parent() {
		if !w.IsVisible() {
			return false
		}
	}
	return true
}

func (e *CodeEditor) SetFindOptions(options EditorFindOptions) {
	if e.findState.options == options {
		return
	}
	e.findState.options = options
	e.findUpdateMatches()
	e.syncFindBar()
}
func (e *CodeEditor) SetFindQuery(query string) {
	e.findText = query
	e.findCursor = utf8.RuneCountInString(query)
	e.findCurrentIdx = 0
	e.queueFindJump(cursorPos{}, false)
	e.findUpdateMatches()
	e.syncFindBar()
}

// ShowFind opens a child-widget bar, optionally with its replacement row.
// Single-line selections seed the query; otherwise the previous query survives.
func (e *CodeEditor) ShowFind(replace bool) {
	if c := e.captureFindSelection(); c.valid && !e.ownsFindSelection() {
		e.findState.selectionCandidate = c
	}
	e.findActive = true
	e.replaceVisible = replace
	e.replaceFocused = replace
	e.findState.documentFocus = false
	if e.hasSelection {
		if s := e.SelectedText(); !strings.ContainsAny(s, "\r\n") {
			e.findText = s
			e.findCursor = utf8.RuneCountInString(s)
		}
	}
	e.ensureFindBar()
	e.findUpdateMatches()
	e.Layout()
	e.focusFindInput(replace, true)
	e.Update()
}

func (e *CodeEditor) findUpdateMatches() { e.requestFind(false) }
func (e *CodeEditor) requestFind(replace bool) int {
	return e.requestFindOperation(editorFindRequest{replace: replace})
}
func (e *CodeEditor) requestFindOperation(operation editorFindRequest) int {
	if operation.replace && e.readOnly {
		return 0
	}
	if !e.findVisible() {
		e.CancelFind()
		return 0
	}
	e.prepareFindNavigation()
	s := &e.findState
	s.generation++
	if s.worker != nil {
		s.worker.cancel()
	}
	r := editorFindRequest{text: e.Text(), query: e.findText, replacement: e.replaceText, options: s.options, revision: e.textRevision, generation: s.generation, replace: operation.replace, one: operation.one, target: operation.target, anchor: operation.anchor, scope: s.scope}
	s.request = r
	s.request.text = ""
	s.valid = false
	s.pending = true
	s.err = nil
	s.count = 0
	e.findMatches = nil
	e.findSpans = nil
	if (!r.options.Regex && len(r.text)+len(r.query)+len(r.replacement) <= editorFindAsyncBytes) || r.query == "" {
		result := computeEditorFind(context.Background(), r)
		e.acceptFindResult(result)
		return result.count
	}
	if s.worker == nil {
		post := s.post
		if post == nil {
			post = Post
		}
		var worker *editorFindWorker
		worker = newEditorFindWorker(post, func(r editorFindResult) {
			if e.findState.worker == worker {
				e.acceptFindResult(r)
			}
		})
		s.worker = worker
	}
	s.worker.request(r)
	e.syncFindBar()
	e.Update()
	return 0
}
func (e *CodeEditor) findRequestCurrent(r editorFindRequest) bool {
	s := &e.findState
	return e.findVisible() && s.pending && s.generation == r.generation && e.TextRevision() == r.revision && e.findText == r.query && s.options == r.options && s.scope == r.scope && (!r.replace || !e.readOnly && e.replaceText == r.replacement)
}
func (e *CodeEditor) acceptFindResult(result editorFindResult) {
	if !e.findVisible() {
		e.CancelFind()
		return
	}
	if !e.findRequestCurrent(result.request) {
		return
	}
	s := &e.findState
	s.pending = false
	s.err = result.err
	if result.err != nil {
		e.clearFindNavigation()
		e.syncFindBar()
		e.Update()
		return
	}
	if result.request.one {
		e.acceptRegexReplacement(result)
		return
	}
	if result.request.replace {
		if result.count > 0 && result.text != e.Text() {
			e.prepareFindScopeChange(result)
			e.ReplaceAllText(result.text)
		} else {
			e.findUpdateMatches()
		}
		return
	}
	s.valid = true
	s.count = result.count
	e.findMatches = result.matches
	e.findSpans = result.spans
	if e.findCurrentIdx < 0 || e.findCurrentIdx >= len(e.findMatches) {
		e.findCurrentIdx = 0
	}
	jump, step, strict := s.jump, s.step, s.jumpStrict
	navigate := s.navigation.current(e)
	e.clearFindNavigation()
	if navigate {
		if jump != nil {
			e.selectFindMatch(jump.line, jump.col, strict)
		}
		if step != 0 {
			e.moveFind(step)
		}
		if len(e.findMatches) == 0 && e.ownsFindSelection() {
			e.clearSelection()
			s.matchSelection = editorSelection{}
			s.matchValid = false
		}
	}
	e.syncFindBar()
	e.Update()
}
func (e *CodeEditor) findRangesCurrent() bool {
	s := &e.findState
	return e.findVisible() && s.valid && !s.pending && s.request.revision == e.TextRevision() && s.request.query == e.findText && s.request.options == s.options && s.request.scope == s.scope
}
func (e *CodeEditor) moveFind(step int) {
	if !e.findRangesCurrent() {
		s := &e.findState
		if !s.navigationFresh && !s.navigation.current(e) {
			e.clearFindNavigation()
		}
		s.step += step
		s.navigation = e.captureFindNavigation()
		s.navigationFresh = false
		if !s.pending {
			e.findUpdateMatches()
		}
		return
	}
	if len(e.findMatches) == 0 {
		return
	}
	e.activateFindMatch(e.findNavigationIndex(step))
}
func (e *CodeEditor) findNext() { e.moveFind(1) }
func (e *CodeEditor) findPrev() { e.moveFind(-1) }

// CancelFind cancels computation and queued deliveries without waiting on the UI
// thread. Hosts must call it when disposing an editor / stopping the event loop.
// A later ShowFind or query edit may start a fresh worker.
func (e *CodeEditor) CancelFind() {
	s := &e.findState
	s.generation++
	s.pending = false
	s.valid = false
	e.clearFindNavigation()
	s.matchSelection = editorSelection{}
	s.matchValid = false
	s.count = 0
	s.err = nil
	e.findMatches = nil
	e.findSpans = nil
	if s.worker != nil {
		s.worker.close()
		s.worker = nil
	}
	if e.findBar != nil {
		e.findBar.tooltips(false)
		e.findBar.status.SetText(e.findStatusText())
		for _, b := range []*editorFindButton{e.findBar.prev, e.findBar.next, e.findBar.one, e.findBar.all} {
			b.SetEnabled(false)
		}
	}
}
func (e *CodeEditor) OnHide() {
	e.closeEditorContextMenu()
	e.CancelFind()
	e.CancelFormat()
	e.hideGoToLine(false)
}
func (e *CodeEditor) OnShow() {
	if e.findActive {
		e.findUpdateMatches()
		e.syncFindBar()
	}
}
func (e *CodeEditor) closeFindBar() {
	e.CancelFind()
	e.findState.scope = editorFindScope{}
	e.findState.selectionCandidate = editorFindScope{}
	e.findActive = false
	e.replaceVisible = false
	e.replaceFocused = false
	e.findMatches = nil
	e.findState.count = 0
	e.findState.err = nil
	e.findBarHeight = codeEditorFindRowHeight
	if e.findBar != nil {
		e.findBar.Hide()
	}
	e.SetFocus()
	e.Layout()
	e.Update()
}
func (e *CodeEditor) toggleReplaceRow() {
	e.replaceVisible = !e.replaceVisible
	e.replaceFocused = e.replaceVisible
	e.findState.documentFocus = false
	e.ensureFindBar()
	e.Layout()
	e.focusFindInput(e.replaceVisible, false)
	e.Update()
}
func (e *CodeEditor) findStatusText() string {
	s := e.FindSnapshot()
	if s.Err != nil {
		return s.Err.Error()
	}
	if s.Pending {
		if e.findState.request.replace {
			return "Replacing…"
		}
		return "Searching…"
	}
	if !s.Ready {
		return "Search paused"
	}
	if s.Count > s.Retained {
		return fmt.Sprintf("%d / %d (first %d)", s.Current, s.Count, s.Retained)
	}
	return fmt.Sprintf("%d / %d", s.Current, s.Count)
}

// Search highlights are sorted; never visit every match once per visible line.
// Clip rune columns before shaping prefixes, particularly on minified files.
func (e *CodeEditor) visibleFindRange(line int, left, right float64) (int, int) {
	ms := e.findMatches
	if len(e.findSpans) == len(ms) && e.findState.options.Regex {
		a := e.lineColumnAtX(line, left)
		b := e.lineColumnAtX(line, right) + 1
		lo := sort.Search(len(ms), func(i int) bool { s := e.findSpans[i]; return s.endLine > line || (s.endLine == line && s.endCol >= a) })
		hi := sort.Search(len(ms), func(i int) bool { return ms[i].line > line || (ms[i].line == line && ms[i].col > b) })
		return min(lo, hi), hi
	}
	lo := sort.Search(len(ms), func(i int) bool { return ms[i].line >= line })
	hi := lo + sort.Search(len(ms)-lo, func(i int) bool { return ms[lo+i].line > line })
	if lo == hi {
		return lo, hi
	}
	a := e.lineColumnAtX(line, left)
	b := e.lineColumnAtX(line, right) + 1
	lo += sort.Search(hi-lo, func(i int) bool { return ms[lo+i].end > a })
	hi = lo + sort.Search(hi-lo, func(i int) bool { return ms[lo+i].col > b })
	return lo, hi
}

func (e *CodeEditor) findHighlightColumns(index, line int) (int, int) {
	a, b := e.findHighlightSpan(index, line)
	if b < 0 {
		b = utf8.RuneCountInString(e.lines[line])
	}
	return a, b
}
func (e *CodeEditor) findHighlightSpan(index, line int) (int, int) {
	m := e.findMatches[index]
	if len(e.findSpans) != len(e.findMatches) || !e.findState.options.Regex {
		return m.col, m.end
	}
	s := e.findSpans[index]
	a, b := 0, s.endCol
	if m.line == line {
		a = m.col
	}
	if s.endLine != line {
		b = -1
	}
	return a, b
}
func (e *CodeEditor) acceptRegexReplacement(result editorFindResult) {
	r := result.request
	old := e.Text()[r.target.start:r.target.end]
	p := cursorPos{r.target.endLine, r.target.endCol}
	// An empty hit must advance strictly beyond the insertion position, or
	// repeated Replace on ^ / \b would insert forever at the same caret.
	strict := r.target.start == r.target.end
	if old != result.text {
		e.prepareFindScopeChange(result)
		e.pushUndo(editAction{kind: 2, line: r.anchor.line, col: r.anchor.col, text: result.text, oldText: old})
		column := textPositionInLine(e.lines[r.anchor.line], r.anchor.col).ByteColumn
		p = e.replaceByteRange(r.anchor.line, column, len(old), result.text)
		e.cursorLine, e.cursorCol = p.line, p.col
		e.queueFindJump(p, strict)
		e.rebuildText()
		if e.findActive {
			return
		}
	}
	e.queueFindJump(p, strict)
	e.findUpdateMatches()
}
