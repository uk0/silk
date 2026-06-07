package gui

import "testing"

// Tests for the single-line paste-sanitiser, the MaxLength cap, and the
// interaction between the two on the paste path. Same convention as
// edit_keyboard_test.go: pure helpers in isolation, internal methods
// exercised directly through NewEdit() without driving a real window.

// --- sanitizePasteForSingleLine ---------------------------------------

func TestSanitizePasteForSingleLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"pure ASCII unchanged", "hello world", "hello world"},
		{"LF becomes space", "foo\nbar", "foo bar"},
		{"CRLF becomes space", "foo\r\nbar", "foo bar"},
		{"bare CR is stripped", "foo\rbar", "foobar"},
		{"leading and trailing whitespace preserved", "  pad  ", "  pad  "},
		{"CRLF + LF mix", "a\r\nb\nc\r\nd", "a b c d"},
		{"multiple LFs each become a space", "x\n\ny", "x  y"},
		{"only newlines", "\n\r\n\r", "  "},
		{"unicode passes through", "héllo\nwörld", "héllo wörld"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizePasteForSingleLine(c.in)
			if got != c.want {
				t.Errorf("sanitizePasteForSingleLine(%q) = %q, want %q",
					c.in, got, c.want)
			}
		})
	}
}

// --- MaxLength accessors ----------------------------------------------

func TestEditMaxLengthDefaultZero(t *testing.T) {
	e := NewEdit()
	if got := e.MaxLength(); got != 0 {
		t.Errorf("MaxLength() = %d, want 0 (unlimited by default)", got)
	}
}

func TestEditSetMaxLengthBelowZeroClears(t *testing.T) {
	e := NewEdit()
	e.SetMaxLength(10)
	e.SetMaxLength(-1)
	if got := e.MaxLength(); got != 0 {
		t.Errorf("MaxLength() after SetMaxLength(-1) = %d, want 0", got)
	}
}

// SetMaxLength below the current rune length truncates the buffer and
// fires the change callback (an explicit API call is a deliberate
// reset, not a stray keystroke).
func TestEditSetMaxLengthTruncatesAndFires(t *testing.T) {
	e := NewEdit()
	e.SetText("0123456789")
	fired := 0
	e.SigTextChanged(func(_ interface{}, _ string) { fired++ })
	fired = 0 // ignore the SetText-driven fire above
	e.SetMaxLength(4)
	if got := e.Text(); got != "0123" {
		t.Errorf("after SetMaxLength(4), Text() = %q, want %q", got, "0123")
	}
	if fired == 0 {
		t.Errorf("SigTextChanged should fire when SetMaxLength truncates the buffer")
	}
}

// SetMaxLength at or above the current rune length is a pure setter — no
// truncation, no callback.
func TestEditSetMaxLengthAboveLenNoTruncate(t *testing.T) {
	e := NewEdit()
	e.SetText("abc")
	fired := 0
	e.SigTextChanged(func(_ interface{}, _ string) { fired++ })
	e.SetMaxLength(10)
	if got := e.Text(); got != "abc" {
		t.Errorf("Text() after SetMaxLength(10) = %q, want %q", got, "abc")
	}
	if fired != 0 {
		t.Errorf("SigTextChanged fired %d times, want 0 (no truncation)", fired)
	}
}

// --- Typing path against MaxLength ------------------------------------

// At the cap, a single-rune keystroke is dropped silently — no buffer
// change, no callback fire.
func TestEditOnTextInputRejectsAtMaxLength(t *testing.T) {
	e := NewEdit()
	e.SetMaxLength(3)
	e.SetText("abc")
	editedFires := 0
	e.SigTextEdited(func(_ interface{}, _ string) { editedFires++ })
	e.OnTextInput("x")
	if got := e.Text(); got != "abc" {
		t.Errorf("Text() = %q, want %q (keystroke past limit must not extend)", got, "abc")
	}
	if editedFires != 0 {
		t.Errorf("SigTextEdited fired %d times, want 0 (rejected keystroke)", editedFires)
	}
}

// With a selection that would be replaced, a keystroke at the cap is
// allowed when the net growth stays within the cap.
func TestEditOnTextInputReplaceSelectionAtMaxLength(t *testing.T) {
	e := NewEdit()
	e.SetMaxLength(3)
	e.SetText("abc")
	// select "b" — replacing it with "Z" keeps the buffer at 3 runes.
	e.SetSelection(1, 2)
	e.OnTextInput("Z")
	if got := e.Text(); got != "aZc" {
		t.Errorf("Text() = %q, want %q", got, "aZc")
	}
}

// --- Paste path -------------------------------------------------------

// pasteString runs sanitisation first, then clamps to remaining
// headroom; the inserted slice loses its tail, the existing buffer is
// not touched.
func TestEditPasteStringNewlinesAndMaxLength(t *testing.T) {
	e := NewEdit()
	e.SetMaxLength(5)
	e.pasteString("foo\nbar\nbaz")
	if got := e.Text(); got != "foo b" {
		t.Errorf("Text() = %q, want %q (sanitise then truncate)", got, "foo b")
	}
}

// Paste without newlines and without a MaxLength uses the unchanged
// historical path — the payload lands verbatim.
func TestEditPasteStringPlainNoLimit(t *testing.T) {
	e := NewEdit()
	e.pasteString("hello")
	if got := e.Text(); got != "hello" {
		t.Errorf("Text() = %q, want %q", got, "hello")
	}
}

// CR/LF cleanup runs even when no MaxLength is set.
func TestEditPasteStringStripsCRReplacesLF(t *testing.T) {
	e := NewEdit()
	e.pasteString("foo\r\nbar")
	if got := e.Text(); got != "foo bar" {
		t.Errorf("Text() = %q, want %q (CR stripped, LF -> space)", got, "foo bar")
	}
}

// Leading / trailing whitespace from the clipboard is preserved — a
// user pasting " hello " into a search field meant the spaces.
func TestEditPasteStringPreservesPaddingWhitespace(t *testing.T) {
	e := NewEdit()
	e.pasteString("  pad  ")
	if got := e.Text(); got != "  pad  " {
		t.Errorf("Text() = %q, want %q (no trim)", got, "  pad  ")
	}
}

// Pasting into a buffer already at the cap is a silent no-op — the
// existing text must not lose any runes.
func TestEditPasteStringAtCapNoOp(t *testing.T) {
	e := NewEdit()
	e.SetText("abc")
	e.SetMaxLength(3)
	// caret at end
	e.SetSelection(3, 3)
	e.pasteString("XYZ")
	if got := e.Text(); got != "abc" {
		t.Errorf("Text() = %q, want %q (paste at cap is no-op)", got, "abc")
	}
}

// Pasting with a selection replaces it; the inserted slice is clamped
// to the new headroom (cap - (existing - selection_size)).
func TestEditPasteStringReplaceSelectionRespectsCap(t *testing.T) {
	e := NewEdit()
	e.SetText("abcde")
	e.SetMaxLength(5)
	// replace "bcd" (3 runes) — headroom for the paste is 3.
	e.SetSelection(1, 4)
	e.pasteString("ZZZZZ")
	if got := e.Text(); got != "aZZZe" {
		t.Errorf("Text() = %q, want %q", got, "aZZZe")
	}
}

// Multi-line Edits keep the raw payload — sanitisation is for single-
// line only. Verifies the ml=true branch.
func TestEditPasteStringMultiLineKeepsNewlines(t *testing.T) {
	e := NewEdit()
	e.SetMultiLine(true)
	e.pasteString("foo\nbar")
	if got := e.Text(); got != "foo\nbar" {
		t.Errorf("Text() = %q, want %q (multi-line keeps LF)", got, "foo\nbar")
	}
}
