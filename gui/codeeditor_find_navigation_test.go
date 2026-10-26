package gui

import (
	"strings"
	"testing"
)

func TestEditorFindNavigationSelectsMatchAndPreservesDocument(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	text := "中🙂 alpha\r\n\talpha tail\nlast  "
	e.SetText(text)
	e.ShowFind(false)
	e.SetFindQuery("alpha")
	rev := e.TextRevision()
	for _, want := range []cursorPos{{0, 8}, {1, 6}, {0, 8}} {
		if e.SelectedText() != "alpha" || (cursorPos{e.cursorLine, e.cursorCol}) != want || e.Text() != text || e.TextRevision() != rev || len(e.undoStack) != 0 {
			t.Fatalf("match not selected: %q cursor=%v want=%v", e.SelectedText(), cursorPos{e.cursorLine, e.cursorCol}, want)
		}
		e.findNext()
	}
	e.findPrev()
	e.closeFindBar()
	if e.SelectedText() != "alpha" {
		t.Fatal("Escape lost selected match")
	}
	e.OnTextInput("FIND")
	if e.Text() != strings.Replace(text, "alpha", "FIND", 1) {
		t.Fatalf("typing did not replace selected match: %q", e.Text())
	}
	e.Undo()
	assertEditorBytes(t, e, text)
}

func TestEditorFindNavigationRegexRanges(t *testing.T) {
	for _, tc := range []struct {
		text, query, selected string
		end                   cursorPos
	}{
		{"中ab\r\ncd tail", `ab\r?\ncd`, "ab\r\ncd", cursorPos{1, 2}},
		{"a\nb", `(?m)^`, "", cursorPos{0, 0}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			e, _ := regexFindEditor(t, tc.text, tc.query)
			if e.SelectedText() != tc.selected || (cursorPos{e.cursorLine, e.cursorCol}) != tc.end || e.hasSelection != (tc.selected != "") {
				t.Fatalf("selection %q caret %d:%d", e.SelectedText(), e.cursorLine, e.cursorCol)
			}
		})
	}
}

func TestEditorFindNavigationLiteralMalformedAndReadOnly(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("x\xffy\xff")
	e.findActive = true
	e.SetReadOnly(true)
	e.SetFindQuery("\xff")
	if e.SelectedText() != "\xff" {
		t.Fatalf("malformed byte not selected: %q", e.SelectedText())
	}
	e.findNext()
	if e.SelectedText() != "\xff" || e.cursorCol != 4 {
		t.Fatal("next malformed match not selected")
	}
	e.closeFindBar()
	e.OnTextInput("bad")
	assertEditorBytes(t, e, "x\xffy\xff")
}

func TestEditorFindNavigationPendingDoesNotStealInteraction(t *testing.T) {
	for _, action := range []string{"caret", "selection", "focus", "multicaret", "parent", "edit"} {
		t.Run(action, func(t *testing.T) {
			e, posts := asyncFindEditor(t)
			oldFocus := focusWidget
			t.Cleanup(func() { focusWidget = oldFocus })
			e.SetFindQuery("foo")
			delivery := waitCompletionPost(t, posts)
			waitFindWorker(t, e.findState.worker)
			switch action {
			case "caret":
				e.cursorLine, e.cursorCol = 20, 4
			case "selection":
				selectFindScope(e, 2, 1, 3, 2)
			case "focus":
				focusWidget = NewEdit()
			case "multicaret":
				e.additionalCursors = []cursorPos{{7, 1}}
			case "parent":
				p := NewLabel("")
				e.SetParent(p)
			case "edit":
				e.cursorLine, e.cursorCol = 10, 4
				e.insertTextAtCursor("!")
			}
			cursor, selection := cursorPos{e.cursorLine, e.cursorCol}, e.selectionSnapshot()
			delivery()
			if action == "edit" {
				waitCompletionPost(t, posts)()
			}
			if cursor != (cursorPos{e.cursorLine, e.cursorCol}) || selection != e.selectionSnapshot() {
				t.Fatalf("late find moved newer %s", action)
			}
			if action != "edit" && (!e.FindSnapshot().Ready || e.FindSnapshot().Count != 5000) {
				t.Fatal("guard discarded useful count", e.FindSnapshot())
			}
		})
	}
}

func TestEditorFindNavigationPendingStepsAccumulate(t *testing.T) {
	e, posts := asyncFindEditor(t)
	e.SetFindQuery("foo")
	e.findNext()
	e.findNext()
	e.findPrev()
	waitCompletionPost(t, posts)()
	if e.findCurrentIdx != 1 || e.SelectedText() != "foo" || e.cursorLine != 1 {
		t.Fatal("queued navigation not accumulated", e.findCurrentIdx, e.cursorLine, e.SelectedText())
	}
}

func TestEditorFindNavigationKeepsOriginalSelectionScope(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("foo foo\nfoo outside")
	selectFindScope(e, 0, 0, 0, 7)
	e.ShowFind(false)
	e.SetFindQuery("foo")
	if e.SelectedText() != "foo" {
		t.Fatal("first hit not selected")
	}
	if err := e.SetFindInSelection(true); err != nil {
		t.Fatal(err)
	}
	if s := e.FindSnapshot(); s.Count != 2 || s.SelectionStart != 0 || s.SelectionEnd != 7 {
		t.Fatal("auto-selected match replaced original scope", s)
	}
}

func TestEditorFindNavigationNoMatchAndSingleCaret(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("foo bar foo")
	e.findActive = true
	e.additionalCursors = []cursorPos{{0, 5}, {0, 9}}
	e.SetFindQuery("foo")
	if len(e.additionalCursors) != 0 || e.SelectedText() != "foo" {
		t.Fatal("find did not select a single match")
	}
	e.SetFindQuery("missing")
	if e.hasSelection || e.FindSnapshot().Count != 0 {
		t.Fatal("no-match query left a destructive stale selection")
	}
	selectFindScope(e, 0, 4, 0, 7)
	e.SetFindQuery("still missing")
	if e.SelectedText() != "bar" {
		t.Fatal("no-match query cleared a manual selection")
	}
}

func TestEditorFindNavigationNewQueryRetiresQueuedSteps(t *testing.T) {
	e, posts := asyncFindEditor(t)
	e.SetFindQuery("foo")
	e.findNext()
	e.findNext()
	old := waitCompletionPost(t, posts)
	waitFindWorker(t, e.findState.worker)
	e.SetFindQuery("safe")
	old()
	waitCompletionPost(t, posts)()
	if e.findCurrentIdx != 0 || e.SelectedText() != "safe" || e.cursorLine != 0 {
		t.Fatal("new query inherited old navigation")
	}
}

func TestEditorFindNavigationPendingCaretChangeThenExplicitStep(t *testing.T) {
	e, posts := asyncFindEditor(t)
	e.SetFindQuery("foo")
	e.cursorLine, e.cursorCol = 30, 4
	// A fresh explicit command may navigate; stale query auto-jump must not also apply.
	e.findNext()
	waitCompletionPost(t, posts)()
	if !e.hasSelection || e.SelectedText() != "foo" || e.cursorLine != 31 {
		t.Fatal("fresh navigation was dropped")
	}
}

func BenchmarkFindMatchNavigation(b *testing.B) {
	e := NewCodeEditor()
	b.Cleanup(e.CancelFind)
	e.SetSize(800, 500)
	e.SetText(strings.Repeat("foo bar\n", 1000))
	e.findActive = true
	e.SetFindQuery("foo")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.findNext()
	}
}

func TestEditorFindNavigationResumesFromManualCaret(t *testing.T) {
	for _, tc := range []struct{ col, step, want int }{{5, 1, 10}, {5, -1, 6}, {3, 1, 6}, {3, -1, 2}, {8, 1, 10}, {0, -1, 10}, {3, 3, 2}, {3, -3, 6}} {
		e := NewCodeEditor()
		e.SetText("xx--xx--xx")
		e.findActive = true
		e.SetFindQuery("xx")
		e.clearSelection()
		e.cursorCol = tc.col
		e.moveFind(tc.step)
		if e.cursorCol != tc.want || e.SelectedText() != "xx" {
			t.Fatalf("caret %d step %d got %d want %d", tc.col, tc.step, e.cursorCol, tc.want)
		}
		e.CancelFind()
	}
}

func TestEditorFindNavigationZeroWidthAdvances(t *testing.T) {
	e, _ := regexFindEditor(t, "a\nb", `(?m)^`)
	for _, line := range []int{1, 0, 1} {
		e.findNext()
		if e.hasSelection || e.cursorLine != line || e.cursorCol != 0 {
			t.Fatal("zero-width navigation stuck", e.cursorLine, e.cursorCol)
		}
	}
}

func TestEditorFindNavigationOptionsDoNotReuseWrongIndex(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetText("FOO foo foo")
	e.findActive = true
	e.SetFindQuery("foo")
	e.findNext()
	if e.cursorCol != 7 {
		t.Fatal(e.cursorCol)
	}
	e.SetFindOptions(EditorFindOptions{MatchCase: true})
	e.findNext()
	if e.cursorCol != 11 || e.SelectedText() != "foo" {
		t.Fatal("option change reused an unrelated result index", e.cursorCol)
	}
}
