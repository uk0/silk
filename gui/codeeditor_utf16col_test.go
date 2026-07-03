package gui

import "testing"

// ---------------------------------------------------------------------------
// UTF-16 column accessor (pure, no GL / no Draw)
//
// LSP's Position.character is a UTF-16 code-unit offset, but the editor tracks
// columns as rune indices. CursorUTF16Col / utf16ColumnOf bridge the two: they
// agree for ASCII / BMP text and diverge only when a non-BMP rune (e.g. an
// emoji) precedes the caret, where that rune counts as two UTF-16 units.
// ---------------------------------------------------------------------------

func TestUTF16ColumnOf(t *testing.T) {
	cases := []struct {
		line    string
		runeCol int
		want    int
	}{
		{"hello", 3, 3},  // ASCII: rune index == UTF-16 offset
		{"café", 4, 4},   // é is BMP -> 1 code unit
		{"x😀y", 2, 3},    // "x😀" = 1 + 2 UTF-16 units
		{"x😀y", 1, 1},    // only "x" precedes the caret
		{"", 0, 0},       // empty line
		{"x😀y", 10, 4},   // runeCol past end clamps to full width (1+2+1)
	}
	for _, c := range cases {
		if got := utf16ColumnOf(c.line, c.runeCol); got != c.want {
			t.Errorf("utf16ColumnOf(%q, %d) = %d, want %d", c.line, c.runeCol, got, c.want)
		}
	}
}

func TestCodeEditorCursorUTF16Col(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x😀y\nab")

	// Caret after the emoji line's last rune: rune col 3, UTF-16 col 4.
	e.cursorLine = 0
	e.cursorCol = 3
	if got := e.CursorUTF16Col(); got != 4 {
		t.Errorf("CursorUTF16Col() on %q = %d, want 4 (x=1, emoji=2, y=1)", "x😀y", got)
	}
	if e.CursorCol() != 3 {
		t.Errorf("CursorCol() = %d, want 3 (rune index must stay unchanged)", e.CursorCol())
	}

	// Pure-ASCII line: UTF-16 col equals the rune col.
	e.cursorLine = 1
	e.cursorCol = 2
	if got := e.CursorUTF16Col(); got != 2 {
		t.Errorf("CursorUTF16Col() on ASCII line = %d, want 2", got)
	}
	if e.CursorUTF16Col() != e.CursorCol() {
		t.Errorf("CursorUTF16Col()=%d != CursorCol()=%d on ASCII line", e.CursorUTF16Col(), e.CursorCol())
	}
}
