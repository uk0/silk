package gui

import (
	"strings"
	"testing"
)

func regexFindEditor(t *testing.T, text, query string) (*CodeEditor, chan func()) {
	t.Helper()
	e, posts := asyncFindEditor(t)
	e.SetText(text)
	e.findState.options = EditorFindOptions{Regex: true, MatchCase: true}
	e.SetFindQuery(query)
	if !e.FindSnapshot().Pending {
		t.Fatal("regex compiled on UI thread")
	}
	waitCompletionPost(t, posts)()
	if !e.FindSnapshot().Ready {
		t.Fatal(e.FindSnapshot())
	}
	return e, posts
}
func TestEditorRegexCurrentReplaceUndoRedoExactBytes(t *testing.T) {
	for _, tc := range []struct{ text, query, replacement, want string }{
		{"中ab\ncd\xfftail", `(?P<x>ab\ncd)`, "${x}!", "中ab\ncd!\xfftail"},
		{"\xc2X\x80", `X`, "", "\xc2\x80"},
		{"ab a", `a(b?)`, "${1}中", "b中 a"},
		{"a\nb", `(?m)^`, "X", "Xa\nb"},
	} {
		t.Run(tc.query+tc.text, func(t *testing.T) {
			e, posts := regexFindEditor(t, tc.text, tc.query)
			e.replaceText = tc.replacement
			changes := 0
			e.SigChanged(func(string) { changes++ })
			e.replaceCurrent()
			if !e.FindSnapshot().Pending || e.Text() != tc.text {
				t.Fatal("not asynchronous")
			}
			waitCompletionPost(t, posts)()
			if e.Text() != tc.want || changes != 1 {
				t.Fatal(e.Text(), tc.want, changes)
			}
			waitCompletionPost(t, posts)()
			e.Undo()
			assertEditorBytes(t, e, tc.text)
			e.Redo()
			assertEditorBytes(t, e, tc.want)
		})
	}
}
func TestEditorRegexCurrentStaleDelivery(t *testing.T) {
	for _, action := range []string{"edit", "query", "options", "replacement", "close", "hide", "ancestor"} {
		t.Run(action, func(t *testing.T) {
			e, posts := regexFindEditor(t, "ab ab", `a(b)`)
			e.replaceText = "$1"
			e.replaceCurrent()
			delivery := waitCompletionPost(t, posts)
			want := e.Text()
			switch action {
			case "edit":
				e.ReplaceAllText("safe")
				want = "safe"
			case "query":
				e.SetFindQuery("b")
			case "options":
				e.SetFindOptions(EditorFindOptions{})
			case "replacement":
				e.replaceText = "other"
			case "close":
				e.closeFindBar()
			case "hide":
				e.Hide()
			case "ancestor":
				p := &Widget{}
				p.Init(p)
				e.SetParent(p)
				p.Hide()
			}
			delivery()
			if e.Text() != want {
				t.Fatal("stale single replace", e.Text())
			}
		})
	}
}
func TestEditorRegexZeroWidthNoOpAndMultilineHighlights(t *testing.T) {
	e, posts := regexFindEditor(t, "a\nb", `(?m)^`)
	e.ShowFind(true)
	waitCompletionPost(t, posts)()
	e.replaceText = ""
	e.replaceCurrent()
	waitCompletionPost(t, posts)()
	waitCompletionPost(t, posts)()
	if e.findCurrentIdx != 1 || e.findBar.status.Text() != "2 / 2" || len(e.undoStack) != 0 {
		t.Fatal(e.FindSnapshot(), e.findBar.status.Text())
	}
	e.SetText("prefix\n中\nlast\nend")
	e.SetFindQuery(`fix\n中\nla`)
	waitCompletionPost(t, posts)()
	for line, want := range [][2]int{{3, 6}, {0, 1}, {0, 2}, {0, 0}} {
		a, b := e.visibleFindRange(line, 0, 10000)
		if line == 3 {
			if a != b {
				t.Fatal("highlight leaked to unrelated line")
			}
			continue
		}
		if b-a != 1 {
			t.Fatal("missing multiline highlight", line, a, b)
		}
		c, d := e.findHighlightColumns(a, line)
		if c != want[0] || d != want[1] {
			t.Fatal(line, c, d, want)
		}
	}
}
func TestEditorRegexErrorAndToggleModel(t *testing.T) {
	e, posts := regexFindEditor(t, "ab ab", `a(b)`)
	e.ShowFind(true)
	waitCompletionPost(t, posts)()
	if !e.findBar.regex.IsChecked() {
		t.Fatal("regex check missing")
	}
	e.SetFindQuery("(")
	waitCompletionPost(t, posts)()
	s := e.FindSnapshot()
	if s.Err == nil || s.Ready || s.Pending || s.Count != 0 || s.Retained != 0 || !strings.Contains(e.findBar.status.Text(), "error parsing regexp") || e.findBar.one.IsEnabled() {
		t.Fatal(s)
	}
	e.findBar.tooltips(true)
	if GetToolTip(e.findBar.status) != e.findBar.status.Text() {
		t.Fatal("error tooltip missing")
	}
	e.CancelFind()
	for _, w := range e.findBar.Children() {
		if GetToolTip(w) != "" {
			t.Fatal("tooltip leak")
		}
	}
}
func TestFindSnapshotInvalidationAndAncestorVisibility(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("foo")
	e.SetFindQuery("foo")
	if !e.FindSnapshot().Ready {
		t.Fatal(e.FindSnapshot())
	}
	e.ReplaceAllText("foo foo") // unopened bar must not keep stale counts
	if s := e.FindSnapshot(); s.Ready || s.Count != 0 || s.Retained != 0 {
		t.Fatal(s)
	}
	e.ShowFind(false)
	p, q := &Widget{}, &Widget{}
	p.Init(p)
	q.Init(q)
	q.SetParent(p)
	e.SetParent(q)
	p.Hide()
	e.ReplaceAllText("foo foo foo")
	if s := e.FindSnapshot(); s.Ready || s.Pending || s.Count != 0 {
		t.Fatal(s)
	}
	p.Show()
	e.ShowFind(false)
	if s := e.FindSnapshot(); !s.Ready || s.Count != 3 || s.Revision != e.TextRevision() {
		t.Fatal(s)
	}
	e.CancelFind()
	if e.FindSnapshot().Ready || e.FindSnapshot().Count != 0 {
		t.Fatal(e.FindSnapshot())
	}
}

// A same-line hit already has its end column. Do not count every rune of a
// megabyte minified line for each visible highlight on every frame.
func BenchmarkEditorRegexMinifiedHighlightColumns(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("a ", 1000000))
	e.findState.options.Regex = true
	e.findMatches = []findMatch{{0, 0, 1}}
	e.findSpans = []editorFindSpan{{0, 1, 0, 1}}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a, z := e.findHighlightColumns(0, 0)
		if a != 0 || z != 1 {
			b.Fatal(a, z)
		}
	}
}
