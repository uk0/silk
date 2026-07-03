package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor edit operations: auto-close brackets/quotes, join lines, trim
// trailing whitespace, duplicate line.
//
// The transforms are extracted as pure helpers (no GL / no Draw) so they can be
// unit-tested directly; the widget methods wire them to the buffer, caret and
// undo funnel. Widget-level tests drive SetText + state + OnTextInput and assert
// buffer/caret, staying GL-free (no Draw), mirroring the existing CodeEditor
// tests.
// ---------------------------------------------------------------------------

// --- autoCloseFor (pure) ---

func TestAutoCloseForPairs(t *testing.T) {
	cases := []struct {
		open rune
		want rune
	}{
		{'(', ')'},
		{'[', ']'},
		{'{', '}'},
		{'"', '"'},
		{'`', '`'},
	}
	for _, c := range cases {
		got, ok := autoCloseFor(c.open)
		if !ok || got != c.want {
			t.Errorf("autoCloseFor(%q) = (%q, %v), want (%q, true)", c.open, got, ok, c.want)
		}
	}
}

func TestAutoCloseForNonBracket(t *testing.T) {
	for _, r := range []rune{'a', '0', ')', ']', '}', ' ', '\t'} {
		if _, ok := autoCloseFor(r); ok {
			t.Errorf("autoCloseFor(%q) reported a pair, want ok=false", r)
		}
	}
}

// --- joinLinesInText (pure) ---

func TestJoinLinesInTextTwoLines(t *testing.T) {
	got := joinLinesInText([]string{"foo", "bar"}, 0, 1)
	want := []string{"foo bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("joinLinesInText = %q, want %q", got, want)
	}
}

func TestJoinLinesInTextCollapsesWhitespace(t *testing.T) {
	// Trailing ws on the first line and leading ws on the next collapse to one space.
	got := joinLinesInText([]string{"foo   ", "\t  bar"}, 0, 1)
	want := []string{"foo bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("joinLinesInText = %q, want %q", got, want)
	}
}

func TestJoinLinesInTextMultiLineRange(t *testing.T) {
	got := joinLinesInText([]string{"a", "b", "c", "d"}, 0, 2)
	want := []string{"a b c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("joinLinesInText = %q, want %q", got, want)
	}
}

func TestJoinLinesInTextLastLineNoOp(t *testing.T) {
	// from >= to (single line, or a range that clamps to the last line) is a no-op.
	in := []string{"a", "b"}
	if got := joinLinesInText(in, 1, 2); !reflect.DeepEqual(got, in) {
		t.Errorf("joinLinesInText last-line = %q, want unchanged %q", got, in)
	}
	if got := joinLinesInText([]string{"only"}, 0, 1); !reflect.DeepEqual(got, []string{"only"}) {
		t.Errorf("joinLinesInText single = %q, want unchanged", got)
	}
}

func TestJoinLinesInTextBlankSides(t *testing.T) {
	if got := joinLinesInText([]string{"", "bar"}, 0, 1); !reflect.DeepEqual(got, []string{"bar"}) {
		t.Errorf("joinLinesInText blank-first = %q, want [\"bar\"]", got)
	}
	if got := joinLinesInText([]string{"foo", ""}, 0, 1); !reflect.DeepEqual(got, []string{"foo"}) {
		t.Errorf("joinLinesInText blank-second = %q, want [\"foo\"]", got)
	}
}

// --- trimTrailingInText (pure) ---

func TestTrimTrailingInText(t *testing.T) {
	in := "a  \nb\t\n  \nc d  \n\te\t "
	want := "a\nb\n\nc d\n\te"
	if got := trimTrailingInText(in); got != want {
		t.Errorf("trimTrailingInText = %q, want %q", got, want)
	}
}

func TestTrimTrailingInTextNoTrailing(t *testing.T) {
	in := "a\nb\nc"
	if got := trimTrailingInText(in); got != in {
		t.Errorf("trimTrailingInText = %q, want unchanged %q", got, in)
	}
}

// --- duplicateLinesInText (pure) ---

func TestDuplicateLinesInTextSingle(t *testing.T) {
	got := duplicateLinesInText([]string{"a", "b"}, 0, 0)
	want := []string{"a", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("duplicateLinesInText = %q, want %q", got, want)
	}
}

func TestDuplicateLinesInTextRange(t *testing.T) {
	got := duplicateLinesInText([]string{"a", "b", "c"}, 0, 1)
	want := []string{"a", "b", "a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("duplicateLinesInText = %q, want %q", got, want)
	}
}

// --- Auto-close at the widget level (OnTextInput) ---

func TestEditOpsAutoCloseInsertsPair(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo")
	e.cursorLine, e.cursorCol = 0, 3

	e.OnTextInput("(")

	if got := e.Text(); got != "foo()" {
		t.Fatalf("Text() = %q, want %q", got, "foo()")
	}
	// Caret parked between the pair.
	if e.cursorCol != 4 {
		t.Errorf("cursorCol = %d, want 4 (between the pair)", e.cursorCol)
	}
	runes := []rune(e.lines[0])
	if runes[e.cursorCol] != ')' {
		t.Errorf("char after caret = %q, want ')'", runes[e.cursorCol])
	}
}

func TestEditOpsAutoCloseQuoteInsertsPair(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x")
	e.cursorLine, e.cursorCol = 0, 1

	e.OnTextInput("\"")

	if got := e.Text(); got != "x\"\"" {
		t.Fatalf("Text() = %q, want %q", got, "x\"\"")
	}
	if e.cursorCol != 2 {
		t.Errorf("cursorCol = %d, want 2 (between the quotes)", e.cursorCol)
	}
}

func TestEditOpsAutoCloseOvertypeSkips(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("()")
	e.cursorLine, e.cursorCol = 0, 1 // between ( and )

	e.OnTextInput(")")

	// Typing ) over an existing ) skips rather than doubling.
	if got := e.Text(); got != "()" {
		t.Fatalf("Text() = %q, want %q (must not double)", got, "()")
	}
	if e.cursorCol != 2 {
		t.Errorf("cursorCol = %d, want 2 (typed over the closer)", e.cursorCol)
	}
}

func TestEditOpsAutoCloseWrapsSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 0, 3
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 0, 3

	e.OnTextInput("(")

	if got := e.Text(); got != "(foo)" {
		t.Fatalf("Text() = %q, want %q", got, "(foo)")
	}
	if !e.HasSelection() {
		t.Fatalf("selection should be preserved after wrap")
	}
	if got := e.SelectedText(); got != "foo" {
		t.Errorf("SelectedText() = %q, want %q (inner text stays selected)", got, "foo")
	}
}

func TestEditOpsAutoCloseWrapsSelectionMultiLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo\nbar")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 1, 3
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 1, 3

	e.OnTextInput("[")

	if got := e.Text(); got != "[foo\nbar]" {
		t.Fatalf("Text() = %q, want %q", got, "[foo\nbar]")
	}
	if got := e.SelectedText(); got != "foo\nbar" {
		t.Errorf("SelectedText() = %q, want %q", got, "foo\nbar")
	}
}

func TestEditOpsAutoCloseNormalCharUnaffected(t *testing.T) {
	// A non-bracket char must type normally (single char, no pair).
	e := NewCodeEditor()
	e.SetText("ab")
	e.cursorLine, e.cursorCol = 0, 1

	e.OnTextInput("x")

	if got := e.Text(); got != "axb" {
		t.Fatalf("Text() = %q, want %q", got, "axb")
	}
	if e.cursorCol != 2 {
		t.Errorf("cursorCol = %d, want 2", e.cursorCol)
	}
}

func TestEditOpsAutoCloseNormalCharReplacesSelection(t *testing.T) {
	// Typing a non-opener over a selection replaces it (does NOT wrap).
	e := NewCodeEditor()
	e.SetText("foo")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 0, 3
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 0, 3

	e.OnTextInput("x")

	if got := e.Text(); got != "x" {
		t.Errorf("Text() = %q, want %q (selection replaced, not wrapped)", got, "x")
	}
}

func TestEditOpsAutoCloseUndoRemovesPair(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo")
	e.cursorLine, e.cursorCol = 0, 3

	e.OnTextInput("(") // -> "foo()"
	if e.Text() != "foo()" {
		t.Fatalf("pre-undo Text() = %q, want %q", e.Text(), "foo()")
	}
	e.undo()
	if got := e.Text(); got != "foo" {
		t.Errorf("post-undo Text() = %q, want %q (whole pair removed)", got, "foo")
	}
}

// --- JoinLines (widget) ---

func TestEditOpsJoinLinesWidget(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo\nbar")
	e.cursorLine, e.cursorCol = 0, 0
	changed := 0
	e.onChanged = func(string) { changed++ }

	e.JoinLines()

	if got := e.Text(); got != "foo bar" {
		t.Fatalf("Text() = %q, want %q", got, "foo bar")
	}
	if e.cursorLine != 0 || e.cursorCol != 3 {
		t.Errorf("caret = (%d,%d), want (0,3)", e.cursorLine, e.cursorCol)
	}
	if changed == 0 {
		t.Errorf("onChanged not fired: mutation did not route through rebuildText")
	}
	e.undo()
	if got := e.Text(); got != "foo\nbar" {
		t.Errorf("post-undo Text() = %q, want %q", got, "foo\nbar")
	}
}

func TestEditOpsJoinLinesLastLineNoOp(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("only")
	e.cursorLine, e.cursorCol = 0, 0

	e.JoinLines()

	if got := e.Text(); got != "only" {
		t.Errorf("Text() = %q, want unchanged %q", got, "only")
	}
}

func TestEditOpsJoinLinesSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 1
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 1

	e.JoinLines()

	if got := e.Text(); got != "a b c\nd" {
		t.Errorf("Text() = %q, want %q", got, "a b c\nd")
	}
}

// --- DuplicateLines (widget) ---

func TestEditOpsDuplicateLineSingle(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x\ny")
	e.cursorLine, e.cursorCol = 0, 0

	e.DuplicateLines()

	if got := e.Text(); got != "x\nx\ny" {
		t.Fatalf("Text() = %q, want %q", got, "x\nx\ny")
	}
	if e.cursorLine != 1 {
		t.Errorf("cursorLine = %d, want 1 (on the copy)", e.cursorLine)
	}
}

func TestEditOpsDuplicateLinesSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 1, 1
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 1, 1

	e.DuplicateLines()

	if got := e.Text(); got != "a\nb\na\nb\nc" {
		t.Fatalf("Text() = %q, want %q", got, "a\nb\na\nb\nc")
	}
	// The copy (lines 2..3) is re-selected.
	if e.selStartLine != 2 || e.selEndLine != 3 {
		t.Errorf("selection lines = %d..%d, want 2..3", e.selStartLine, e.selEndLine)
	}
	e.undo()
	if got := e.Text(); got != "a\nb\nc" {
		t.Errorf("post-undo Text() = %q, want %q", got, "a\nb\nc")
	}
}

// --- TrimTrailingWhitespace (widget) ---

func TestEditOpsTrimTrailingWidget(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a  \nb\t\n  \nc d  ")
	e.cursorLine, e.cursorCol = 0, 3 // caret parked in the trailing whitespace
	changed := 0
	e.onChanged = func(string) { changed++ }

	e.TrimTrailingWhitespace()

	if got := e.Text(); got != "a\nb\n\nc d" {
		t.Fatalf("Text() = %q, want %q", got, "a\nb\n\nc d")
	}
	// Caret column clamped down to the now-shorter line.
	if e.cursorCol > len([]rune(e.lines[0])) {
		t.Errorf("cursorCol = %d, want <= %d after trim", e.cursorCol, len([]rune(e.lines[0])))
	}
	if changed == 0 {
		t.Errorf("onChanged not fired: mutation did not route through rebuildText")
	}
	e.undo()
	if got := e.Text(); got != "a  \nb\t\n  \nc d  " {
		t.Errorf("post-undo Text() = %q, want original with trailing ws restored", got)
	}
}
