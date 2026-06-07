package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure helpers
//
// applyInsertAtCursors / applyBackspaceAtCursors / applyDeleteAtCursors /
// dedupCursorsList sit beside the mutating *AtAllCursors methods so multi-
// cursor edit math is exercisable without spinning up a CodeEditor instance.
// ---------------------------------------------------------------------------

// TestDedupCursorsListRemovesDuplicates: identical (line, col) pairs collapse
// to one occurrence; original ordering of the survivors is preserved.
func TestDedupCursorsListRemovesDuplicates(t *testing.T) {
	in := []cursorPos{
		{line: 2, col: 4},
		{line: 0, col: 0},
		{line: 2, col: 4}, // duplicate of [0]
		{line: 1, col: 7},
		{line: 0, col: 0}, // duplicate of [1]
	}
	got := dedupCursorsList(in)
	want := []cursorPos{
		{line: 2, col: 4},
		{line: 0, col: 0},
		{line: 1, col: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupCursorsList = %v, want %v", got, want)
	}
}

// TestDedupCursorsListEmpty: empty input returns nil, no allocation.
func TestDedupCursorsListEmpty(t *testing.T) {
	if got := dedupCursorsList(nil); got != nil {
		t.Errorf("dedupCursorsList(nil) = %v, want nil", got)
	}
	if got := dedupCursorsList([]cursorPos{}); got != nil {
		t.Errorf("dedupCursorsList([]) = %v, want nil", got)
	}
}

// TestApplyInsertAtCursorsThreeCursors: insert one rune at 3 positions on a
// single line. Right-to-left processing must shift each later cursor by the
// total inserted length to its left, and the buffer must reflect all three
// insertions.
func TestApplyInsertAtCursorsThreeCursors(t *testing.T) {
	// Buffer: "abc def ghi"
	//          ^ p   ^   ^
	// Cursors at col 0 (primary), col 4, col 8.
	text := "abc def ghi"
	primary := cursorPos{line: 0, col: 0}
	extras := []cursorPos{
		{line: 0, col: 4},
		{line: 0, col: 8},
	}
	newText, newPrimary, newExtras := applyInsertAtCursors(text, primary, extras, "X")
	// Each of the three carets inserts an "X":
	// col 0 → "Xabc def ghi"
	// col 4 (now 5) → "Xabc Xdef ghi"
	// col 8 (now 10) → "Xabc Xdef Xghi"
	if newText != "Xabc Xdef Xghi" {
		t.Errorf("text = %q, want %q", newText, "Xabc Xdef Xghi")
	}
	// Primary started at col 0; after its own insertion it advances by 1.
	if newPrimary != (cursorPos{line: 0, col: 1}) {
		t.Errorf("primary = %v, want {0,1}", newPrimary)
	}
	wantExtras := []cursorPos{
		{line: 0, col: 6},  // 4 + 1 (primary insertion to its left) + 1 (own)
		{line: 0, col: 11}, // 8 + 2 (two earlier insertions) + 1 (own)
	}
	if !reflect.DeepEqual(newExtras, wantExtras) {
		t.Errorf("extras = %v, want %v", newExtras, wantExtras)
	}
}

// TestApplyInsertAtCursorsMultiLine: cursors on different lines insert
// independently; columns on other lines are untouched.
func TestApplyInsertAtCursorsMultiLine(t *testing.T) {
	text := "foo\nbar\nbaz"
	primary := cursorPos{line: 0, col: 3} // end of "foo"
	extras := []cursorPos{
		{line: 1, col: 3}, // end of "bar"
		{line: 2, col: 3}, // end of "baz"
	}
	newText, newPrimary, newExtras := applyInsertAtCursors(text, primary, extras, "!")
	if newText != "foo!\nbar!\nbaz!" {
		t.Errorf("text = %q, want %q", newText, "foo!\nbar!\nbaz!")
	}
	if newPrimary != (cursorPos{line: 0, col: 4}) {
		t.Errorf("primary = %v, want {0,4}", newPrimary)
	}
	wantExtras := []cursorPos{
		{line: 1, col: 4},
		{line: 2, col: 4},
	}
	if !reflect.DeepEqual(newExtras, wantExtras) {
		t.Errorf("extras = %v, want %v", newExtras, wantExtras)
	}
}

// TestApplyBackspaceAtCursorsSameLine: three carets on one line each delete
// the rune to their left.
func TestApplyBackspaceAtCursorsSameLine(t *testing.T) {
	// "abc def ghi" with cursors after each three-letter word.
	text := "abc def ghi"
	primary := cursorPos{line: 0, col: 3} // after "abc"
	extras := []cursorPos{
		{line: 0, col: 7},  // after "def"
		{line: 0, col: 11}, // after "ghi"
	}
	newText, _, _ := applyBackspaceAtCursors(text, primary, extras)
	if newText != "ab de gh" {
		t.Errorf("text = %q, want %q", newText, "ab de gh")
	}
}

// TestApplyDeleteAtCursorsSameLine: three carets each delete the rune to
// their right.
func TestApplyDeleteAtCursorsSameLine(t *testing.T) {
	text := "abc def ghi"
	primary := cursorPos{line: 0, col: 0} // before "abc"
	extras := []cursorPos{
		{line: 0, col: 4}, // before "def"
		{line: 0, col: 8}, // before "ghi"
	}
	newText, _, _ := applyDeleteAtCursors(text, primary, extras)
	if newText != "bc ef hi" {
		t.Errorf("text = %q, want %q", newText, "bc ef hi")
	}
}

// ---------------------------------------------------------------------------
// Public-API drive (OnTextInput / OnKeyDown indirect via internal methods)
//
// OnKeyDown polls live keyboard state (IsKeyDown), unreachable in a headless
// test. We exercise the methods OnKeyDown delegates to: selectNextOccurrence,
// ClearAdditionalCursors, OnTextInput (no modifiers consulted there).
// ---------------------------------------------------------------------------

// TestCodeEditorSelectNextOccurrenceBuildsCursors: the editor starts at the
// first "foo"; one Cmd+D selects that word and adds a cursor at the second
// "foo" (total 2 carets); a second Cmd+D adds the third (total 3 carets).
func TestCodeEditorSelectNextOccurrenceBuildsCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo bar foo baz foo")
	// Place cursor on the first "foo".
	e.cursorLine = 0
	e.cursorCol = 1 // inside "foo"

	e.selectNextOccurrence()
	if got, want := len(e.additionalCursors), 1; got != want {
		t.Fatalf("after 1st Cmd+D: additionalCursors=%d, want %d", got, want)
	}
	if !e.hasSelection {
		t.Fatalf("after 1st Cmd+D: expected primary selection over 'foo'")
	}

	e.selectNextOccurrence()
	if got, want := len(e.additionalCursors), 2; got != want {
		t.Fatalf("after 2nd Cmd+D: additionalCursors=%d, want %d", got, want)
	}

	// Each secondary cursor sits at the END of its occurrence (col 3 from
	// the start of each match).
	want := map[cursorPos]bool{
		{line: 0, col: 11}: true, // end of second  "foo" at column 8..11
		{line: 0, col: 19}: true, // end of third   "foo" at column 16..19
	}
	for _, c := range e.additionalCursors {
		if !want[c] {
			t.Errorf("unexpected secondary cursor %v; want one of %v", c, want)
		}
	}
}

// TestCodeEditorEscClearsAdditionalCursors: ClearAdditionalCursors (what Esc
// calls when no popup is open) drops every secondary caret.
func TestCodeEditorEscClearsAdditionalCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo foo foo")
	e.cursorLine = 0
	e.cursorCol = 1
	e.selectNextOccurrence()
	e.selectNextOccurrence()
	if len(e.additionalCursors) == 0 {
		t.Fatal("setup failed: expected additional cursors before Esc")
	}
	e.ClearAdditionalCursors()
	if got := len(e.additionalCursors); got != 0 {
		t.Errorf("after Esc: additionalCursors=%d, want 0", got)
	}
}

// TestCodeEditorOnTextInputAtAllCursors: typing a single character routes
// through OnTextInput, which inserts at the primary AND every secondary
// caret. This is the same path Cmd+D users hit while typing.
func TestCodeEditorOnTextInputAtAllCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo foo foo")
	// Two secondary cursors after the second/third "foo".
	e.cursorLine = 0
	e.cursorCol = 3 // end of first "foo"
	e.additionalCursors = []cursorPos{
		{line: 0, col: 7},  // end of second
		{line: 0, col: 11}, // end of third
	}
	e.OnTextInput("!")
	if got, want := e.Text(), "foo! foo! foo!"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	// Each cursor advances by one rune.
	if e.cursorCol != 4 {
		t.Errorf("primary cursorCol = %d, want 4", e.cursorCol)
	}
	wantExtras := []cursorPos{
		{line: 0, col: 9},
		{line: 0, col: 14},
	}
	if !reflect.DeepEqual(e.additionalCursors, wantExtras) {
		t.Errorf("additionalCursors = %v, want %v", e.additionalCursors, wantExtras)
	}
}

// TestCodeEditorBackspaceAtAllCursors: backspaceAtAllCursors deletes the
// rune to the left of every caret. Verifies the OnKeyDown(KeyBackSpace)
// delegate path that fires when secondary cursors are active.
func TestCodeEditorBackspaceAtAllCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc def ghi")
	e.cursorLine = 0
	e.cursorCol = 3 // after "abc"
	e.additionalCursors = []cursorPos{
		{line: 0, col: 7},  // after "def"
		{line: 0, col: 11}, // after "ghi"
	}
	e.backspaceAtAllCursors()
	e.rebuildText()
	if got, want := e.Text(), "ab de gh"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

// TestCodeEditorDeleteAtAllCursors: forward-delete at every caret.
func TestCodeEditorDeleteAtAllCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc def ghi")
	e.cursorLine = 0
	e.cursorCol = 0 // before "abc"
	e.additionalCursors = []cursorPos{
		{line: 0, col: 4}, // before "def"
		{line: 0, col: 8}, // before "ghi"
	}
	e.deleteAtAllCursors()
	e.rebuildText()
	if got, want := e.Text(), "bc ef hi"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

// TestCodeEditorMoveAllCursorsByLeftRight: plain arrow-key movement
// advances every cursor by the same delta, clamped at line boundaries.
func TestCodeEditorMoveAllCursorsByLeftRight(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abcdef\nghijkl\nmnopqr")
	e.cursorLine = 0
	e.cursorCol = 3
	e.additionalCursors = []cursorPos{
		{line: 1, col: 3},
		{line: 2, col: 3},
	}
	e.moveAllCursorsBy(0, +1)
	if e.cursorCol != 4 {
		t.Errorf("primary col after Right = %d, want 4", e.cursorCol)
	}
	wantExtras := []cursorPos{
		{line: 1, col: 4},
		{line: 2, col: 4},
	}
	if !reflect.DeepEqual(e.additionalCursors, wantExtras) {
		t.Errorf("extras after Right = %v, want %v", e.additionalCursors, wantExtras)
	}
	e.moveAllCursorsBy(0, -1)
	if e.cursorCol != 3 {
		t.Errorf("primary col after Left = %d, want 3", e.cursorCol)
	}
}

// TestCodeEditorMoveAllCursorsByVerticalDedup: two cursors moved vertically
// to the same (line, col) collapse to one.
func TestCodeEditorMoveAllCursorsByVerticalDedup(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	// Primary on line 0 col 0, secondary on line 2 col 0. Moving up by 1
	// puts primary at (-1 → 0, 0) and secondary at (1, 0). Distinct.
	e.cursorLine = 1
	e.cursorCol = 0
	e.additionalCursors = []cursorPos{{line: 2, col: 0}}
	e.moveAllCursorsBy(-1, 0)
	// primary now 0,0 ; secondary now 1,0 — both distinct, no dedup.
	if e.cursorLine != 0 || len(e.additionalCursors) != 1 {
		t.Errorf("after up: primary=%d,%d extras=%v", e.cursorLine, e.cursorCol, e.additionalCursors)
	}
	// Move up once more: primary clamps at 0,0; secondary becomes 0,0 → dedup.
	e.moveAllCursorsBy(-1, 0)
	if e.cursorLine != 0 || e.cursorCol != 0 {
		t.Errorf("primary = %d,%d, want 0,0", e.cursorLine, e.cursorCol)
	}
	if len(e.additionalCursors) != 0 {
		t.Errorf("expected dedup to collapse coincident cursor, got %v", e.additionalCursors)
	}
}

// TestCodeEditorMoveAllCursorsToLineBound: Home/End jumps every cursor to
// col 0 / end of its own line.
func TestCodeEditorMoveAllCursorsToLineBound(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc\ndefgh\nij")
	e.cursorLine = 0
	e.cursorCol = 2
	e.additionalCursors = []cursorPos{
		{line: 1, col: 1},
		{line: 2, col: 1},
	}
	e.moveAllCursorsToLineBound(true)
	if e.cursorCol != 3 {
		t.Errorf("primary col after End = %d, want 3 (len of 'abc')", e.cursorCol)
	}
	wantExtras := []cursorPos{
		{line: 1, col: 5}, // len of "defgh"
		{line: 2, col: 2}, // len of "ij"
	}
	if !reflect.DeepEqual(e.additionalCursors, wantExtras) {
		t.Errorf("extras after End = %v, want %v", e.additionalCursors, wantExtras)
	}
	e.moveAllCursorsToLineBound(false)
	if e.cursorCol != 0 {
		t.Errorf("primary col after Home = %d, want 0", e.cursorCol)
	}
	for _, c := range e.additionalCursors {
		if c.col != 0 {
			t.Errorf("secondary col after Home = %d, want 0", c.col)
		}
	}
}
