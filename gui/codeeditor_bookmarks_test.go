package gui

import (
	"slices"
	"strings"
	"testing"
)

func TestBookmarksBaselineFollowInsertedLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first\ntarget\nlast")
	e.NavigateToLineCol(1, 0)
	e.ToggleBookmark()
	e.NavigateToLineCol(0, 0)
	e.OnTextInput("new\n")
	e.NextBookmark()
	if e.CursorLine() != 2 {
		t.Fatalf("bookmark stayed at %d instead of moved target 2", e.CursorLine())
	}
}
func TestBookmarksBaselineRevealAndRetireExtraCarets(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func f() {\n target()\n}\n")
	e.NavigateToLineCol(1, 0)
	e.ToggleBookmark()
	e.NavigateToLineCol(0, 0)
	e.FoldAll()
	e.AddCursorAtLine(2, 0)
	e.NextBookmark()
	if e.CursorCount() != 1 || e.IsFolded(0) {
		t.Fatal("bookmark navigation retained extra carets or a hidden target")
	}
}

func assertBookmarks(t *testing.T, e *CodeEditor, want ...int) {
	t.Helper()
	got := e.BookmarkLines()
	if !slices.Equal(got, want) || e.BookmarkCount() != len(want) {
		t.Fatalf("marks %v want %v", got, want)
	}
	for _, row := range got {
		if _, ok := e.LineText(row); !ok {
			t.Fatalf("invalid mark %d", row)
		}
	}
}
func TestBookmarksValidationAndReadonly(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.MarkSaved()
	e.SetReadOnly(true)
	if !e.SetBookmarkLines([]int{2, 0, 2}) {
		t.Fatal("import")
	}
	assertBookmarks(t, e, 0, 2)
	if e.SetBookmarkLines([]int{1, 9}) || e.SetBookmark(-1, true) || e.SetBookmark(9, true) {
		t.Fatal("invalid accepted")
	}
	assertBookmarks(t, e, 0, 2)
	got := e.BookmarkLines()
	got[0] = 9
	assertBookmarks(t, e, 0, 2)
	e.NavigateToLineCol(0, 0)
	e.PrevBookmark()
	if e.CursorLine() != 2 {
		t.Fatal("prev wrap")
	}
	e.NextBookmark()
	if e.CursorLine() != 0 {
		t.Fatal("next wrap")
	}
	e.ClearBookmarks()
	assertBookmarks(t, e)
	if e.Modified() || e.CanUndo() || e.CanRedo() {
		t.Fatal("annotation dirtied text")
	}
}
func TestBookmarksByteMappingAndHistory(t *testing.T) {
	cases := []struct {
		name, text       string
		marks            []int
		line, col, count int
		insert           string
		want             []int
	}{
		{"insert before", "a\nb\nc", []int{1, 2}, 0, 0, 0, "new\n", []int{2, 3}},
		{"split at start", "a\nb\nc", []int{1}, 1, 0, 0, "new\n", []int{2}},
		{"split inside", "a\nbbb\nc", []int{1}, 1, 1, 0, "\n", []int{1}},
		{"delete row", "a\nb\nc", []int{0, 1, 2}, 1, 0, 2, "", []int{0, 1}},
		{"join collision", "a\nb\nc", []int{0, 1, 2}, 0, 1, 1, "", []int{0, 1}},
		{"delete tail", "a\nb\nc", []int{2}, 1, 1, 2, "", nil},
		{"replace selection", "a\nb\nc\nd", []int{0, 1, 2, 3}, 0, 1, 4, "\nX\n", []int{0, 3}},
		{"unicode CRLF", "中🙂\r\nb\r\nc", []int{0, 1, 2}, 0, 0, 0, "新\r\n", []int{1, 2, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetText(c.text)
			e.SetBookmarkLines(c.marks)
			old := e.Text()
			start := byteOffsetForCursor(e.lines, c.line, runeColumnAtByte(e.lines[c.line], c.col))
			removed := old[start : start+c.count]
			e.pushUndo(editAction{kind: 2, line: c.line, col: runeColumnAtByte(e.lines[c.line], c.col), oldText: removed, text: c.insert})
			e.replaceByteRange(c.line, c.col, c.count, c.insert)
			e.rebuildText()
			after := e.Text()
			assertBookmarks(t, e, c.want...)
			e.Undo()
			assertBookmarks(t, e, c.marks...)
			if e.Text() != old {
				t.Fatal("undo text")
			}
			e.Redo()
			assertBookmarks(t, e, c.want...)
			if e.Text() != after {
				t.Fatal("redo text")
			}
		})
	}
}
func TestBookmarksLineCommandsAndIdenticalText(t *testing.T) {
	for _, text := range []string{"a\nb\nc", "a\r\nb\r\nc", "a\na\na"} {
		t.Run(text, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetText(text)
			e.SetBookmarkLines([]int{1})
			e.NavigateToLineCol(1, 0)
			e.MoveLinesUp()
			assertBookmarks(t, e, 0)
			e.Undo()
			assertBookmarks(t, e, 1)
			e.Redo()
			assertBookmarks(t, e, 0)
			e.Undo()
			e.NavigateToLineCol(1, 0)
			e.DuplicateLines()
			assertBookmarks(t, e, 1)
			e.Undo()
			assertBookmarks(t, e, 1)
			e.NavigateToLineCol(2, 0)
			e.SetBookmark(2, true)
			e.DuplicateLines()
			assertBookmarks(t, e, 1, 2)
			e.Undo()
			assertBookmarks(t, e, 1, 2)
			e.NavigateToLineCol(2, 0)
			e.DeleteLines()
			assertBookmarks(t, e, 1)
			e.Undo()
			assertBookmarks(t, e, 1, 2)
			e.Redo()
			assertBookmarks(t, e, 1)
		})
	}
}
func TestBookmarksExplicitChangesSurviveTextUndo(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.SetBookmark(1, true)
	e.NavigateToLineCol(0, 0)
	e.OnTextInput("new\n")
	assertBookmarks(t, e, 2)
	e.SetBookmark(2, false)
	e.SetBookmark(3, true)
	e.Undo()
	assertBookmarks(t, e, 2)
	e.Redo()
	assertBookmarks(t, e, 3)
	e.ClearBookmarks()
	e.Undo()
	e.Redo()
	assertBookmarks(t, e)
}
func TestBookmarksReloadAndWholeBufferReplace(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.SetBookmark(2, true)
	e.ReplaceAllText("prefix\na\nb\nc")
	assertBookmarks(t, e, 3)
	e.Undo()
	assertBookmarks(t, e, 2)
	e.Redo()
	assertBookmarks(t, e, 3)
	e.SetText("c")
	assertBookmarks(t, e, 0)
	if e.CanUndo() {
		t.Fatal("reload retained history")
	}
	e.SetText("")
	assertBookmarks(t, e, 0)
}
func TestBookmarksObserversSnapshotAndContext(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb")
	calls := []string{}
	var stopB func()
	stopA := e.SubscribeBookmarksChanged(func() {
		calls = append(calls, "A")
		stopB()
		if e.Text() == "" {
			t.Fatal("not published")
		}
	})
	stopB = e.SubscribeBookmarksChanged(func() { calls = append(calls, "B") })
	e.SetBookmark(1, true)
	if !slices.Equal(calls, []string{"A"}) {
		t.Fatal(calls)
	}
	stopA()
	stopA()
	calls = nil
	e.SetBookmark(0, true)
	if len(calls) != 0 {
		t.Fatal("detached callback")
	}
	n := 0
	e.SubscribeBookmarksChanged(func() { n++ })
	e.NavigateToLineCol(1, 1)
	e.OnTextInput("x")
	if n != 1 {
		t.Fatal("context event", n)
	}
	e.NextBookmark()
	e.PrevBookmark()
	if n != 1 {
		t.Fatal("navigation notified")
	}
	e.SetBookmarkLines([]int{0, 1})
	if n != 1 {
		t.Fatal("no-op notified")
	}
}
func TestBookmarksMultiCaretAndMergeKeys(t *testing.T) {
	for _, backspace := range []bool{false, true} {
		e := NewCodeEditor()
		e.SetText("a\nb\nc")
		e.SetBookmarkLines([]int{0, 1, 2})
		if backspace {
			e.NavigateToLineCol(1, 0)
			e.onKeyDownWithShift(KeyBackSpace, false, false)
		} else {
			e.NavigateToLineCol(0, 1)
			e.onKeyDownWithShift(KeyDelete, false, false)
		}
		assertBookmarks(t, e, 0, 1)
		e.Undo()
		assertBookmarks(t, e, 0, 1, 2)
		e.Redo()
		assertBookmarks(t, e, 0, 1)
	}
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.SetBookmarkLines([]int{0, 1, 2})
	e.NavigateToLineCol(0, 0)
	e.AddCursorAtLine(2, 0)
	e.OnTextInput("x\n")
	assertBookmarks(t, e, 1, 2, 4)
	e.Undo()
	assertBookmarks(t, e, 0, 1, 2)
	e.Redo()
	assertBookmarks(t, e, 1, 2, 4)
}
func BenchmarkEditorBookmarkInlineMapping(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("line\n", 100000))
	e.SetBookmarkLines([]int{1, 50000, 99999})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.mapByteBookmarks(42, 1, 42, 2, 0)
	}
}
