package gui

import (
	"reflect"
	"strings"
	"testing"
)

// Unit tests for the TextArea multi-line plain-text input. The pure
// insert / delete-range helpers are exercised in isolation (no widget
// state, no GL); the widget-facing API is then driven through its
// public methods. We deliberately stay off of Draw — the test harness
// has no Cairo context.

// --- textAreaInsert ---------------------------------------------------

// TestTextAreaInsertAtStart: inserting at (0,0) prepends to the first
// line and leaves the caret just past the inserted text.
func TestTextAreaInsertAtStart(t *testing.T) {
	lines, line, col := textAreaInsert([]string{"world"}, 0, 0, "hello ")
	if !reflect.DeepEqual(lines, []string{"hello world"}) {
		t.Fatalf("lines = %q, want [hello world]", lines)
	}
	if line != 0 || col != 6 {
		t.Fatalf("caret = (%d,%d), want (0,6)", line, col)
	}
}

// TestTextAreaInsertAtMiddle: inserting mid-line splices in cleanly
// and lands the caret at the splice point.
func TestTextAreaInsertAtMiddle(t *testing.T) {
	lines, line, col := textAreaInsert([]string{"abef"}, 0, 2, "cd")
	if !reflect.DeepEqual(lines, []string{"abcdef"}) {
		t.Fatalf("lines = %q, want [abcdef]", lines)
	}
	if line != 0 || col != 4 {
		t.Fatalf("caret = (%d,%d), want (0,4)", line, col)
	}
}

// TestTextAreaInsertAtEnd: appending past the end clamps col to the
// line length, then inserts.
func TestTextAreaInsertAtEnd(t *testing.T) {
	lines, line, col := textAreaInsert([]string{"foo"}, 0, 999, "bar")
	if !reflect.DeepEqual(lines, []string{"foobar"}) {
		t.Fatalf("lines = %q, want [foobar]", lines)
	}
	if line != 0 || col != 6 {
		t.Fatalf("caret = (%d,%d), want (0,6)", line, col)
	}
}

// TestTextAreaInsertNewline: a lone "\n" splits the current line and
// lands the caret at column 0 of the new line.
func TestTextAreaInsertNewline(t *testing.T) {
	lines, line, col := textAreaInsert([]string{"abcdef"}, 0, 3, "\n")
	if !reflect.DeepEqual(lines, []string{"abc", "def"}) {
		t.Fatalf("lines = %q, want [abc def]", lines)
	}
	if line != 1 || col != 0 {
		t.Fatalf("caret = (%d,%d), want (1,0)", line, col)
	}
}

// TestTextAreaInsertMultiLine: a "\n"-bearing string splits into
// multiple new lines and parks the caret on the last inserted line.
func TestTextAreaInsertMultiLine(t *testing.T) {
	lines, line, col := textAreaInsert([]string{"head", "tail"}, 0, 4, "X\nY\nZ")
	if !reflect.DeepEqual(lines, []string{"headX", "Y", "Z", "tail"}) {
		t.Fatalf("lines = %q", lines)
	}
	if line != 2 || col != 1 {
		t.Fatalf("caret = (%d,%d), want (2,1)", line, col)
	}
}

// TestTextAreaInsertIntoEmpty: a fresh insert into an empty buffer
// still yields a single-line result with the inserted text.
func TestTextAreaInsertIntoEmpty(t *testing.T) {
	lines, line, col := textAreaInsert([]string{""}, 0, 0, "hi")
	if !reflect.DeepEqual(lines, []string{"hi"}) {
		t.Fatalf("lines = %q, want [hi]", lines)
	}
	if line != 0 || col != 2 {
		t.Fatalf("caret = (%d,%d), want (0,2)", line, col)
	}
}

// --- textAreaDeleteRange ---------------------------------------------

// TestTextAreaDeleteSameLine: removing a same-line span splices the
// remainder and lands the caret at the start.
func TestTextAreaDeleteSameLine(t *testing.T) {
	lines, line, col := textAreaDeleteRange([]string{"hello world"}, 0, 5, 0, 11)
	if !reflect.DeepEqual(lines, []string{"hello"}) {
		t.Fatalf("lines = %q, want [hello]", lines)
	}
	if line != 0 || col != 5 {
		t.Fatalf("caret = (%d,%d), want (0,5)", line, col)
	}
}

// TestTextAreaDeleteAcrossLines: a multi-line delete merges the head
// of the first line with the tail of the last line, dropping the
// lines in between.
func TestTextAreaDeleteAcrossLines(t *testing.T) {
	lines, line, col := textAreaDeleteRange([]string{"one", "two", "three"}, 0, 1, 2, 2)
	if !reflect.DeepEqual(lines, []string{"oree"}) {
		t.Fatalf("lines = %q, want [oree]", lines)
	}
	if line != 0 || col != 1 {
		t.Fatalf("caret = (%d,%d), want (0,1)", line, col)
	}
}

// TestTextAreaDeleteReversedRange: passing the points out-of-order is
// idempotent — the helper canonicalises start <= end.
func TestTextAreaDeleteReversedRange(t *testing.T) {
	lines, line, col := textAreaDeleteRange([]string{"abcdef"}, 0, 5, 0, 1)
	if !reflect.DeepEqual(lines, []string{"af"}) {
		t.Fatalf("lines = %q, want [af]", lines)
	}
	if line != 0 || col != 1 {
		t.Fatalf("caret = (%d,%d), want (0,1)", line, col)
	}
}

// TestTextAreaDeleteIdempotentEmptyRange: deleting an empty range
// returns a slice that compares equal to the input.
func TestTextAreaDeleteEmptyRange(t *testing.T) {
	lines, line, col := textAreaDeleteRange([]string{"hello", "world"}, 1, 2, 1, 2)
	if !reflect.DeepEqual(lines, []string{"hello", "world"}) {
		t.Fatalf("lines = %q, want unchanged", lines)
	}
	if line != 1 || col != 2 {
		t.Fatalf("caret = (%d,%d), want (1,2)", line, col)
	}
}

// --- public API round-trips ------------------------------------------

// TestTextAreaSetTextRoundTrip: SetText + Text returns the original
// string verbatim, including embedded newlines.
func TestTextAreaSetTextRoundTrip(t *testing.T) {
	ta := NewTextArea()
	in := "line1\nline2\nline3"
	ta.SetText(in)
	if got := ta.Text(); got != in {
		t.Fatalf("Text() = %q, want %q", got, in)
	}
	if got := ta.LineCount(); got != 3 {
		t.Fatalf("LineCount() = %d, want 3", got)
	}
}

// TestTextAreaSetTextEmpty: an empty SetText still leaves the widget
// with exactly one (empty) line so the rendering and cursor math
// don't need to special-case an empty buffer.
func TestTextAreaSetTextEmpty(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("")
	if got := ta.LineCount(); got != 1 {
		t.Fatalf("LineCount() = %d, want 1", got)
	}
	if got := ta.Text(); got != "" {
		t.Fatalf("Text() = %q, want empty", got)
	}
}

// TestTextAreaOnTextInputInsertsAndFires: OnTextInput inserts at the
// caret and fires SigTextChanged with the new full text.
func TestTextAreaOnTextInputInsertsAndFires(t *testing.T) {
	ta := NewTextArea()
	var last string
	var count int
	ta.SigTextChanged(func(_ interface{}, s string) {
		last = s
		count++
	})
	ta.OnTextInput("hello")
	if got := ta.Text(); got != "hello" {
		t.Fatalf("Text() = %q, want hello", got)
	}
	if count == 0 || last != "hello" {
		t.Fatalf("SigTextChanged not fired correctly: count=%d last=%q", count, last)
	}
}

// TestTextAreaEnterInsertsNewline: KeyEnter through OnKeyDown splits
// the line at the caret and grows the line count. This is the key
// behavioural difference from single-line Edit, which would call
// Submit on Enter.
func TestTextAreaEnterInsertsNewline(t *testing.T) {
	ta := NewTextArea()
	ta.OnTextInput("abcdef")
	// Place caret at col 3.
	ta.moveCursor(0, 3, false)
	before := ta.LineCount()
	ta.OnKeyDown(KeyEnter, false)
	if got := ta.LineCount(); got != before+1 {
		t.Fatalf("LineCount after Enter = %d, want %d", got, before+1)
	}
	if got := ta.Text(); got != "abc\ndef" {
		t.Fatalf("Text() = %q, want abc\\ndef", got)
	}
	// Caret should be parked at the start of the new line.
	if ta.cursorLine != 1 || ta.cursorCol != 0 {
		t.Fatalf("caret = (%d,%d), want (1,0)", ta.cursorLine, ta.cursorCol)
	}
}

// TestTextAreaArrowsClampAtBoundaries: arrow keys never carry the
// caret outside the buffer.
func TestTextAreaArrowsClampAtBoundaries(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("abc\ndef")

	// Send Left repeatedly from the end; caret must land at (0,0).
	for i := 0; i < 50; i++ {
		ta.OnKeyDown(KeyLeft, false)
	}
	if ta.cursorLine != 0 || ta.cursorCol != 0 {
		t.Fatalf("after many Left, caret = (%d,%d), want (0,0)", ta.cursorLine, ta.cursorCol)
	}
	// Send Right repeatedly; must land at the very end.
	for i := 0; i < 50; i++ {
		ta.OnKeyDown(KeyRight, false)
	}
	last := ta.LineCount() - 1
	wantCol := len(ta.lines[last])
	if ta.cursorLine != last || ta.cursorCol != wantCol {
		t.Fatalf("after many Right, caret = (%d,%d), want (%d,%d)", ta.cursorLine, ta.cursorCol, last, wantCol)
	}
	// Up at top stays at (0, *).
	ta.moveCursor(0, 1, false)
	ta.OnKeyDown(KeyUp, false)
	if ta.cursorLine != 0 {
		t.Fatalf("Up at top line moved to line %d", ta.cursorLine)
	}
	// Down at last line stays on the last line.
	ta.moveCursor(last, 0, false)
	ta.OnKeyDown(KeyDown, false)
	if ta.cursorLine != last {
		t.Fatalf("Down at last line moved to line %d", ta.cursorLine)
	}
}

// TestTextAreaBackspaceJoinsLines: Backspace at column 0 of any line
// after the first joins it with the previous line, mirroring every
// other editor's "delete the newline" behaviour.
func TestTextAreaBackspaceJoinsLines(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("first\nsecond")
	// Park the caret at (1, 0).
	ta.moveCursor(1, 0, false)
	ta.OnKeyDown(KeyBackSpace, false)
	if got := ta.Text(); got != "firstsecond" {
		t.Fatalf("Text() = %q, want firstsecond", got)
	}
	if ta.cursorLine != 0 || ta.cursorCol != 5 {
		t.Fatalf("caret = (%d,%d), want (0,5)", ta.cursorLine, ta.cursorCol)
	}
	if ta.LineCount() != 1 {
		t.Fatalf("LineCount() = %d, want 1", ta.LineCount())
	}
}

// TestTextAreaBackspaceMidLine: a plain Backspace mid-line removes
// the byte before the caret. Plain-ASCII path is enough here — multi
// -rune handling is documented as intentionally lean.
func TestTextAreaBackspaceMidLine(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("abc")
	ta.moveCursor(0, 2, false)
	ta.OnKeyDown(KeyBackSpace, false)
	if got := ta.Text(); got != "ac" {
		t.Fatalf("Text() = %q, want ac", got)
	}
	if ta.cursorCol != 1 {
		t.Fatalf("caret col = %d, want 1", ta.cursorCol)
	}
}

// TestTextAreaSetReadOnlyBlocksMutations: with read-only on, neither
// OnTextInput, KeyEnter, KeyBackSpace nor KeyDelete should mutate the
// buffer.
func TestTextAreaSetReadOnlyBlocksMutations(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("hello")
	ta.SetReadOnly(true)
	ta.moveCursor(0, 5, false)

	ta.OnTextInput("X")
	if ta.Text() != "hello" {
		t.Fatalf("read-only OnTextInput leaked: %q", ta.Text())
	}
	ta.OnKeyDown(KeyEnter, false)
	if ta.Text() != "hello" {
		t.Fatalf("read-only Enter leaked: %q", ta.Text())
	}
	ta.OnKeyDown(KeyBackSpace, false)
	if ta.Text() != "hello" {
		t.Fatalf("read-only Backspace leaked: %q", ta.Text())
	}
	ta.moveCursor(0, 0, false)
	ta.OnKeyDown(KeyDelete, false)
	if ta.Text() != "hello" {
		t.Fatalf("read-only Delete leaked: %q", ta.Text())
	}
}

// TestTextAreaSelectAllSpansBuffer: SelectAll covers the entire
// buffer regardless of where the caret started.
func TestTextAreaSelectAllSpansBuffer(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("a\nb\nc")
	ta.moveCursor(1, 1, false)
	ta.SelectAll()
	if !ta.hasSelection() {
		t.Fatal("SelectAll left no selection")
	}
	l0, c0, l1, c1 := ta.canonicalSelection()
	if l0 != 0 || c0 != 0 {
		t.Fatalf("selection start = (%d,%d), want (0,0)", l0, c0)
	}
	if l1 != 2 || c1 != 1 {
		t.Fatalf("selection end = (%d,%d), want (2,1)", l1, c1)
	}
}

// TestTextAreaPlaceholderSetGet: SetPlaceholder + Placeholder is a
// trivial round-trip — included to keep the public surface covered.
func TestTextAreaPlaceholderSetGet(t *testing.T) {
	ta := NewTextArea()
	ta.SetPlaceholder("type here")
	if got := ta.Placeholder(); got != "type here" {
		t.Fatalf("Placeholder() = %q, want type here", got)
	}
}

// TestTextAreaInsertAtCursorReplacesSelection: a selection followed
// by a typed character (or paste) replaces the selection with the new
// text, leaving the caret just past the new text.
func TestTextAreaInsertAtCursorReplacesSelection(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("abcdef")
	// Anchor at col 1, caret at col 4 — selects "bcd".
	ta.selLine, ta.selCol = 0, 1
	ta.cursorLine, ta.cursorCol = 0, 4
	ta.OnTextInput("X")
	if got := ta.Text(); got != "aXef" {
		t.Fatalf("Text() = %q, want aXef", got)
	}
	if ta.cursorLine != 0 || ta.cursorCol != 2 {
		t.Fatalf("caret = (%d,%d), want (0,2)", ta.cursorLine, ta.cursorCol)
	}
}

// TestTextAreaFactoryRegistered: the gui.TextArea factory must be
// findable so the visual designer can drag-spawn one.
func TestTextAreaFactoryRegistered(t *testing.T) {
	// Cheap smoke check: NewTextArea returns a non-nil pointer with the
	// expected interface. The factory init() registers under the name
	// "gui.TextArea"; the broader factory machinery is exercised
	// elsewhere — we just confirm the type is usable here.
	ta := NewTextArea()
	if ta == nil {
		t.Fatal("NewTextArea returned nil")
	}
	if ta.LineCount() != 1 {
		t.Fatalf("fresh TextArea has LineCount() = %d, want 1", ta.LineCount())
	}
	// Sanity: SizeHints returns positive width/height in the documented
	// 200x80 ballpark so the designer can place it.
	sh := ta.SizeHints()
	if sh.Width < 100 || sh.Height < 40 {
		t.Fatalf("SizeHints = %+v, want at least 100x40", sh)
	}
}

// TestTextAreaTextInputWithEmbeddedNewlines verifies that pasting a
// multi-line string in one OnTextInput call grows the line count
// correctly (the same code path used by KeyEnter).
func TestTextAreaTextInputWithEmbeddedNewlines(t *testing.T) {
	ta := NewTextArea()
	ta.OnTextInput("a\nb\nc")
	if ta.LineCount() != 3 {
		t.Fatalf("LineCount() = %d, want 3", ta.LineCount())
	}
	if got := ta.Text(); got != "a\nb\nc" {
		t.Fatalf("Text() = %q", got)
	}
	if ta.cursorLine != 2 || ta.cursorCol != 1 {
		t.Fatalf("caret = (%d,%d), want (2,1)", ta.cursorLine, ta.cursorCol)
	}
}

// TestTextAreaHomeEndKeysCollapseToLine verifies Home / End without
// Ctrl jump to the boundaries of the current line, leaving line
// position untouched. Ctrl variants jump to start/end of buffer.
func TestTextAreaHomeEndKeysCollapseToLine(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("one\ntwo\nthree")
	ta.moveCursor(1, 1, false)
	ta.OnKeyDown(KeyHome, false)
	if ta.cursorLine != 1 || ta.cursorCol != 0 {
		t.Fatalf("Home: caret = (%d,%d), want (1,0)", ta.cursorLine, ta.cursorCol)
	}
	ta.OnKeyDown(KeyEnd, false)
	if ta.cursorLine != 1 || ta.cursorCol != len("two") {
		t.Fatalf("End: caret = (%d,%d), want (1,3)", ta.cursorLine, ta.cursorCol)
	}
}

// TestTextAreaSelectionTextSpansLines pulls a multi-line selection
// out as a "\n"-joined string, matching what copy/cut will put on the
// clipboard.
func TestTextAreaSelectionTextSpansLines(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("alpha\nbeta\ngamma")
	ta.selLine, ta.selCol = 0, 2
	ta.cursorLine, ta.cursorCol = 2, 3
	got := ta.selectionText()
	want := "pha\nbeta\ngam"
	if got != want {
		t.Fatalf("selectionText() = %q, want %q", got, want)
	}
	// Sanity: selection text reassembles into what would land between
	// the anchor and caret on the joined text.
	if !strings.Contains(ta.Text(), got) {
		t.Fatalf("selection %q not a substring of Text() %q", got, ta.Text())
	}
}
