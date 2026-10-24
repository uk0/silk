package gui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func selectFindScope(e *CodeEditor, sl, sc, el, ec int) {
	e.hasSelection = true
	e.selStartLine, e.selStartCol, e.selEndLine, e.selEndCol = sl, sc, el, ec
}
func settleScope(t *testing.T, e *CodeEditor, posts chan func()) {
	t.Helper()
	for i := 0; e.FindSnapshot().Pending; i++ {
		if i == 6 {
			t.Fatal("scope never settled", e.FindSnapshot())
		}
		waitCompletionPost(t, posts)()
	}
}
func scopedEditor(t *testing.T, text, query string, regex bool, sl, sc, el, ec int) (*CodeEditor, chan func()) {
	t.Helper()
	e, posts := asyncFindEditor(t)
	e.SetText(text)
	selectFindScope(e, sl, sc, el, ec)
	e.ShowFind(true)
	e.findState.options = EditorFindOptions{Regex: regex, MatchCase: true}
	e.SetFindQuery(query)
	if err := e.SetFindInSelection(true); err != nil {
		t.Fatal(err)
	}
	settleScope(t, e, posts)
	if !e.FindSnapshot().Ready {
		t.Fatal(e.FindSnapshot())
	}
	return e, posts
}
func TestEditorFindScopeProjectionAndLocalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		query       string
		regex, word bool
		count       int
	}{
		{"foo", false, false, 3}, {`foo`, true, false, 3}, {`\Afoo`, true, false, 1}, {`foo\z`, true, false, 1}, {"foo", false, true, 3}, {`foo`, true, true, 3},
	} {
		t.Run(tc.query+string(rune('0'+tc.count))+string(rune('a'+boolIntScope(tc.regex))), func(t *testing.T) {
			e, posts := scopedEditor(t, "外foo\n中foo foo\nfoo尾\nfoo", tc.query, tc.regex, 2, 3, 1, 1) // reverse selection
			e.SetFindOptions(EditorFindOptions{Regex: tc.regex, WholeWord: tc.word, MatchCase: true})
			settleScope(t, e, posts)
			s := e.FindSnapshot()
			if s.Count != tc.count || !s.SelectionValid || s.SelectionStart != len("外foo\n中") || s.SelectionEnd != len("外foo\n中foo foo\nfoo") {
				t.Fatal(s)
			}
			for i, m := range e.findMatches {
				span := e.findSpans[i]
				if span.start < s.SelectionStart || span.end > s.SelectionEnd || m.line < 1 || m.line > 2 {
					t.Fatal(m, span)
				}
				if e.Text()[span.start:span.end] != "foo" {
					t.Fatal(span)
				}
			}
		})
	}
}
func boolIntScope(b bool) int {
	if b {
		return 1
	}
	return 0
}
func TestEditorFindScopeReplaceHistory(t *testing.T) {
	for _, regex := range []bool{false, true} {
		for _, one := range []bool{false, true} {
			t.Run(map[bool]string{true: "regex", false: "literal"}[regex]+map[bool]string{true: "One", false: "All"}[one], func(t *testing.T) {
				before := "foo\n中foo foo\nfoo尾\nfoo"
				e, posts := scopedEditor(t, before, "foo", regex, 1, 1, 2, 3)
				old := e.findState.scope
				e.replaceText = "龙\nQ"
				changes := 0
				e.SigChanged(func(string) { changes++ })
				if one {
					e.replaceCurrent()
				} else {
					e.replaceAll()
				}
				settleScope(t, e, posts)
				middle := "龙\nQ 龙\nQ\n龙\nQ"
				if one {
					middle = "龙\nQ foo\nfoo"
				}
				want := "foo\n中" + middle + "尾\nfoo"
				assertEditorBytes(t, e, want)
				after := e.findState.scope
				if changes != 1 || len(e.undoStack) != 1 || !after.valid || after.end != after.start+len(middle) {
					t.Fatal(changes, len(e.undoStack), after)
				}
				actual := editorRegexPosition{}
				_ = actual.advance(context.Background(), want[:after.end], after.end)
				if after.last != (cursorPos{actual.line, actual.col}) {
					t.Fatal(after, actual)
				}
				e.Undo()
				settleScope(t, e, posts)
				assertEditorBytes(t, e, before)
				if s := e.findState.scope; !s.valid || s.start != old.start || s.end != old.end || s.last != old.last {
					t.Fatal(s, old)
				}
				e.Redo()
				settleScope(t, e, posts)
				assertEditorBytes(t, e, want)
				if s := e.findState.scope; !s.valid || s.end != after.end || s.last != after.last {
					t.Fatal(s, after)
				}
			})
		}
	}
}
func TestEditorFindScopeCandidateAndFailClosed(t *testing.T) {
	e, posts := scopedEditor(t, "foo foo foo", "foo", false, 0, 4, 0, 7)
	e.findNext()
	e.findPrev()
	e.SetFindQuery("f")
	if s := e.FindSnapshot(); s.Count != 1 || s.SelectionStart != 4 || s.SelectionEnd != 7 {
		t.Fatal(s)
	}
	e.clearSelection()
	e.findState.documentFocus = true
	e.cursorCol = 0
	e.insertTextAtCursor("!")
	settleScope(t, e, posts)
	if s := e.FindSnapshot(); s.SelectionValid || !s.InSelection || s.Ready || !errors.Is(s.Err, ErrEditorFindSelection) || e.findBar.all.IsEnabled() {
		t.Fatal(s)
	}
	before := e.Text()
	e.replaceText = "oops"
	e.replaceAll()
	settleScope(t, e, posts)
	assertEditorBytes(t, e, before)
	e.Undo()
	settleScope(t, e, posts)
	if e.FindSnapshot().SelectionValid {
		t.Fatal("unrelated undo resurrected scope")
	}
	e.closeFindBar()
	e.ShowFind(false)
	e.SetFindQuery("foo")
	if s := e.FindSnapshot(); s.InSelection || s.Count != 3 {
		t.Fatal(s)
	}
	e.clearSelection()
	if err := e.SetFindInSelection(true); !errors.Is(err, ErrEditorFindSelection) {
		t.Fatal(err)
	}
	if e.FindSnapshot().Ready || e.findBar.one.IsEnabled() {
		t.Fatal("missing selection widened search")
	}
	// Reopen with a fresh selection; ShowFind may clear the live selection when navigating.
	selectFindScope(e, 0, 8, 0, 11)
	e.ShowFind(false)
	e.clearSelection()
	if err := e.SetFindInSelection(true); err != nil {
		t.Fatal(err)
	}
	if s := e.FindSnapshot(); !s.SelectionValid || s.SelectionStart != 8 || s.Count != 1 {
		t.Fatal(s)
	}
}
func TestEditorFindScopeEmptyAndNoOp(t *testing.T) {
	e, posts := scopedEditor(t, "outside|X|outside", `X`, true, 0, 8, 0, 9)
	e.replaceText = "X"
	e.replaceCurrent()
	settleScope(t, e, posts)
	if len(e.undoStack) != 0 || !e.FindSnapshot().SelectionValid {
		t.Fatal("no-op changed history/scope")
	}
	e.replaceText = ""
	e.replaceAll()
	settleScope(t, e, posts)
	assertEditorBytes(t, e, "outside||outside")
	if s := e.FindSnapshot(); !s.SelectionValid || s.SelectionStart != s.SelectionEnd {
		t.Fatal(s)
	}
	e.SetFindQuery(`\A\z`)
	settleScope(t, e, posts)
	if e.FindSnapshot().Count != 1 {
		t.Fatal(e.FindSnapshot())
	}
	e.replaceText = "中"
	e.replaceCurrent()
	settleScope(t, e, posts)
	assertEditorBytes(t, e, "outside|中|outside")
	if s := e.findState.scope; !s.valid || s.last.col != 9 || s.end != 11 {
		t.Fatal(s)
	}
}
func TestEditorFindScopeStaleDeliveryAndHistoryIdentity(t *testing.T) {
	for _, action := range []string{"disable", "recapture", "edit", "replacement", "close", "hide"} {
		t.Run(action, func(t *testing.T) {
			e, posts := scopedEditor(t, "foo foo foo", `foo`, true, 0, 4, 0, 7)
			e.replaceText = "bad"
			e.replaceAll()
			delivery := waitCompletionPost(t, posts)
			want := e.Text()
			switch action {
			case "disable":
				_ = e.SetFindInSelection(false)
			case "recapture":
				_ = e.SetFindInSelection(false)
				selectFindScope(e, 0, 8, 0, 11)
				_ = e.SetFindInSelection(true)
			case "edit":
				e.ReplaceAllText("safe")
				want = "safe"
			case "replacement":
				e.replaceText = "changed"
			case "close":
				e.closeFindBar()
			case "hide":
				e.Hide()
			}
			delivery()
			assertEditorBytes(t, e, want)
		})
	}
	e, posts := scopedEditor(t, "foo foo foo", "foo", false, 0, 4, 0, 7)
	e.replaceText = "bar"
	e.replaceAll()
	settleScope(t, e, posts)
	_ = e.SetFindInSelection(false)
	selectFindScope(e, 0, 8, 0, 11)
	_ = e.SetFindInSelection(true)
	id := e.findState.scope.id
	e.Undo()
	settleScope(t, e, posts)
	if e.findState.scope.id != id || e.findState.scope.valid {
		t.Fatal("old history retargeted new scope")
	}
	e.closeFindBar()
	e.Redo()
	if e.findState.scope.enabled {
		t.Fatal("history reopened scope")
	}
}
func TestEditorFindScopeMalformedSeams(t *testing.T) {
	for _, one := range []bool{true, false} {
		e, posts := scopedEditor(t, "\xc2X\x80", "X", false, 0, 1, 0, 2)
		e.replaceText = ""
		if one {
			e.replaceCurrent()
		} else {
			e.replaceAll()
		}
		settleScope(t, e, posts)
		assertEditorBytes(t, e, "\xc2\x80")
		if e.findState.scope.valid || e.FindSnapshot().Ready {
			t.Fatal("scope split a newly joined rune")
		}
		e.Undo()
		settleScope(t, e, posts)
		assertEditorBytes(t, e, "\xc2X\x80")
		if !e.findState.scope.valid {
			t.Fatal("Undo did not restore boundary")
		}
		e.Redo()
		settleScope(t, e, posts)
		if e.findState.scope.valid {
			t.Fatal("Redo kept stale coordinates")
		}
	}
	// Include all internal malformed seams: single replacement fails closed;
	// all replacement computes fresh coordinates from the complete new fragment.
	for _, one := range []bool{true, false} {
		e, posts := scopedEditor(t, "|a\xc2X\x80z|", "X", false, 0, 1, 0, 6)
		e.replaceText = ""
		if one {
			e.replaceCurrent()
		} else {
			e.replaceAll()
		}
		settleScope(t, e, posts)
		assertEditorBytes(t, e, "|a\xc2\x80z|")
		if e.findState.scope.valid == one {
			t.Fatal(one, e.findState.scope)
		}
		if !one && e.findState.scope.last.col != 4 {
			t.Fatal(e.findState.scope)
		}
	}
	for _, text := range []string{"中😀x", "\xc2\x80", "\xffx", "a\xe2\x82\xac"} {
		for i := 0; i <= len(text); i++ {
			want := utf8.RuneCountInString(text[:i])+utf8.RuneCountInString(text[i:]) == utf8.RuneCountInString(text)
			for split := 0; split <= len(text); split++ {
				if got := editorFindPieceBoundary(i, text[:split], text[split:]); got != want {
					t.Fatal([]byte(text), i, split, got, want)
				}
			}
		}
	}
}
func TestEditorFindScopeBudgetCancellationAndFullCount(t *testing.T) {
	text := "foo|" + strings.Repeat("foo ", editorFindHitLimit+3) + "|foo"
	scope := editorFindScope{enabled: true, valid: true, start: 4, end: len(text) - 4, first: cursorPos{0, 4}, last: cursorPos{0, len(text) - 4}, revision: 1}
	r := editorFindRequest{text: text, query: "foo", scope: scope, revision: 1}
	got := computeEditorFind(context.Background(), r)
	if got.err != nil || got.count != editorFindHitLimit+3 || len(got.matches) != editorFindHitLimit || len(got.spans) != editorFindHitLimit {
		t.Fatal(got.count, len(got.matches), got.err)
	}
	r.replace = true
	r.replacement = "x"
	got = computeEditorFind(context.Background(), r)
	if got.err != nil || got.text != "foo|"+strings.Repeat("x ", editorFindHitLimit+3)+"|foo" {
		t.Fatal(got.err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = computeEditorFind(ctx, r)
	if !errors.Is(got.err, context.Canceled) || got.count != 0 || got.text != "" {
		t.Fatal(got.err)
	}
	// Full-document budget includes the unchanged outside; exercise a small
	// injected budget without allocating a 128 MiB fixture.
	r = editorFindRequest{text: "123foo456", query: "foo", replacement: "long", replace: true, budgetSet: true, budget: 9, revision: 1, scope: editorFindScope{enabled: true, valid: true, start: 3, end: 6, first: cursorPos{0, 3}, last: cursorPos{0, 6}, revision: 1}}
	got = computeEditorFind(context.Background(), r)
	if !errors.Is(got.err, ErrEditorFindReplacementTooLarge) || got.text != "" || got.count != 0 {
		t.Fatal(got.err, got.text)
	}
}
