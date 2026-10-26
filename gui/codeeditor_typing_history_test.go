package gui

import (
	"strings"
	"testing"
	"time"
)

func selectTypingRange(e *CodeEditor, sl, sc, el, ec int, reverse bool) {
	e.selStartLine, e.selStartCol, e.selEndLine, e.selEndCol = sl, sc, el, ec
	if reverse {
		e.selStartLine, e.selStartCol, e.selEndLine, e.selEndCol = el, ec, sl, sc
	}
	e.hasSelection = true
	e.cursorLine, e.cursorCol = e.selEndLine, e.selEndCol
}

func typeHistoryRun(e *CodeEditor, text string) {
	for _, r := range text {
		// Exercise grouping deterministically even on a slow race-enabled runner.
		if n := len(e.undoStack); n > 0 {
			e.undoStack[n-1].stamp = time.Now()
		}
		e.OnTextInput(string(r))
	}
}

func TestEditorTypingSelectionSingleUndo(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, input := range []string{"Menu focus 2.75", "新🙂e\u0301 42"} {
			t.Run(input+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				e := NewCodeEditor()
				e.filePath = "typing.txt"
				original := "prefix 中🙂 alpha \t\r\n    beta\t\n\tgamma  \r\nlast\xff suffix"
				e.SetText(original)
				e.MarkSaved()
				e.SetBookmarkLines([]int{1, 3})
				selectTypingRange(e, 0, 7, 3, 5, reverse)
				typeHistoryRun(e, input)
				want := "prefix " + input + " suffix"
				assertEditorBytes(t, e, want)
				if len(e.undoStack) != 1 {
					t.Fatalf("selection typing created %d undo groups; want one", len(e.undoStack))
				}
				e.Undo()
				assertEditorBytes(t, e, original)
				if e.Modified() || e.CanUndo() || !e.CanRedo() {
					t.Fatal("undo did not reach clean baseline")
				}
				if !e.HasBookmark(1) || !e.HasBookmark(3) {
					t.Fatal("undo lost original bookmarks")
				}
				e.Redo()
				assertEditorBytes(t, e, want)
				if !e.Modified() {
					t.Fatal("redo must be dirty")
				}
				e.Undo()
				assertEditorBytes(t, e, original)
			})
		}
	}
}

func TestEditorTypingAtomicInputBoundaries(t *testing.T) {
	for _, selection := range []bool{false, true} {
		for _, kind := range []string{"paste_one", "paste_many", "ime_commit", "newline", "pair"} {
			t.Run(kind+map[bool]string{false: "/insert", true: "/replace"}[selection], func(t *testing.T) {
				e := NewCodeEditor()
				e.filePath = "typing.txt"
				e.SetText("old")
				e.ScrollToLineCol(0, 3)
				if selection {
					selectTypingRange(e, 0, 0, 0, 3, false)
				}
				switch kind {
				case "paste_one":
					e.pasteText("7")
				case "paste_many":
					e.pasteText("paste")
				case "ime_commit":
					e.OnTextInput("中文")
				case "newline":
					e.OnTextInput("\n")
				case "pair":
					e.OnTextInput("(")
				}
				atomic := e.Text()
				e.OnTextInput("8")
				e.Undo()
				assertEditorBytes(t, e, atomic)
				e.Undo()
				assertEditorBytes(t, e, "old")
			})
		}
	}
}

func TestEditorTypingHistoryBoundaries(t *testing.T) {
	for _, selection := range []bool{false, true} {
		for _, boundary := range []string{"save", "undo_redo", "caret_roundtrip", "arrow_roundtrip", "timeout", "bookmark", "focus"} {
			t.Run(boundary+map[bool]string{false: "/insert", true: "/replace"}[selection], func(t *testing.T) {
				e := NewCodeEditor()
				e.filePath = "typing.txt"
				e.SetText("old")
				e.ScrollToLineCol(0, 3)
				if selection {
					selectTypingRange(e, 0, 0, 0, 3, false)
				}
				e.OnTextInput("7")
				before := e.Text()
				switch boundary {
				case "save":
					e.MarkSaved()
				case "undo_redo":
					e.Undo()
					e.Redo()
				case "caret_roundtrip":
					col := e.cursorCol
					e.ScrollToLineCol(0, 0)
					e.ScrollToLineCol(0, col)
				case "arrow_roundtrip":
					e.onKeyDownWithShift(KeyLeft, false, false)
					e.onKeyDownWithShift(KeyRight, false, false)
				case "timeout":
					e.undoStack[len(e.undoStack)-1].stamp = time.Now().Add(-time.Second)
				case "bookmark":
					e.SetBookmark(0, true)
				case "focus":
					e.OnFocusChanged(nil, e.Self())
					e.OnFocusChanged(e.Self(), nil)
				}
				e.OnTextInput("8")
				e.Undo()
				assertEditorBytes(t, e, before)
				if boundary == "save" && e.Modified() {
					t.Fatal("undo skipped saved state")
				}
				e.Undo()
				assertEditorBytes(t, e, "old")
			})
		}
	}
}

func TestEditorTypingDoesNotJoinPreviousOrBranchedInput(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("old")
	e.ScrollToLineCol(0, 3)
	e.OnTextInput("7")
	selectTypingRange(e, 0, 0, 0, 4, false)
	typeHistoryRun(e, "89")
	e.Undo()
	assertEditorBytes(t, e, "old7")
	e.OnTextInput("X")
	e.Undo()
	assertEditorBytes(t, e, "old7")
	e.Undo()
	assertEditorBytes(t, e, "old")
}

func TestEditorTypingEmptyInputPreservesHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("old")
	selectTypingRange(e, 0, 0, 0, 3, false)
	rev := e.TextRevision()
	e.OnTextInput("")
	assertEditorBytes(t, e, "old")
	if e.CanUndo() || e.TextRevision() != rev || !e.hasSelection {
		t.Fatal("empty commit mutated selection/history")
	}
}

func BenchmarkEditorTypingSelectionHistory(b *testing.B) {
	e := NewCodeEditor()
	e.filePath = "typing.txt"
	e.SetText(strings.Repeat("line\r\n", 1000))
	defer e.CancelSymbolIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selectTypingRange(e, 0, 0, 0, 4, false)
		for _, r := range "123456789" {
			e.OnTextInput(string(r))
		}
		e.Undo()
	}
}
