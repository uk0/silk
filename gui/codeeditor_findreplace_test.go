package gui

// Tests for the CodeEditor find/replace bar. Split into three layers:
//   1. findMatches         - pure match scanner (case fold, adjacency, empty query)
//   2. replaceAllInText    - pure whole-text replacer (count, drift, no-loop)
//   3. widget-level        - state/helper drive (no GL, no Draw): open find, set
//                            query, Replace (advance), Replace All (+ undo).

import (
	"testing"
)

// --- 1. findMatches (pure) ---

func TestFindMatchesMultiple(t *testing.T) {
	got := findMatches("foo bar\nfoo baz\nqux foo", "foo", false)
	if len(got) != 3 {
		t.Fatalf("want 3 matches, got %d: %v", len(got), got)
	}
	want := []findMatch{{0, 0, 3}, {1, 0, 3}, {2, 4, 7}}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("match %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestFindMatchesCaseSensitivity(t *testing.T) {
	text := "Foo foo FOO"
	if got := findMatches(text, "foo", false); len(got) != 3 {
		t.Errorf("case-insensitive: want 3, got %d: %v", len(got), got)
	}
	if got := findMatches(text, "foo", true); len(got) != 1 {
		t.Errorf("case-sensitive: want 1, got %d: %v", len(got), got)
	}
	// Case-sensitive hit must land on the lowercase occurrence (col 4).
	if got := findMatches(text, "foo", true); len(got) == 1 && got[0].col != 4 {
		t.Errorf("case-sensitive match col = %d, want 4", got[0].col)
	}
}

func TestFindMatchesNoMatch(t *testing.T) {
	if got := findMatches("hello world", "zzz", false); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestFindMatchesAdjacentNonOverlapping(t *testing.T) {
	// "aa" in "aaaa" must be two non-overlapping hits, not three overlapping.
	got := findMatches("aaaa", "aa", false)
	if len(got) != 2 {
		t.Fatalf("want 2 non-overlapping, got %d: %v", len(got), got)
	}
	if got[0] != (findMatch{0, 0, 2}) || got[1] != (findMatch{0, 2, 4}) {
		t.Errorf("adjacency wrong: %v", got)
	}
}

func TestFindMatchesEmptyQuery(t *testing.T) {
	// Empty query yields no matches, NOT one-per-position.
	if got := findMatches("abc", "", false); got != nil {
		t.Errorf("empty query must yield no matches, got %v", got)
	}
}

func TestFindMatchesColumnsAreRunes(t *testing.T) {
	// Multi-byte runes before the hit must not shift the reported rune column.
	got := findMatches("héllo foo", "foo", false)
	if len(got) != 1 {
		t.Fatalf("want 1, got %d: %v", len(got), got)
	}
	if got[0].col != 6 || got[0].end != 9 { // h é l l o _ f o o -> f at rune 6
		t.Errorf("rune columns wrong: %+v, want col 6 end 9", got[0])
	}
}

// --- 2. replaceAllInText (pure) ---

func TestReplaceAllBasic(t *testing.T) {
	out, n := replaceAllInText("foo bar foo", "foo", "X", false)
	if out != "X bar X" || n != 2 {
		t.Errorf("got %q,%d want %q,%d", out, n, "X bar X", 2)
	}
}

func TestReplaceAllAdjacent(t *testing.T) {
	out, n := replaceAllInText("aaaa", "aa", "b", false)
	if out != "bb" || n != 2 {
		t.Errorf("got %q,%d want %q,%d", out, n, "bb", 2)
	}
}

func TestReplaceAllReplacementContainsQuery(t *testing.T) {
	// "a"->"aa" must count the ORIGINAL 3 matches and must not loop.
	out, n := replaceAllInText("aaa", "a", "aa", false)
	if out != "aaaaaa" || n != 3 {
		t.Errorf("got %q,%d want %q,%d", out, n, "aaaaaa", 3)
	}
}

func TestReplaceAllEmptyQuery(t *testing.T) {
	out, n := replaceAllInText("abc", "", "X", false)
	if out != "abc" || n != 0 {
		t.Errorf("empty query: got %q,%d want %q,0", out, n, "abc")
	}
}

func TestReplaceAllNoMatch(t *testing.T) {
	out, n := replaceAllInText("xyz", "q", "r", false)
	if out != "xyz" || n != 0 {
		t.Errorf("got %q,%d want %q,0", out, n, "xyz")
	}
}

func TestReplaceAllCaseFolding(t *testing.T) {
	// Case-insensitive replaces both cases; case-sensitive only the exact one.
	if out, n := replaceAllInText("Aa", "a", "b", false); out != "bb" || n != 2 {
		t.Errorf("insensitive: got %q,%d want %q,2", out, n, "bb")
	}
	if out, n := replaceAllInText("Aa", "a", "b", true); out != "Ab" || n != 1 {
		t.Errorf("sensitive: got %q,%d want %q,1", out, n, "Ab")
	}
}

func TestReplaceAllPreservesNonMatchedCase(t *testing.T) {
	// Case-insensitive matching must not lowercase the surrounding text.
	out, n := replaceAllInText("HELLO foo World", "foo", "bar", false)
	if out != "HELLO bar World" || n != 1 {
		t.Errorf("got %q,%d want %q,1", out, n, "HELLO bar World")
	}
}

// --- 3. widget-level (no GL) ---

func TestCodeEditorFindOpenAndCount(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo bar\nfoo baz\nqux foo")
	e.findActive = true
	e.findText = "foo"
	e.findUpdateMatches()
	if len(e.findMatches) != 3 {
		t.Fatalf("findMatches=%d, want 3", len(e.findMatches))
	}
	if e.findCurrentIdx != 0 {
		t.Errorf("findCurrentIdx=%d, want 0", e.findCurrentIdx)
	}
}

func TestCodeEditorFindCaseInsensitiveByDefault(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("Foo foo FOO")
	e.findActive = true
	e.findText = "foo"
	e.findUpdateMatches()
	if len(e.findMatches) != 3 {
		t.Errorf("default find should be case-insensitive: got %d, want 3", len(e.findMatches))
	}
}

func TestCodeEditorReplaceCurrentAdvances(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo bar\nfoo baz\nqux foo")
	e.findActive = true
	e.findText = "foo"
	e.replaceText = "XYZ"
	e.findUpdateMatches()
	e.findCurrentIdx = 0

	e.replaceCurrent()

	if got := e.lines[0]; got != "XYZ bar" {
		t.Errorf("line 0 = %q, want %q", got, "XYZ bar")
	}
	// The first "foo" is gone; two remain and the current index should point at
	// the next occurrence (line 1), not re-target the replaced spot.
	if len(e.findMatches) != 2 {
		t.Fatalf("after replace, matches=%d, want 2", len(e.findMatches))
	}
	if cur := e.findMatches[e.findCurrentIdx]; cur.line != 1 {
		t.Errorf("current match line = %d, want 1 (advanced)", cur.line)
	}
}

func TestCodeEditorReplaceCurrentNoReMatchIntoReplacement(t *testing.T) {
	// Replacing "a" with "aa" must advance PAST the inserted text, not sit on
	// the freshly created "a".
	e := NewCodeEditor()
	e.SetText("a b a")
	e.findActive = true
	e.findText = "a"
	e.replaceText = "aa"
	e.findUpdateMatches()
	e.findCurrentIdx = 0

	e.replaceCurrent()

	if e.Text() != "aa b a" {
		t.Fatalf("buffer = %q, want %q", e.Text(), "aa b a")
	}
	// Current match must be the trailing "a" (col 5), skipping the new one at col 1.
	if cur := e.findMatches[e.findCurrentIdx]; cur.col != 5 {
		t.Errorf("current match col = %d, want 5 (advanced past replacement)", cur.col)
	}
}

func TestCodeEditorReplaceAll(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo foo\nfoo")
	e.findActive = true
	e.findText = "foo"
	e.replaceText = "bar"
	e.findUpdateMatches()

	n := e.replaceAll()

	if n != 3 {
		t.Errorf("replaceAll count = %d, want 3", n)
	}
	if e.Text() != "bar bar\nbar" {
		t.Errorf("buffer = %q, want %q", e.Text(), "bar bar\nbar")
	}
	if len(e.findMatches) != 0 {
		t.Errorf("after Replace All the old query should have no matches, got %d", len(e.findMatches))
	}
}

func TestCodeEditorReplaceAllUndoable(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("cat cat cat")
	before := e.Text()
	e.findActive = true
	e.findText = "cat"
	e.replaceText = "dog"
	e.findUpdateMatches()

	if n := e.replaceAll(); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
	if e.Text() != "dog dog dog" {
		t.Fatalf("after = %q, want %q", e.Text(), "dog dog dog")
	}
	e.undo()
	if e.Text() != before {
		t.Errorf("after undo = %q, want %q", e.Text(), before)
	}
}

func TestCodeEditorReplaceAllNoMatchNoChange(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("hello world")
	e.findActive = true
	e.findText = "zzz"
	e.replaceText = "x"
	e.findUpdateMatches()
	if n := e.replaceAll(); n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if e.Text() != "hello world" {
		t.Errorf("buffer changed to %q", e.Text())
	}
}

func TestCodeEditorOnTextInputRoutesToFocusedField(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc abc")
	e.findActive = true

	// Find has focus by default: typing edits findText and updates matches.
	e.OnTextInput("abc")
	if e.findText != "abc" {
		t.Fatalf("findText = %q, want %q", e.findText, "abc")
	}
	if len(e.findMatches) != 2 {
		t.Errorf("matches = %d, want 2", len(e.findMatches))
	}

	// Show + focus the replace input: typing now edits replaceText only.
	e.toggleReplaceRow()
	if !e.replaceVisible || !e.replaceFocused {
		t.Fatalf("toggleReplaceRow: visible=%v focused=%v, want both true", e.replaceVisible, e.replaceFocused)
	}
	e.OnTextInput("Z")
	if e.replaceText != "Z" {
		t.Errorf("replaceText = %q, want %q", e.replaceText, "Z")
	}
	if e.findText != "abc" {
		t.Errorf("findText changed to %q while replace focused", e.findText)
	}
}

func TestCodeEditorToggleReplaceRowResizesBar(t *testing.T) {
	e := NewCodeEditor()
	e.findActive = true
	base := e.topOffset() // one row + breadcrumb

	e.toggleReplaceRow()
	if e.findBarHeight != codeEditorFindRowHeight*2 {
		t.Errorf("findBarHeight = %v, want %v", e.findBarHeight, codeEditorFindRowHeight*2)
	}
	if got, want := e.topOffset(), base+codeEditorFindRowHeight; got != want {
		t.Errorf("topOffset grew to %v, want %v", got, want)
	}

	e.toggleReplaceRow() // back to one row
	if e.findBarHeight != codeEditorFindRowHeight {
		t.Errorf("findBarHeight = %v, want %v after collapse", e.findBarHeight, codeEditorFindRowHeight)
	}
	if e.replaceVisible {
		t.Errorf("replaceVisible should be false after second toggle")
	}
}

func TestCodeEditorCloseFindBarResets(t *testing.T) {
	e := NewCodeEditor()
	e.findActive = true
	e.toggleReplaceRow()
	e.findText = "x"
	e.findMatches = []findMatch{{0, 0, 1}}

	e.closeFindBar()

	if e.findActive || e.replaceVisible || e.replaceFocused {
		t.Errorf("close left flags set: active=%v replaceVisible=%v replaceFocused=%v",
			e.findActive, e.replaceVisible, e.replaceFocused)
	}
	if e.findMatches != nil {
		t.Errorf("close left findMatches=%v", e.findMatches)
	}
	if e.findBarHeight != codeEditorFindRowHeight {
		t.Errorf("close left findBarHeight=%v, want %v", e.findBarHeight, codeEditorFindRowHeight)
	}
}

func TestCodeEditorReplaceButtonHit(t *testing.T) {
	e := NewCodeEditor()
	e.findActive = true
	e.toggleReplaceRow() // replace row visible at y in [rowH, 2*rowH)
	rowH := float64(codeEditorFindRowHeight)
	y := rowH + rowH/2 // middle of the replace row
	btnX := findBarInputX + findBarInputW + 10

	if hit := e.replaceButtonHit(btnX+2, y); hit != 1 {
		t.Errorf("Replace button hit = %d, want 1", hit)
	}
	if hit := e.replaceButtonHit(btnX+findBarBtnW+6+2, y); hit != 2 {
		t.Errorf("Replace All button hit = %d, want 2", hit)
	}
	if hit := e.replaceButtonHit(5, y); hit != 0 {
		t.Errorf("label area hit = %d, want 0", hit)
	}
	// Nothing on the find row (y < rowH).
	if hit := e.replaceButtonHit(btnX+2, rowH/2); hit != 0 {
		t.Errorf("find-row hit = %d, want 0", hit)
	}
}
