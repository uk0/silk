package gui

import (
	"strings"
	"testing"
)

func TestRuneColumnFromUTF16(t *testing.T) {
	for _, tc := range []struct {
		name, text   string
		column, want int
	}{
		{"empty", "", 10, 0}, {"negative", "abc", -1, 0},
		{"ascii", "abc", 2, 2}, {"bmp", "中文a", 2, 2},
		{"start", "a😀b", 1, 1}, {"surrogate", "a😀b", 2, 1},
		{"after", "a😀b", 3, 2}, {"end", "a😀b", 4, 3},
		{"beyond", "a😀b", int(^uint(0) >> 1), 3},
		{"combining", "e\u0301😀x", 4, 3},
		{"invalid", "a\xff😀z", 4, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuneColumnFromUTF16(tc.text, tc.column); got != tc.want {
				t.Fatalf("column %d in %q = %d, want %d", tc.column, tc.text, got, tc.want)
			}
		})
	}
	line := strings.Repeat("中文😀", 10000)
	if allocations := testing.AllocsPerRun(10, func() { RuneColumnFromUTF16(line, 39999) }); allocations != 0 {
		t.Fatalf("conversion allocated %g times", allocations)
	}
}

func TestCodeEditorScrollToUTF16Position(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("😀first\n中文🧪 target\ne\u0301x")
	for _, tc := range []struct{ line, column, wantLine, wantColumn int }{
		{1, 5, 1, 4}, {1, 3, 1, 2}, {1, -10, 1, 0},
		{-100, 2, 0, 1}, {100, 100, 2, 3}, {2, 2, 2, 2},
	} {
		ed.ScrollToUTF16Position(tc.line, tc.column)
		if ed.CursorLine() != tc.wantLine || ed.CursorCol() != tc.wantColumn {
			t.Fatalf("input %+v: got %d:%d", tc, ed.CursorLine(), ed.CursorCol())
		}
	}
	ed.SetText("")
	ed.ScrollToUTF16Position(20, 30)
	if ed.CursorLine() != 0 || ed.CursorCol() != 0 {
		t.Fatal("empty buffer caret is invalid")
	}
}

func TestNavigateToLineColRetiresSelectionCaretsAndRevealsTarget(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	e := NewCodeEditor()
	e.SetSize(200, 100)
	source := "package p\nfunc Example() {\n if true {\n  target()\n }\n}\n"
	e.SetText(source)
	e.FoldAll()
	e.editorSelectAll()
	e.AddCursorAtLine(1, 1)
	if !e.HasSelection() || len(e.additionalCursors) == 0 {
		t.Fatal("invalid selection fixture")
	}
	changed, undo := 0, len(e.undoStack)
	e.onChanged = func(string) { changed++ }
	e.NavigateToLineCol(3, 2)
	if e.HasSelection() || len(e.additionalCursors) != 0 || !e.HasFocus() || e.CursorLine() != 3 || e.CursorCol() != 2 {
		t.Fatal("interactive navigation retained old editing targets")
	}
	for _, r := range e.FoldRegions() {
		if r.startLine < 3 && 3 <= r.endLine && e.IsFolded(r.startLine) {
			t.Fatal("target remains folded")
		}
	}
	if e.Text() != source || changed != 0 || len(e.undoStack) != undo {
		t.Fatal("navigation mutated source/history")
	}
	e.OnTextInput("QA_")
	if e.Text() != strings.Replace(source, "target()", "QA_target()", 1) {
		t.Fatal("typing replaced old selection or used extra caret", e.Text())
	}
	e.Undo()
	if e.Text() != source {
		t.Fatal("navigation typing undo mismatch")
	}
	e.SetText("中文😀")
	e.NavigateToLineCol(99, 99)
	if e.CursorLine() != 0 || e.CursorCol() != 3 {
		t.Fatal("rune clamping failed")
	}
	e.SetText("")
	e.NavigateToLineCol(-99, -99)
	if e.CursorLine() != 0 || e.CursorCol() != 0 {
		t.Fatal("empty cursor invalid")
	}
}
