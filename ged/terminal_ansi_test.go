package ged

// Tests for the terminal state machine only. AnsiTerm is pure: no PTY, no
// shell, no window — nothing here spawns a process or touches a font.

import "testing"

// feed writes s into the terminal, failing the test if the byte count is off.
func feed(t *testing.T, term *AnsiTerm, s string) {
	t.Helper()
	n, err := term.Write([]byte(s))
	if err != nil || n != len(s) {
		t.Fatalf("Write(%q) = (%d, %v), want (%d, nil)", s, n, err, len(s))
	}
}

// wantLines checks the whole screen, trailing blanks trimmed per row.
func wantLines(t *testing.T, term *AnsiTerm, want ...string) {
	t.Helper()
	if len(want) != term.Rows() {
		t.Fatalf("test wants %d rows, terminal has %d", len(want), term.Rows())
	}
	for r, exp := range want {
		if got := term.LineString(r); got != exp {
			t.Errorf("row %d = %q, want %q", r, got, exp)
		}
	}
}

func wantCursor(t *testing.T, term *AnsiTerm, row, col int) {
	t.Helper()
	if r, c := term.CursorPos(); r != row || c != col {
		t.Errorf("CursorPos() = (%d, %d), want (%d, %d)", r, c, row, col)
	}
}

func TestAnsiTermNewIsBlank(t *testing.T) {
	term := NewAnsiTerm(3, 4)
	if term.Rows() != 3 || term.Cols() != 4 {
		t.Fatalf("size = %dx%d, want 3x4", term.Rows(), term.Cols())
	}
	for _, row := range term.Screen() {
		for _, c := range row {
			if c.Rune != ' ' || c.Fg != AnsiDefaultColor || c.Bg != AnsiDefaultColor {
				t.Fatalf("blank cell = %+v, want space with default colors", c)
			}
		}
	}
	wantCursor(t, term, 0, 0)
	if s := term.LineString(-1); s != "" {
		t.Errorf("LineString(-1) = %q, want empty", s)
	}
	if s := term.LineString(3); s != "" {
		t.Errorf("LineString(3) = %q, want empty", s)
	}
}

func TestAnsiTermSGRColorsAndAttributes(t *testing.T) {
	term := NewAnsiTerm(2, 12)
	feed(t, term, "\x1b[1;31mA\x1b[0mB")
	feed(t, term, "\x1b[38;5;196mC")
	feed(t, term, "\x1b[48;2;10;20;30mD")
	feed(t, term, "\x1b[0;93mE")
	feed(t, term, "\x1b[3;4;7mF\x1b[23;24;27mG")

	row := term.Screen()[0]
	if row[0].Rune != 'A' || row[0].Fg != 1 || !row[0].Bold {
		t.Errorf("cell A = %+v, want red bold A", row[0])
	}
	if row[1].Rune != 'B' || row[1].Fg != AnsiDefaultColor || row[1].Bold {
		t.Errorf("cell B = %+v, want default non-bold B", row[1])
	}
	if row[2].Fg != 196 {
		t.Errorf("cell C fg = %d, want 196", row[2].Fg)
	}
	wantBg := AnsiRGBBase + 10<<16 + 20<<8 + 30
	if row[3].Bg != wantBg {
		t.Errorf("cell D bg = %d, want %d", row[3].Bg, wantBg)
	}
	if r, g, b, ok := AnsiColorRGB(row[3].Bg); !ok || r != 10 || g != 20 || b != 30 {
		t.Errorf("AnsiColorRGB(cell D bg) = (%d,%d,%d,%v), want (10,20,30,true)", r, g, b, ok)
	}
	// SGR 0 in the same sequence must clear the 24-bit background again.
	if row[4].Fg != 11 || row[4].Bg != AnsiDefaultColor {
		t.Errorf("cell E = %+v, want bright fg 11 on default bg", row[4])
	}
	if !row[5].Italic || !row[5].Underline || !row[5].Reverse {
		t.Errorf("cell F = %+v, want italic+underline+reverse", row[5])
	}
	if row[6].Italic || row[6].Underline || row[6].Reverse {
		t.Errorf("cell G = %+v, want attributes cleared", row[6])
	}
}

func TestAnsiColorRGBEncodings(t *testing.T) {
	if _, _, _, ok := AnsiColorRGB(AnsiDefaultColor); ok {
		t.Error("AnsiColorRGB(default) reported a color; want ok=false")
	}
	if r, g, b, ok := AnsiColorRGB(1); !ok || r != 205 || g != 49 || b != 49 {
		t.Errorf("palette 1 = (%d,%d,%d,%v), want (205,49,49,true)", r, g, b, ok)
	}
	if r, g, b, ok := AnsiColorRGB(196); !ok || r != 255 || g != 0 || b != 0 {
		t.Errorf("palette 196 = (%d,%d,%d,%v), want (255,0,0,true)", r, g, b, ok)
	}
	if r, g, b, ok := AnsiColorRGB(232); !ok || r != 8 || g != 8 || b != 8 {
		t.Errorf("palette 232 = (%d,%d,%d,%v), want (8,8,8,true)", r, g, b, ok)
	}
}

func TestAnsiTermCUPAbsolutePositioning(t *testing.T) {
	term := NewAnsiTerm(5, 10)
	feed(t, term, "\x1b[3;5HX")
	wantCursor(t, term, 2, 5)
	if got := term.LineString(2); got != "    X" {
		t.Errorf("row 2 = %q, want %q", got, "    X")
	}
	// Missing parameters default to 1 (home), out-of-range values clamp.
	feed(t, term, "\x1b[HY")
	if got := term.LineString(0); got != "Y" {
		t.Errorf("row 0 = %q, want %q", got, "Y")
	}
	feed(t, term, "\x1b[99;99HZ")
	if got := term.LineString(4); got != "         Z" {
		t.Errorf("row 4 = %q, want Z in the last column", got)
	}
	// Relative moves are clamped too, and CUB/CUF stay on the row.
	feed(t, term, "\x1b[5;5H\x1b[2A\x1b[3D")
	wantCursor(t, term, 2, 1)
	feed(t, term, "\x1b[99C")
	wantCursor(t, term, 2, 9)
}

func TestAnsiTermEraseLine(t *testing.T) {
	cases := []struct {
		name string
		seq  string
		want string
	}{
		{"EL0 cursor to end", "\x1b[0K", "ab"},
		{"EL default is 0", "\x1b[K", "ab"},
		{"EL1 start to cursor", "\x1b[1K", "   def"},
		{"EL2 whole line", "\x1b[2K", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term := NewAnsiTerm(2, 10)
			feed(t, term, "abcdef\x1b[1;3H"+tc.seq)
			if got := term.LineString(0); got != tc.want {
				t.Errorf("row 0 = %q, want %q", got, tc.want)
			}
			wantCursor(t, term, 0, 2) // erase never moves the cursor
		})
	}
}

func TestAnsiTermEraseDisplay(t *testing.T) {
	fill := "aaa\r\nbbb\r\nccc"

	term := NewAnsiTerm(3, 5)
	feed(t, term, fill+"\x1b[2J")
	wantLines(t, term, "", "", "")
	wantCursor(t, term, 2, 3)

	term = NewAnsiTerm(3, 5)
	feed(t, term, fill+"\x1b[2;2H\x1b[0J")
	wantLines(t, term, "aaa", "b", "")

	term = NewAnsiTerm(3, 5)
	feed(t, term, fill+"\x1b[2;2H\x1b[1J")
	wantLines(t, term, "", "  b", "ccc")
}

func TestAnsiTermWrapAtRightEdge(t *testing.T) {
	term := NewAnsiTerm(3, 5)
	feed(t, term, "abcdef")
	wantLines(t, term, "abcde", "f", "")
	wantCursor(t, term, 1, 1)

	// The wrap is deferred: filling the last column leaves the cursor there,
	// so a carriage return still lands on the same row.
	term = NewAnsiTerm(3, 5)
	feed(t, term, "abcde")
	wantCursor(t, term, 0, 4)
	feed(t, term, "\rZ")
	wantLines(t, term, "Zbcde", "", "")

	// Wrapping on the last row scrolls the screen.
	term = NewAnsiTerm(2, 3)
	feed(t, term, "123456789")
	wantLines(t, term, "456", "789")
}

func TestAnsiTermBackspace(t *testing.T) {
	term := NewAnsiTerm(2, 8)
	feed(t, term, "ab\bC")
	wantLines(t, term, "aC", "")
	wantCursor(t, term, 0, 2)

	// Backspace at column 0 stays put and does not walk onto the row above.
	term = NewAnsiTerm(2, 8)
	feed(t, term, "\b\bx")
	wantLines(t, term, "x", "")
}

func TestAnsiTermCarriageReturnOverwrite(t *testing.T) {
	term := NewAnsiTerm(2, 8)
	feed(t, term, "hello\rH")
	wantLines(t, term, "Hello", "")
	wantCursor(t, term, 0, 1)

	// A bare CR does not advance the row: the next write overwrites in place,
	// which is how progress output redraws itself.
	feed(t, term, "\r12345678\rdone")
	wantLines(t, term, "done5678", "")
}

func TestAnsiTermLineFeedAndTab(t *testing.T) {
	term := NewAnsiTerm(3, 20)
	feed(t, term, "\tX")
	if got := term.LineString(0); got != "        X" {
		t.Errorf("row 0 = %q, want a tab stop at column 8", got)
	}
	// LF keeps the column (no implicit carriage return).
	feed(t, term, "\nY")
	if got := term.LineString(1); got != "         Y" {
		t.Errorf("row 1 = %q, want Y at column 9", got)
	}
	// LF on the last row scrolls the whole screen up.
	feed(t, term, "\x1b[3;1Hbottom\n")
	wantLines(t, term, "         Y", "bottom", "")
}

func TestAnsiTermScrollRegion(t *testing.T) {
	term := NewAnsiTerm(5, 5)
	feed(t, term, "1\r\n2\r\n3\r\n4\r\n5")
	wantLines(t, term, "1", "2", "3", "4", "5")

	// Rows 2..4 become the scroll region; DECSTBM homes the cursor into it.
	feed(t, term, "\x1b[2;4r")
	wantCursor(t, term, 1, 0)

	// A line feed on the last region row scrolls only rows 2..4.
	feed(t, term, "\x1b[4;1H\nX")
	wantLines(t, term, "1", "3", "4", "X", "5")

	// Reverse index at the top of the region scrolls it back down.
	feed(t, term, "\x1b[2;1H\x1bMtop")
	wantLines(t, term, "1", "top", "3", "4", "5")

	// A region reset restores full-screen scrolling.
	feed(t, term, "\x1b[r")
	wantCursor(t, term, 0, 0)
	feed(t, term, "\x1b[5;1H\n")
	wantLines(t, term, "top", "3", "4", "5", "")
}

func TestAnsiTermInsertDeleteAndEraseChars(t *testing.T) {
	term := NewAnsiTerm(2, 8)
	feed(t, term, "abcdef\x1b[1;3H\x1b[2P") // DCH: drop "cd"
	wantLines(t, term, "abef", "")

	feed(t, term, "\x1b[1;3H\x1b[2@") // ICH: reopen two cells
	wantLines(t, term, "ab  ef", "")

	feed(t, term, "\x1b[1;1H\x1b[3X") // ECH: blank in place, no shifting
	wantLines(t, term, "    ef", "")

	// IL / DL move whole lines inside the scroll region.
	term = NewAnsiTerm(3, 8)
	feed(t, term, "one\r\ntwo\r\nsix\x1b[2;1H\x1b[L")
	wantLines(t, term, "one", "", "two")
	feed(t, term, "\x1b[2;1H\x1b[M")
	wantLines(t, term, "one", "two", "")
}

func TestAnsiTermResizePreservesContent(t *testing.T) {
	term := NewAnsiTerm(4, 8)
	feed(t, term, "one\r\ntwo\r\nthree")
	wantCursor(t, term, 2, 5)

	// Growing keeps every row where it was and adds blanks below.
	term.Resize(6, 8)
	wantLines(t, term, "one", "two", "three", "", "", "")
	wantCursor(t, term, 2, 5)

	// Shrinking drops leading rows so the cursor row survives.
	term.Resize(2, 8)
	wantLines(t, term, "two", "three")
	wantCursor(t, term, 1, 5)

	// Narrowing truncates on the right and clamps the cursor.
	term.Resize(2, 4)
	wantLines(t, term, "two", "thre")
	wantCursor(t, term, 1, 3)

	// The grid stays usable after the resize.
	feed(t, term, "\x1b[1;1HZ")
	wantLines(t, term, "Zwo", "thre")

	// Degenerate sizes are raised to 1 instead of producing an empty grid.
	term.Resize(0, 0)
	if term.Rows() != 1 || term.Cols() != 1 {
		t.Errorf("Resize(0,0) = %dx%d, want 1x1", term.Rows(), term.Cols())
	}
}

func TestAnsiTermUnknownEscapeIgnored(t *testing.T) {
	term := NewAnsiTerm(2, 8)
	feed(t, term, "A")
	feed(t, term, "\x1b[42;42;42q")     // unhandled CSI final byte
	feed(t, term, "\x1b]0;a title\x07") // OSC title: payload dropped
	feed(t, term, "\x1b]2;another\x1b\\")
	feed(t, term, "\x1b[?25l")  // private DEC mode
	feed(t, term, "\x1b[>4;2m") // xterm private SGR-lookalike
	feed(t, term, "\x1bZ")      // unknown ESC final
	feed(t, term, "\x1b(B")     // charset selection
	feed(t, term, "\x1b_apc\x1b\\")
	feed(t, term, "B")

	wantLines(t, term, "AB", "")
	wantCursor(t, term, 0, 2)
	if c := term.Screen()[0][1]; c.Fg != AnsiDefaultColor || c.Bold || c.Reverse {
		t.Errorf("cell B = %+v, want untouched default attributes", c)
	}
}

func TestAnsiTermSequenceSplitAcrossWrites(t *testing.T) {
	term := NewAnsiTerm(2, 8)
	// A PTY read can end anywhere, including mid-escape and mid-rune.
	feed(t, term, "\x1b[")
	feed(t, term, "32")
	feed(t, term, "mgo")
	if c := term.Screen()[0][0]; c.Rune != 'g' || c.Fg != 2 {
		t.Errorf("cell = %+v, want green g", c)
	}

	wide := []byte("中")
	if _, err := term.Write(wide[:2]); err != nil {
		t.Fatalf("Write(partial rune): %v", err)
	}
	if _, err := term.Write(wide[2:]); err != nil {
		t.Fatalf("Write(rest of rune): %v", err)
	}
	if c := term.Screen()[0][2]; c.Rune != '中' {
		t.Errorf("cell = %q, want 中", c.Rune)
	}
}

func TestAnsiTermResetAndSaveRestoreCursor(t *testing.T) {
	term := NewAnsiTerm(3, 6)
	feed(t, term, "\x1b[2;3H\x1b7\x1b[1;1Hx\x1b8y")
	if got := term.LineString(1); got != "  y" {
		t.Errorf("row 1 = %q, want the restored cursor at column 2", got)
	}
	feed(t, term, "\x1b[31m\x1bc")
	wantLines(t, term, "", "", "")
	wantCursor(t, term, 0, 0)
	feed(t, term, "z")
	if c := term.Screen()[0][0]; c.Fg != AnsiDefaultColor {
		t.Errorf("cell after RIS = %+v, want default attributes", c)
	}
}
