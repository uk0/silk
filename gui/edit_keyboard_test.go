package gui

import "testing"

// Keyboard editing tests for the single-line Edit (Qt QLineEdit parity):
// Select-All, word-wise caret jump, word delete, and Shift-extend.
//
// NOTE on modifier state: the OnKeyDown branches that gate on Ctrl/Shift/
// Alt read IsKeyDown(...), which polls live GLFW window state and cannot
// be set from a headless test. So the tests below exercise (a) the pure
// word-boundary helpers in isolation and (b) the internal methods the key
// handler delegates to (selectAll via SelectAll, moveWordLeft/Right,
// deleteWordBefore/After, moveCaret) directly. The non-modifier OnKeyDown
// paths (plain Backspace / Left / Right / Home / End) are driven through
// OnKeyDown itself since they do not depend on a held modifier.

// --- prevWordBoundary -------------------------------------------------

func TestPrevWordBoundary(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		caret int
		want  int
	}{
		{"empty", "", 0, 0},
		{"start of text", "hello", 0, 0},
		{"middle of word jumps to word start", "hello", 3, 0},
		{"end of single word", "hello", 5, 0},
		{"caret after trailing space skips space then word", "hello ", 6, 0},
		{"second word start", "hello world", 11, 6},
		{"caret in second word", "hello world", 8, 6},
		{"caret right after a space lands on prev word start", "hello world", 6, 0},
		{"multiple spaces collapse", "foo   bar", 9, 6},
		{"punctuation cluster is its own word", "a==b", 3, 1},
		{"caret at end after punctuation", "a==b", 4, 3},
		{"leading spaces", "   abc", 6, 3},
		{"underscore is a word rune", "foo_bar baz", 7, 0},
		{"caret beyond len clamps", "hi", 99, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := prevWordBoundary([]rune(c.text), c.caret)
			if got != c.want {
				t.Errorf("prevWordBoundary(%q, %d) = %d, want %d", c.text, c.caret, got, c.want)
			}
		})
	}
}

// --- nextWordBoundary -------------------------------------------------

func TestNextWordBoundary(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		caret int
		want  int
	}{
		{"empty", "", 0, 0},
		{"end of text", "hello", 5, 5},
		{"from start skips word then space", "hello world", 0, 6},
		{"from middle of word to next word", "hello world", 2, 6},
		{"caret on space skips to next word", "hello world", 5, 6},
		{"last word has no trailing space", "hello world", 6, 11},
		{"multiple spaces collapse", "foo   bar", 0, 6},
		{"punctuation cluster", "a==b", 1, 3},
		{"word then punctuation (no space)", "a==b", 0, 1},
		{"trailing spaces go to end", "abc   ", 0, 6},
		{"underscore is a word rune", "foo_bar baz", 0, 8},
		{"caret negative clamps", "hi there", -5, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextWordBoundary([]rune(c.text), c.caret)
			if got != c.want {
				t.Errorf("nextWordBoundary(%q, %d) = %d, want %d", c.text, c.caret, got, c.want)
			}
		})
	}
}

func TestIsWordRune(t *testing.T) {
	for _, r := range []rune{'a', 'Z', '0', '9', '_', '世'} {
		if !isWordRune(r) {
			t.Errorf("isWordRune(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{' ', '\t', '.', ',', '-', '=', '\n'} {
		if isWordRune(r) {
			t.Errorf("isWordRune(%q) = true, want false", r)
		}
	}
}

// --- SelectAll (delegate of Ctrl+A) -----------------------------------

func TestEditSelectAll(t *testing.T) {
	e := NewEdit()
	e.SetText("hello world")
	e.SelectAll()
	a, b := e.Selection()
	if a != 0 || b != 11 {
		t.Errorf("SelectAll selection = (%d,%d), want (0,11)", a, b)
	}
	if e.SelectionText() != "hello world" {
		t.Errorf("SelectAll text = %q, want %q", e.SelectionText(), "hello world")
	}
}

func TestEditSelectAllEmpty(t *testing.T) {
	e := NewEdit()
	e.SetText("")
	e.SelectAll()
	a, b := e.Selection()
	if a != 0 || b != 0 {
		t.Errorf("SelectAll on empty = (%d,%d), want (0,0)", a, b)
	}
}

// --- Word-jump (delegate of Ctrl/Alt+Left/Right) ----------------------

func TestEditMoveWordLeftLandsOnBoundary(t *testing.T) {
	e := NewEdit()
	e.SetText("hello world")
	e.SetSelection(11, 11) // caret at end
	e.moveWordLeft(false)
	if e.CaretPos() != 6 {
		t.Errorf("moveWordLeft from 11: caret = %d, want 6", e.CaretPos())
	}
	e.moveWordLeft(false)
	if e.CaretPos() != 0 {
		t.Errorf("moveWordLeft from 6: caret = %d, want 0", e.CaretPos())
	}
	a, b := e.Selection()
	if a != 0 || b != 0 {
		t.Errorf("non-extend move should collapse selection, got (%d,%d)", a, b)
	}
}

func TestEditMoveWordRightLandsOnBoundary(t *testing.T) {
	e := NewEdit()
	e.SetText("hello world")
	e.SetSelection(0, 0) // caret at start
	e.moveWordRight(false)
	if e.CaretPos() != 6 {
		t.Errorf("moveWordRight from 0: caret = %d, want 6", e.CaretPos())
	}
	e.moveWordRight(false)
	if e.CaretPos() != 11 {
		t.Errorf("moveWordRight from 6: caret = %d, want 11", e.CaretPos())
	}
}

func TestEditMoveWordExtendsSelection(t *testing.T) {
	e := NewEdit()
	e.SetText("hello world")
	e.SetSelection(11, 11)
	e.moveWordLeft(true) // Shift+Ctrl+Left: extend to word start
	a, b := e.Selection()
	if a != 6 || b != 11 {
		t.Errorf("extend moveWordLeft selection = (%d,%d), want (6,11)", a, b)
	}
	if e.SelectionText() != "world" {
		t.Errorf("extend selection text = %q, want %q", e.SelectionText(), "world")
	}
}

// --- moveCaret extend (delegate of Shift+Left/Right/Home/End) ----------

func TestEditMoveCaretExtendAndCollapse(t *testing.T) {
	e := NewEdit()
	e.SetText("abcdef")
	e.SetSelection(3, 3)
	e.moveCaret(5, true) // Shift+Right twice
	a, b := e.Selection()
	if a != 3 || b != 5 {
		t.Errorf("extend caret selection = (%d,%d), want (3,5)", a, b)
	}
	e.moveCaret(0, false) // plain move collapses
	a, b = e.Selection()
	if a != 0 || b != 0 {
		t.Errorf("collapse caret selection = (%d,%d), want (0,0)", a, b)
	}
}

func TestEditMoveCaretClamps(t *testing.T) {
	e := NewEdit()
	e.SetText("abc")
	e.SetSelection(1, 1)
	e.moveCaret(99, false)
	if e.CaretPos() != 3 {
		t.Errorf("moveCaret clamp high: caret = %d, want 3", e.CaretPos())
	}
	e.moveCaret(-5, false)
	if e.CaretPos() != 0 {
		t.Errorf("moveCaret clamp low: caret = %d, want 0", e.CaretPos())
	}
}

// --- Word-delete (delegate of Ctrl+Backspace / Ctrl+Delete) -----------

func TestEditDeleteWordBefore(t *testing.T) {
	e := NewEdit()
	var fired string
	e.SigTextChanged(func(_ interface{}, s string) { fired = s })
	e.SetText("hello world")
	e.SetSelection(11, 11) // caret at end
	e.deleteWordBefore()
	if e.Text() != "hello " {
		t.Errorf("deleteWordBefore text = %q, want %q", e.Text(), "hello ")
	}
	if fired != "hello " {
		t.Errorf("deleteWordBefore callback = %q, want %q", fired, "hello ")
	}
	if e.CaretPos() != 6 {
		t.Errorf("deleteWordBefore caret = %d, want 6", e.CaretPos())
	}
}

func TestEditDeleteWordBeforeAtStartNoop(t *testing.T) {
	e := NewEdit()
	e.SetText("hello")
	e.SetSelection(0, 0)
	e.deleteWordBefore()
	if e.Text() != "hello" {
		t.Errorf("deleteWordBefore at start changed text to %q, want unchanged", e.Text())
	}
}

func TestEditDeleteWordAfter(t *testing.T) {
	e := NewEdit()
	var fired string
	e.SigTextChanged(func(_ interface{}, s string) { fired = s })
	e.SetText("hello world")
	e.SetSelection(0, 0) // caret at start
	e.deleteWordAfter()
	if e.Text() != "world" {
		t.Errorf("deleteWordAfter text = %q, want %q", e.Text(), "world")
	}
	if fired != "world" {
		t.Errorf("deleteWordAfter callback = %q, want %q", fired, "world")
	}
	if e.CaretPos() != 0 {
		t.Errorf("deleteWordAfter caret = %d, want 0", e.CaretPos())
	}
}

func TestEditDeleteWordReadOnlyNoop(t *testing.T) {
	e := NewEdit()
	e.SetText("hello world")
	e.SetReadOnly(true)
	e.SetSelection(11, 11)
	e.deleteWordBefore()
	e.SetSelection(0, 0)
	e.deleteWordAfter()
	if e.Text() != "hello world" {
		t.Errorf("read-only word delete changed text to %q, want unchanged", e.Text())
	}
}

// --- Non-modifier OnKeyDown paths (no held modifier needed) ------------

func TestEditOnKeyDownPlainBackspace(t *testing.T) {
	e := NewEdit()
	e.SetText("abc")
	e.SetSelection(3, 3)
	e.OnKeyDown(KeyBackSpace, false)
	if e.Text() != "ab" {
		t.Errorf("plain Backspace text = %q, want %q", e.Text(), "ab")
	}
}

func TestEditOnKeyDownBackspaceReplacesSelection(t *testing.T) {
	e := NewEdit()
	e.SetText("hello")
	e.SetSelection(1, 4) // select "ell"
	e.OnKeyDown(KeyBackSpace, false)
	if e.Text() != "ho" {
		t.Errorf("Backspace with selection text = %q, want %q", e.Text(), "ho")
	}
}

func TestEditOnTextInputReplacesSelection(t *testing.T) {
	e := NewEdit()
	e.SetText("hello")
	e.SetSelection(0, 5) // whole text selected
	e.OnTextInput("Z")
	if e.Text() != "Z" {
		t.Errorf("typing over selection text = %q, want %q", e.Text(), "Z")
	}
}

func TestEditOnKeyDownPlainArrowsAndHomeEnd(t *testing.T) {
	e := NewEdit()
	e.SetText("hello")
	e.SetSelection(5, 5)
	e.OnKeyDown(KeyLeft, false)
	if e.CaretPos() != 4 {
		t.Errorf("plain Left: caret = %d, want 4", e.CaretPos())
	}
	e.OnKeyDown(KeyRight, false)
	if e.CaretPos() != 5 {
		t.Errorf("plain Right: caret = %d, want 5", e.CaretPos())
	}
	e.OnKeyDown(KeyHome, false)
	if e.CaretPos() != 0 {
		t.Errorf("Home: caret = %d, want 0", e.CaretPos())
	}
	e.OnKeyDown(KeyEnd, false)
	if e.CaretPos() != 5 {
		t.Errorf("End: caret = %d, want 5", e.CaretPos())
	}
}
