package gui

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorIndentSelectionPreservesUndoHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("one\ntwo\nlast")
	e.ReplaceAllText("one\ntwo\nlast!")
	e.hasSelection = true
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 1, 3
	e.cursorLine, e.cursorCol = 1, 3
	e.IndentSelection()
	if e.Text() != "\tone\n\ttwo\nlast!" {
		t.Fatal(e.Text())
	}
	e.Undo()
	if e.Text() != "one\ntwo\nlast!" {
		t.Fatalf("indent lost its Undo: %q", e.Text())
	}
	if !e.HasSelection() || e.selStartCol != 0 || e.selEndCol != 3 {
		t.Fatal("Undo lost selection")
	}
	e.Redo()
	if e.Text() != "\tone\n\ttwo\nlast!" || !e.HasSelection() {
		t.Fatal("Redo lost selection/text")
	}
	e.Undo()
	e.Undo()
	if e.Text() != "one\ntwo\nlast" {
		t.Fatal("earlier history lost")
	}
}

func TestEditorIndentShiftTabWithoutSelectionDedents(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\tfoo")
	e.cursorCol = 1
	e.onKeyDownWithShift(KeyTab, false, true)
	if e.Text() != "foo" || e.CursorCol() != 0 {
		t.Fatalf("Shift+Tab inserted instead: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "\tfoo" || e.CursorCol() != 1 {
		t.Fatal("dedent undo")
	}
}

func TestEditorIndentExcludesSelectionEndAtColumnZero(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.hasSelection = true
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 0
	e.cursorLine, e.cursorCol = 2, 0
	e.IndentSelection()
	if e.Text() != "\ta\n\tb\nc" || e.selEndCol != 0 {
		t.Fatalf("indented excluded line: %q", e.Text())
	}
}

func TestEditorIndentOptionsAndSoftTabStops(t *testing.T) {
	for _, width := range []int{1, 2, 4, 8, 16} {
		for _, text := range []string{"", "a", "ab", "中🙂", "a\xff", "\ta"} {
			e := NewCodeEditor()
			e.SetText(text)
			e.MarkSaved()
			e.SetTabSize(width)
			e.SetInsertSpaces(true)
			if !e.InsertSpaces() || e.Modified() || len(e.undoStack) != 0 {
				t.Fatal("settings edited document")
			}
			e.cursorCol = utf8.RuneCountInString(text)
			col := 0
			for _, r := range text {
				if r == '\t' {
					col += width - col%width
				} else {
					col++
				}
			}
			want := text + strings.Repeat(" ", width-col%width)
			e.onKeyDownWithShift(KeyTab, false, false)
			if e.Text() != want {
				t.Fatalf("width=%d got=%q want=%q", width, e.Text(), want)
			}
			e.Undo()
			if e.Text() != text || e.Modified() {
				t.Fatal("Tab undo")
			}
			e.Redo()
			if e.Text() != want {
				t.Fatal("Tab redo")
			}
		}
	}
	e := NewCodeEditor()
	if e.InsertSpaces() {
		t.Fatal("legacy default changed")
	}
	e.SetText("x")
	e.cursorCol = 1
	e.onKeyDownWithShift(KeyTab, false, false)
	if e.Text() != "x\t" {
		t.Fatal(e.Text())
	}
}

func TestEditorIndentWidthSingleLineSelectionAndNoop(t *testing.T) {
	e := NewCodeEditor()
	e.SetTabSize(2)
	e.SetInsertSpaces(true)
	e.SetText("    中\xff🙂")
	e.cursorCol = 7
	e.hasSelection = true
	e.selStartLine, e.selEndLine = 0, 0
	e.selStartCol, e.selEndCol = 4, 7
	changes := 0
	e.SigChanged(func(string) { changes++ })
	before := e.selectionSnapshot()
	e.OutdentLines()
	if e.Text() != "  中\xff🙂" || changes != 1 || e.selStartCol != 2 || e.selEndCol != 5 || e.CursorCol() != 5 {
		t.Fatal(e.Text(), changes, e.selectionSnapshot())
	}
	e.Undo()
	if e.Text() != "    中\xff🙂" || e.selectionSnapshot() != before {
		t.Fatal("selection/dedent undo")
	}
	e.SetText("abc")
	e.MarkSaved()
	changes = 0
	e.OutdentLines()
	if changes != 0 || e.Modified() || len(e.undoStack) != 0 {
		t.Fatal("no-op changed state")
	}
	e.SetTabSize(2)
	e.SetText("abc")
	e.hasSelection = true
	e.selStartCol, e.selEndCol = 1, 3
	e.cursorCol = 3
	e.onKeyDownWithShift(KeyTab, false, false)
	if e.Text() != "a " {
		t.Fatalf("single-line replacement = %q", e.Text())
	}
	e.Undo()
	if e.Text() != "abc" {
		t.Fatal("replacement undo")
	}
}

func TestEditorIndentReverseSelectionAndOneChange(t *testing.T) {
	for _, remove := range []bool{false, true} {
		e := NewCodeEditor()
		e.SetText("  中\xff🙂\r\n  b\nuntouched")
		e.SetTabSize(2)
		e.SetInsertSpaces(true)
		e.hasSelection = true
		e.selStartLine, e.selStartCol = 2, 0
		e.selEndLine, e.selEndCol = 0, 2
		e.cursorLine, e.cursorCol = 0, 2
		original, selected := e.Text(), e.selectionSnapshot()
		changes := 0
		e.SigChanged(func(string) { changes++ })
		if remove {
			e.DedentSelection()
		} else {
			e.IndentSelection()
		}
		prefix := "    "
		if remove {
			prefix = ""
		}
		want := prefix + "中\xff🙂\r\n" + prefix + "b\nuntouched"
		if e.Text() != want || changes != 1 || e.selStartLine != 2 || e.selStartCol != 0 || !e.HasSelection() {
			t.Fatal(e.Text(), changes, e.selectionSnapshot())
		}
		after := e.selectionSnapshot()
		e.Undo()
		if e.Text() != original || e.selectionSnapshot() != selected || changes != 2 {
			t.Fatal("undo")
		}
		e.Redo()
		if e.Text() != want || e.selectionSnapshot() != after || changes != 3 {
			t.Fatal("redo")
		}
	}
}

func TestEditorIndentMultiCaretTabOutdentAndEnter(t *testing.T) {
	e := NewCodeEditor()
	e.SetTabSize(4)
	e.SetInsertSpaces(true)
	e.SetText("a\n中🙂\n\tb")
	e.cursorCol = 1
	e.additionalCursors = []cursorPos{{1, 2}, {2, 2}}
	before := append([]cursorPos{{0, 1}}, e.additionalCursors...)
	e.onKeyDownWithShift(KeyTab, false, false)
	if e.Text() != "a   \n中🙂  \n\tb   " {
		t.Fatalf("per-caret tab stops: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "a\n中🙂\n\tb" || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), before) {
		t.Fatal("multi tab undo")
	}
	e.SetText("    a\n\tb")
	e.cursorCol = 4
	e.additionalCursors = []cursorPos{{0, 5}, {1, 1}}
	e.onKeyDownWithShift(KeyTab, false, true)
	if e.Text() != "a\nb" || e.CursorCount() != 3 {
		t.Fatal("outdent each line once", e.Text(), e.CursorCount())
	}
	e.Undo()
	if e.Text() != "    a\n\tb" || e.CursorCount() != 3 {
		t.Fatal("multi outdent undo")
	}
	e.SetTabSize(2)
	e.SetText("if a {\n  if b {")
	e.cursorCol = 6
	e.additionalCursors = []cursorPos{{1, 8}}
	e.onKeyDownWithShift(KeyEnter, false, false)
	if e.Text() != "if a {\n  \n  if b {\n    " {
		t.Fatal("Enter ignored policy", e.Text())
	}
	e.Undo()
	if e.Text() != "if a {\n  if b {" {
		t.Fatal("multi Enter undo")
	}
	e.SetText("    }")
	e.cursorCol = 4
	e.onKeyDownWithShift(KeyEnter, false, false)
	if e.Text() != "    \n  }" {
		t.Fatal("closing brace dedent width", e.Text())
	}
}

func TestEditorIndentTransactionDoesNotStoreWholeDocument(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("x", 1<<20) + "\nsecond")
	e.cursorCol = 1
	e.IndentLines()
	a := e.undoStack[len(e.undoStack)-1]
	if a.kind != 4 || a.oldText != "" || a.text != "" || len(a.group) != 1 || a.group[0].after != "\t" || a.group[0].before != "" {
		t.Fatal("full-document indent history")
	}
	e.Undo()
	if len(e.Text()) != (1<<20)+7 {
		t.Fatal("large undo")
	}
}

func BenchmarkNextLineIndentLongPrefix(b *testing.B) {
	line := strings.Repeat(" ", 8192) + "x"
	b.ReportAllocs()
	for b.Loop() {
		if nextLineIndent(line, 8193, "  ") != line[:8192] {
			b.Fatal("indent")
		}
	}
}
