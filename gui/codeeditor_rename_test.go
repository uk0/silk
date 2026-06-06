package gui

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// RenameSymbolAtCursor: AST-based rename of the identifier under the cursor.
//
// The editor exposes this for the host (silkide) to wire to F2 via an input
// dialog. The method itself does not bind any key; it grabs the word at the
// cursor, delegates to code_refactor.go::RenameSymbolCount, swaps the buffer,
// fires the changed callback, and snaps the cursor back to the same byte
// offset so the caret stays roughly on the renamed symbol.
// ---------------------------------------------------------------------------

func TestRenameSymbolAtCursorBasic(t *testing.T) {
	e := NewCodeEditor()
	// RenameSymbolCount requires the source to parse, which means a leading
	// "package" clause; the rename itself is exercised on the two "Foo"
	// occurrences on lines 1 and 2.
	src := "package p\nfunc Foo() {}\nfunc main() { Foo() }"
	e.SetText(src)
	// Place the cursor inside the SECOND occurrence of "Foo" on line 2.
	// Line 2: "func main() { Foo() }" — col 14 is 'F', col 15 is the first 'o'.
	e.cursorLine = 2
	e.cursorCol = 15

	var fired string
	e.SigChanged(func(s string) { fired = s })

	oldName, count, err := e.RenameSymbolAtCursor("Bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldName != "Foo" {
		t.Errorf("oldName = %q, want Foo", oldName)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	got := e.Text()
	if !strings.Contains(got, "func Bar() {}") {
		t.Errorf("buffer missing declaration rename: %q", got)
	}
	if !strings.Contains(got, "Bar()") {
		t.Errorf("buffer missing call-site rename: %q", got)
	}
	if strings.Contains(got, "Foo") {
		t.Errorf("buffer still contains old name: %q", got)
	}
	if fired != got {
		t.Errorf("changed callback fired %q, want %q", fired, got)
	}
	// Cursor should still sit somewhere on the renamed call site (line 2,
	// inside "Bar"). We don't pin the exact column because the old/new name
	// lengths happen to match here, but the cursor must not have escaped the
	// renamed identifier neighborhood.
	if e.cursorLine != 2 {
		t.Errorf("cursorLine = %d, want 2", e.cursorLine)
	}
	if e.cursorCol < 14 || e.cursorCol > 17 {
		t.Errorf("cursorCol = %d, want roughly 14..17 (inside renamed identifier)", e.cursorCol)
	}
}

func TestRenameSymbolAtCursorEmptyWord(t *testing.T) {
	e := NewCodeEditor()
	// Blank first line — cursor at (0,0) lands on no identifier.
	e.SetText("\nfunc Foo() {}")
	e.cursorLine = 0
	e.cursorCol = 0

	before := e.Text()
	_, count, err := e.RenameSymbolAtCursor("Bar")
	if err == nil {
		t.Fatal("expected error when cursor word is empty, got nil")
	}
	if count != 0 {
		t.Errorf("count = %d on error, want 0", count)
	}
	if e.Text() != before {
		t.Errorf("buffer mutated on empty-cursor-word error: %q", e.Text())
	}
}

func TestRenameSymbolAtCursorInvalidNewName(t *testing.T) {
	cases := []struct {
		name    string
		newName string
	}{
		{"empty", ""},
		{"reserved", "if"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewCodeEditor()
			src := "func Foo() {}\nfunc main() { Foo() }"
			e.SetText(src)
			e.cursorLine = 0
			e.cursorCol = 6 // inside "Foo" on the decl line

			_, count, err := e.RenameSymbolAtCursor(tc.newName)
			if err == nil {
				t.Fatalf("expected error for invalid newName %q, got nil", tc.newName)
			}
			if count != 0 {
				t.Errorf("count = %d on invalid newName, want 0", count)
			}
			if e.Text() != src {
				t.Errorf("buffer mutated on invalid newName: %q", e.Text())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// applyLineIndent pure helper.
// ---------------------------------------------------------------------------

func TestApplyLineIndentInsertTab(t *testing.T) {
	in := []string{"a", "b", "c"}
	got, deltas := applyLineIndent(in, 0, 2, "\t", false)
	want := []string{"\ta", "\tb", "\tc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(deltas, []int{1, 1, 1}) {
		t.Errorf("deltas = %v, want [1 1 1]", deltas)
	}
	// Input must not be mutated.
	if !reflect.DeepEqual(in, []string{"a", "b", "c"}) {
		t.Errorf("input mutated: %q", in)
	}
}

func TestApplyLineIndentRemoveTab(t *testing.T) {
	in := []string{"\ta", "\tb", "\tc"}
	got, deltas := applyLineIndent(in, 0, 2, "\t", true)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(deltas, []int{-1, -1, -1}) {
		t.Errorf("deltas = %v, want [-1 -1 -1]", deltas)
	}
}

// Mix of tabs and spaces: leading tab strips as 1, leading <=4 spaces strip
// up to 4, lines without leading whitespace are left alone (delta 0).
func TestApplyLineIndentRemoveMixedTabsAndSpaces(t *testing.T) {
	in := []string{"\tx", "    y", "  z", "no-indent"}
	got, deltas := applyLineIndent(in, 0, 3, "\t", true)
	want := []string{"x", "y", "z", "no-indent"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(deltas, []int{-1, -4, -2, 0}) {
		t.Errorf("deltas = %v, want [-1 -4 -2 0]", deltas)
	}
}

// Range narrower than the input slice: only [startLine, endLine] is touched;
// the surrounding lines pass through verbatim with delta 0.
func TestApplyLineIndentRangeSubset(t *testing.T) {
	in := []string{"keep", "a", "b", "keep"}
	got, deltas := applyLineIndent(in, 1, 2, "\t", false)
	want := []string{"keep", "\ta", "\tb", "keep"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(deltas, []int{0, 1, 1, 0}) {
		t.Errorf("deltas = %v, want [0 1 1 0]", deltas)
	}
}

// ---------------------------------------------------------------------------
// IndentSelection / DedentSelection against the editor buffer.
// ---------------------------------------------------------------------------

func TestCodeEditorIndentSelectionMultiLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	// Select all three lines.
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 1
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 1

	var fired string
	e.SigChanged(func(s string) { fired = s })

	e.IndentSelection()

	want := "\ta\n\tb\n\tc"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if fired != want {
		t.Errorf("changed callback fired %q, want %q", fired, want)
	}
	if !e.HasSelection() {
		t.Fatalf("selection should persist after indent")
	}
	if e.selStartLine != 0 || e.selEndLine != 2 {
		t.Errorf("selection lines = %d..%d, want 0..2", e.selStartLine, e.selEndLine)
	}
	for i, ln := range e.lines {
		if !strings.HasPrefix(ln, "\t") {
			t.Errorf("line %d = %q, want leading tab", i, ln)
		}
	}
}

func TestCodeEditorDedentSelectionMultiLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\ta\n\tb\n\tc")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 2
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 2

	e.DedentSelection()

	want := "a\nb\nc"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	for i, ln := range e.lines {
		if strings.HasPrefix(ln, "\t") {
			t.Errorf("line %d = %q, still has leading tab", i, ln)
		}
	}
}

// DedentSelection on lines without leading whitespace is a no-op for those
// lines — the helper documents delta 0 in that case, so the buffer must be
// preserved character-for-character.
func TestCodeEditorDedentSelectionNoLeadingWhitespace(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 1
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 1

	e.DedentSelection()

	if got := e.Text(); got != "a\nb\nc" {
		t.Errorf("Text() = %q, want unchanged %q", got, "a\nb\nc")
	}
}

// Single-line Tab (selection on one line) must NOT route through the
// multi-line indent path; OnKeyDown reserves IndentSelection / DedentSelection
// for selections that span at least two lines. This regression test pins the
// behaviour by calling IndentSelection on a single-line selection and
// asserting it still indents (the public method is unconditional) — the
// single-line guard lives in OnKeyDown, not in IndentSelection itself.
func TestCodeEditorIndentSelectionSingleLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("hello")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 0, 5
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 0, 5

	e.IndentSelection()
	if got := e.Text(); got != "\thello" {
		t.Errorf("Text() = %q, want %q", got, "\thello")
	}
}
