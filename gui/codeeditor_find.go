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
}

// EditorFindSnapshot describes a literal search without exposing mutable ranges.
// Count includes all matches; Retained is the bounded navigation/highlight set.
type EditorFindSnapshot struct {
	Query                    string
	Options                  EditorFindOptions
	Count, Retained, Current int
	Pending                  bool
	Err                      error
}

// FindSnapshot and the other find APIs must be called on the UI thread.
func (e *CodeEditor) FindSnapshot() EditorFindSnapshot {
	current := 0
	if len(e.findMatches) > 0 {
		current = e.findCurrentIdx + 1
	}
	return EditorFindSnapshot{e.findText, e.findState.options, e.findState.count, len(e.findMatches), current, e.findState.pending, e.findState.err}
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
	e.findState.jump = &cursorPos{}
	e.findUpdateMatches()
	e.syncFindBar()
}

// ShowFind opens a child-widget bar, optionally with its replacement row.
// Single-line selections seed the query; otherwise the previous query survives.
func (e *CodeEditor) ShowFind(replace bool) {
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
	s := &e.findState
	s.generation++
	if s.worker != nil {
		s.worker.cancel()
	}
	r := editorFindRequest{text: e.Text(), query: e.findText, replacement: e.replaceText, options: s.options, revision: e.textRevision, generation: s.generation, replace: replace}
	s.request = r
	s.request.text = ""
	s.valid = false
	s.pending = true
	s.err = nil
	s.count = 0
	e.findMatches = nil
	if len(r.text)+len(r.query)+len(r.replacement) <= editorFindAsyncBytes || r.query == "" {
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
	return s.pending && s.generation == r.generation && e.TextRevision() == r.revision && e.findText == r.query && s.options == r.options && (!r.replace || e.replaceText == r.replacement)
}
func (e *CodeEditor) acceptFindResult(result editorFindResult) {
	if !e.findRequestCurrent(result.request) {
		return
	}
	s := &e.findState
	s.pending = false
	s.err = result.err
	if result.err != nil {
		e.syncFindBar()
		e.Update()
		return
	}
	if result.request.replace {
		if result.count > 0 && result.text != e.Text() {
			e.ReplaceAllText(result.text)
		} else {
			e.findUpdateMatches()
		}
		return
	}
	s.valid = true
	s.count = result.count
	e.findMatches = result.matches
	if e.findCurrentIdx < 0 || e.findCurrentIdx >= len(e.findMatches) {
		e.findCurrentIdx = 0
	}
	if s.jump != nil {
		p := *s.jump
		s.jump = nil
		e.selectMatchAtOrAfter(p.line, p.col)
	}
	if s.step != 0 {
		step := s.step
		s.step = 0
		e.moveFind(step)
	}
	e.syncFindBar()
	e.Update()
}
func (e *CodeEditor) findRangesCurrent() bool {
	s := &e.findState
	return s.valid && !s.pending && s.request.revision == e.TextRevision() && s.request.query == e.findText && s.request.options == s.options
}
func (e *CodeEditor) moveFind(step int) {
	if !e.findRangesCurrent() {
		e.findState.step = step
		if !e.findState.pending {
			e.findUpdateMatches()
		}
		return
	}
	if len(e.findMatches) == 0 {
		return
	}
	e.findCurrentIdx = (e.findCurrentIdx + step + len(e.findMatches)) % len(e.findMatches)
	m := e.findMatches[e.findCurrentIdx]
	e.cursorLine, e.cursorCol = m.line, m.col
	e.clearSelection()
	e.ensureCursorVisible()
	e.syncFindBar()
	e.Update()
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
	s.jump = nil
	s.step = 0
	if s.worker != nil {
		s.worker.close()
		s.worker = nil
	}
	if e.findBar != nil {
		e.findBar.tooltips(false)
	}
}
func (e *CodeEditor) OnHide() { e.CancelFind() }
func (e *CodeEditor) OnShow() {
	if e.findActive {
		e.findUpdateMatches()
		e.syncFindBar()
	}
}
func (e *CodeEditor) closeFindBar() {
	e.CancelFind()
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
	if s.Count > s.Retained {
		return fmt.Sprintf("%d / %d (first %d)", s.Current, s.Count, s.Retained)
	}
	return fmt.Sprintf("%d / %d", s.Current, s.Count)
}

// Search highlights are sorted; never visit every match once per visible line.
// Clip rune columns before shaping prefixes, particularly on minified files.
func (e *CodeEditor) visibleFindRange(line int, left, right float64) (int, int) {
	ms := e.findMatches
	lo := sort.Search(len(ms), func(i int) bool { return ms[i].line >= line })
	hi := lo + sort.Search(len(ms)-lo, func(i int) bool { return ms[lo+i].line > line })
	if lo == hi {
		return lo, hi
	}
	a := e.textColumnAtX(e.lines[line], left)
	b := e.textColumnAtX(e.lines[line], right) + 1
	lo += sort.Search(hi-lo, func(i int) bool { return ms[lo+i].end > a })
	hi = lo + sort.Search(hi-lo, func(i int) bool { return ms[lo+i].col > b })
	return lo, hi
}
