package ged

// Terminal screen model: a pure ANSI / VT state machine.
//
// TerminalPanel feeds raw pseudo-terminal bytes into AnsiTerm.Write and
// renders the resulting cell grid. Nothing here touches the GUI or the
// operating system, so the whole escape-sequence surface is unit testable
// without a window.

import (
	"strings"
	"unicode/utf8"
)

const (
	// AnsiDefaultColor marks a cell drawn with the terminal's default
	// foreground / background color.
	AnsiDefaultColor = -1

	// AnsiRGBBase is the offset applied to 24-bit colors coming from
	// SGR 38;2 / 48;2. A cell color at or above it packs r<<16|g<<8|b;
	// a value in 0..255 is an xterm palette index.
	AnsiRGBBase = 1 << 24

	// ansiTabWidth is the fixed tab stop distance (VT default).
	ansiTabWidth = 8

	// ansiMaxParam bounds a single CSI numeric parameter so a hostile or
	// corrupt stream cannot overflow the accumulator.
	ansiMaxParam = 1 << 16
)

// Cell is one character cell of the terminal screen. Colors follow the
// AnsiDefaultColor / palette-index / AnsiRGBBase encoding described above;
// use AnsiColorRGB to resolve one to concrete channels. Cells are always
// produced by AnsiTerm (a zero Cell would mean palette black on black).
type Cell struct {
	Rune      rune
	Fg        int
	Bg        int
	Bold      bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// ansiState is the escape-sequence parser state.
type ansiState uint8

const (
	ansiGround    ansiState = iota // printable text and C0 controls
	ansiEscape                     // ESC seen
	ansiCSI                        // ESC [ — collecting parameters
	ansiString                     // OSC / DCS / APC / PM — swallow to ST
	ansiStringEsc                  // ESC seen inside a string sequence
	ansiSkipOne                    // ESC ( ) * + # % — one byte to drop
)

// AnsiTerm is a rows x cols terminal screen driven by a byte stream. It
// implements the subset a shell session needs: SGR attributes, relative and
// absolute cursor motion, line/screen erase, a scroll region, insert/delete
// of lines and characters, and the C0 controls. Unsupported sequences are
// swallowed without disturbing the screen.
//
// AnsiTerm is not safe for concurrent use; TerminalPanel feeds it from the
// UI thread only.
type AnsiTerm struct {
	rows, cols int
	cells      [][]Cell

	cx, cy         int // cursor, 0-based
	saveCx, saveCy int
	wrapNext       bool // cursor parked past the right edge (deferred wrap)

	top, bot int // scroll region, 0-based inclusive

	attr Cell // current SGR attributes; Rune unused

	state   ansiState
	params  []int
	private byte // CSI private marker ('?', '<', '=', '>')
	utf8Buf []byte
}

// NewAnsiTerm returns a cleared rows x cols screen. Non-positive dimensions
// are raised to 1 so the cursor always addresses a real cell.
func NewAnsiTerm(rows, cols int) *AnsiTerm {
	t := new(AnsiTerm)
	t.rows, t.cols = ansiClampMin(rows, 1), ansiClampMin(cols, 1)
	t.cells = make([][]Cell, t.rows)
	for r := range t.cells {
		t.cells[r] = make([]Cell, t.cols)
	}
	t.Reset()
	return t
}

// Rows returns the screen height in cells.
func (t *AnsiTerm) Rows() int { return t.rows }

// Cols returns the screen width in cells.
func (t *AnsiTerm) Cols() int { return t.cols }

// Screen returns the live cell grid, row-major. The slices are owned by the
// terminal and are only valid until the next Write or Resize: read them,
// never retain or mutate them.
func (t *AnsiTerm) Screen() [][]Cell { return t.cells }

// CursorPos returns the 0-based cursor row and column. A cursor parked past
// the right edge by a deferred wrap reports the last column.
func (t *AnsiTerm) CursorPos() (row, col int) { return t.cy, t.cx }

// LineString returns row's text with trailing blanks removed, or "" when row
// is out of range.
func (t *AnsiTerm) LineString(row int) string {
	if row < 0 || row >= t.rows {
		return ""
	}
	var b strings.Builder
	for _, c := range t.cells[row] {
		if c.Rune == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(c.Rune)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Reset restores the power-on state: cleared screen, home cursor, default
// attributes, full-screen scroll region, ground parser state.
func (t *AnsiTerm) Reset() {
	t.attr = Cell{Rune: ' ', Fg: AnsiDefaultColor, Bg: AnsiDefaultColor}
	t.cx, t.cy = 0, 0
	t.saveCx, t.saveCy = 0, 0
	t.wrapNext = false
	t.top, t.bot = 0, t.rows-1
	t.state = ansiGround
	t.params = t.params[:0]
	t.private = 0
	t.utf8Buf = t.utf8Buf[:0]
	for r := range t.cells {
		t.clearRow(r)
	}
}

// Write feeds bytes into the state machine. It never fails and never blocks,
// so AnsiTerm satisfies io.Writer for a PTY reader.
func (t *AnsiTerm) Write(p []byte) (int, error) {
	for _, b := range p {
		t.feed(b)
	}
	return len(p), nil
}

// Resize changes the grid to rows x cols, keeping as much content as makes
// sense: rows are copied from the top unless that would push the cursor line
// off the bottom, and columns are truncated on the right. The scroll region is
// kept when it spanned the whole screen and clamped otherwise; the cursor is
// clamped into the new bounds.
func (t *AnsiTerm) Resize(rows, cols int) {
	rows, cols = ansiClampMin(rows, 1), ansiClampMin(cols, 1)
	if rows == t.rows && cols == t.cols {
		return
	}
	full := t.top == 0 && t.bot == t.rows-1

	next := make([][]Cell, rows)
	for r := range next {
		next[r] = make([]Cell, cols)
		for c := range next[r] {
			next[r][c] = t.blank()
		}
	}
	// Shift the copy window down only as far as it takes to keep the cursor
	// row visible, so shrinking drops leading lines the way a real terminal
	// does instead of cutting off the live prompt.
	maxOff := t.rows - rows
	if maxOff < 0 {
		maxOff = 0
	}
	srcOff := ansiClamp(t.cy-(rows-1), 0, maxOff)
	copyCols := cols
	if t.cols < copyCols {
		copyCols = t.cols
	}
	for r := 0; r+srcOff < t.rows && r < rows; r++ {
		copy(next[r][:copyCols], t.cells[r+srcOff][:copyCols])
	}

	t.cells = next
	t.rows, t.cols = rows, cols
	t.cy -= srcOff
	t.cx = ansiClamp(t.cx, 0, cols-1)
	t.cy = ansiClamp(t.cy, 0, rows-1)
	t.wrapNext = false
	if full {
		t.top, t.bot = 0, rows-1
	} else {
		t.top = ansiClamp(t.top, 0, rows-1)
		t.bot = ansiClamp(t.bot, t.top, rows-1)
	}
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

func (t *AnsiTerm) feed(b byte) {
	switch t.state {
	case ansiGround:
		t.ground(b)
	case ansiEscape:
		t.escape(b)
	case ansiCSI:
		t.csi(b)
	case ansiString:
		// OSC / DCS / APC payloads (window title, clipboard, …) carry no
		// screen state we model: swallow to BEL or ST.
		switch b {
		case 0x07:
			t.state = ansiGround
		case 0x1b:
			t.state = ansiStringEsc
		}
	case ansiStringEsc:
		// ESC \ ends the string; anything else was part of the payload.
		if b == '\\' {
			t.state = ansiGround
		} else {
			t.state = ansiString
		}
	case ansiSkipOne:
		t.state = ansiGround
	}
}

func (t *AnsiTerm) ground(b byte) {
	switch {
	case b == 0x1b:
		t.utf8Buf = t.utf8Buf[:0]
		t.state = ansiEscape
	case b == '\n', b == 0x0b, b == 0x0c: // LF, VT, FF
		t.lineFeed()
	case b == '\r':
		t.cx = 0
		t.wrapNext = false
	case b == '\b':
		t.wrapNext = false
		if t.cx > 0 {
			t.cx--
		}
	case b == '\t':
		t.tab()
	case b < 0x20, b == 0x7f: // remaining C0 controls and DEL: no screen effect
	default:
		t.put(b)
	}
}

func (t *AnsiTerm) escape(b byte) {
	t.state = ansiGround
	switch b {
	case '[':
		t.params = t.params[:0]
		t.private = 0
		t.state = ansiCSI
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC
		t.state = ansiString
	case 'D': // IND
		t.lineFeed()
	case 'E': // NEL
		t.cx = 0
		t.lineFeed()
	case 'M': // RI
		t.reverseIndex()
	case '7': // DECSC
		t.saveCx, t.saveCy = t.cx, t.cy
	case '8': // DECRC
		t.cx = ansiClamp(t.saveCx, 0, t.cols-1)
		t.cy = ansiClamp(t.saveCy, 0, t.rows-1)
		t.wrapNext = false
	case 'c': // RIS
		t.Reset()
	case '(', ')', '*', '+', '#', '%', ' ': // charset / DEC selectors
		t.state = ansiSkipOne
	}
	// Everything else (keypad modes, unknown finals) is ignored.
}

func (t *AnsiTerm) csi(b byte) {
	switch {
	case b >= '0' && b <= '9':
		if len(t.params) == 0 {
			t.params = append(t.params, 0)
		}
		if v := t.params[len(t.params)-1]; v < ansiMaxParam {
			t.params[len(t.params)-1] = v*10 + int(b-'0')
		}
	case b == ';' || b == ':':
		if len(t.params) == 0 {
			t.params = append(t.params, 0)
		}
		t.params = append(t.params, 0)
	case b >= '<' && b <= '?':
		t.private = b
	case b >= 0x20 && b <= 0x2f: // intermediates: no sequence we model uses them
	case b >= 0x40 && b <= 0x7e:
		t.execCSI(b)
		t.state = ansiGround
	case b == 0x1b:
		t.state = ansiEscape
	case b < 0x20: // embedded C0 control
		t.ground(b)
	default:
		t.state = ansiGround
	}
}

// param returns parameter i, mapping "absent" and 0 onto def. Use for
// sequences where Pn=0 means "one" (cursor motion, scrolling, counts).
func (t *AnsiTerm) param(i, def int) int {
	if i >= len(t.params) || t.params[i] == 0 {
		return def
	}
	return t.params[i]
}

// paramRaw returns parameter i verbatim, 0 when absent. Use for sequences
// where 0 is a meaningful selector (SGR, ED, EL).
func (t *AnsiTerm) paramRaw(i int) int {
	if i >= len(t.params) {
		return 0
	}
	return t.params[i]
}

func (t *AnsiTerm) execCSI(final byte) {
	// Private sequences (CSI ? … h/l for DEC modes, CSI > … m for xterm key
	// reporting) share final bytes with the ones below but mean something
	// else entirely, so they are dropped rather than misinterpreted.
	if t.private != 0 {
		return
	}
	switch final {
	case 'A': // CUU
		t.moveUp(t.param(0, 1))
	case 'B': // CUD
		t.moveDown(t.param(0, 1))
	case 'C': // CUF
		t.cx = ansiClamp(t.cx+t.param(0, 1), 0, t.cols-1)
		t.wrapNext = false
	case 'D': // CUB
		t.cx = ansiClamp(t.cx-t.param(0, 1), 0, t.cols-1)
		t.wrapNext = false
	case 'E': // CNL
		t.moveDown(t.param(0, 1))
		t.cx = 0
	case 'F': // CPL
		t.moveUp(t.param(0, 1))
		t.cx = 0
	case 'G', '`': // CHA / HPA
		t.cx = ansiClamp(t.param(0, 1)-1, 0, t.cols-1)
		t.wrapNext = false
	case 'd': // VPA
		t.cy = ansiClamp(t.param(0, 1)-1, 0, t.rows-1)
		t.wrapNext = false
	case 'H', 'f': // CUP / HVP
		t.cy = ansiClamp(t.param(0, 1)-1, 0, t.rows-1)
		t.cx = ansiClamp(t.param(1, 1)-1, 0, t.cols-1)
		t.wrapNext = false
	case 'J': // ED
		t.eraseDisplay(t.paramRaw(0))
	case 'K': // EL
		t.eraseLine(t.paramRaw(0))
	case 'L': // IL
		t.insertLines(t.param(0, 1))
	case 'M': // DL
		t.deleteLines(t.param(0, 1))
	case '@': // ICH
		t.insertChars(t.param(0, 1))
	case 'P': // DCH
		t.deleteChars(t.param(0, 1))
	case 'X': // ECH
		t.eraseChars(t.param(0, 1))
	case 'S': // SU
		t.scrollUp(t.param(0, 1))
	case 'T': // SD
		t.scrollDown(t.param(0, 1))
	case 'm': // SGR
		t.sgr()
	case 'r': // DECSTBM
		t.setScrollRegion(t.param(0, 1)-1, t.param(1, t.rows)-1)
	case 's': // SCOSC
		t.saveCx, t.saveCy = t.cx, t.cy
	case 'u': // SCORC
		t.cx = ansiClamp(t.saveCx, 0, t.cols-1)
		t.cy = ansiClamp(t.saveCy, 0, t.rows-1)
		t.wrapNext = false
	}
	// Mode changes (h/l), device reports (c/n) and anything unknown carry no
	// state we model and are dropped here.
}

// ---------------------------------------------------------------------------
// Screen operations
// ---------------------------------------------------------------------------

func (t *AnsiTerm) blank() Cell {
	return Cell{Rune: ' ', Fg: AnsiDefaultColor, Bg: AnsiDefaultColor}
}

func (t *AnsiTerm) clearRow(row int) {
	blank := t.blank()
	for c := range t.cells[row] {
		t.cells[row][c] = blank
	}
}

// put writes one stream byte as text, decoding UTF-8 across calls.
func (t *AnsiTerm) put(b byte) {
	if b < 0x80 && len(t.utf8Buf) == 0 {
		t.putRune(rune(b))
		return
	}
	t.utf8Buf = append(t.utf8Buf, b)
	if utf8.FullRune(t.utf8Buf) {
		r, _ := utf8.DecodeRune(t.utf8Buf)
		t.utf8Buf = t.utf8Buf[:0]
		t.putRune(r)
		return
	}
	if len(t.utf8Buf) >= utf8.UTFMax {
		t.utf8Buf = t.utf8Buf[:0]
		t.putRune(utf8.RuneError)
	}
}

// putRune stores one rune at the cursor with the current attributes. Writing
// in the last column parks the cursor there and defers the wrap until the
// next rune, matching how real terminals treat the right margin.
func (t *AnsiTerm) putRune(r rune) {
	if t.wrapNext {
		t.wrapNext = false
		t.cx = 0
		t.lineFeed()
	}
	cell := t.attr
	cell.Rune = r
	t.cells[t.cy][t.cx] = cell
	if t.cx+1 >= t.cols {
		t.wrapNext = true
	} else {
		t.cx++
	}
}

func (t *AnsiTerm) tab() {
	t.wrapNext = false
	next := (t.cx/ansiTabWidth + 1) * ansiTabWidth
	t.cx = ansiClamp(next, 0, t.cols-1)
}

func (t *AnsiTerm) lineFeed() {
	t.wrapNext = false
	switch {
	case t.cy == t.bot:
		t.scrollUp(1)
	case t.cy+1 < t.rows:
		t.cy++
	}
}

func (t *AnsiTerm) reverseIndex() {
	t.wrapNext = false
	switch {
	case t.cy == t.top:
		t.scrollDown(1)
	case t.cy > 0:
		t.cy--
	}
}

func (t *AnsiTerm) moveUp(n int) {
	limit := 0
	if t.cy >= t.top {
		limit = t.top
	}
	t.cy = ansiClamp(t.cy-n, limit, t.rows-1)
	t.wrapNext = false
}

func (t *AnsiTerm) moveDown(n int) {
	limit := t.rows - 1
	if t.cy <= t.bot {
		limit = t.bot
	}
	t.cy = ansiClamp(t.cy+n, 0, limit)
	t.wrapNext = false
}

func (t *AnsiTerm) setScrollRegion(top, bot int) {
	top = ansiClamp(top, 0, t.rows-1)
	bot = ansiClamp(bot, 0, t.rows-1)
	if top >= bot {
		top, bot = 0, t.rows-1
	}
	t.top, t.bot = top, bot
	t.cy, t.cx = top, 0
	t.wrapNext = false
}

// scrollUp moves the scroll region up by n lines, blanking the lines that
// scroll in at the bottom.
func (t *AnsiTerm) scrollUp(n int) {
	if n <= 0 {
		return
	}
	if n > t.bot-t.top+1 {
		n = t.bot - t.top + 1
	}
	for r := t.top; r <= t.bot-n; r++ {
		t.cells[r], t.cells[r+n] = t.cells[r+n], t.cells[r]
	}
	for r := t.bot - n + 1; r <= t.bot; r++ {
		t.clearRow(r)
	}
}

// scrollDown moves the scroll region down by n lines, blanking the lines
// that scroll in at the top.
func (t *AnsiTerm) scrollDown(n int) {
	if n <= 0 {
		return
	}
	if n > t.bot-t.top+1 {
		n = t.bot - t.top + 1
	}
	for r := t.bot; r >= t.top+n; r-- {
		t.cells[r], t.cells[r-n] = t.cells[r-n], t.cells[r]
	}
	for r := t.top; r < t.top+n; r++ {
		t.clearRow(r)
	}
}

// eraseDisplay implements ED: 0 = cursor to end, 1 = start to cursor,
// 2 and 3 = whole screen. The cursor never moves.
func (t *AnsiTerm) eraseDisplay(mode int) {
	blank := t.blank()
	switch mode {
	case 0:
		for c := t.cx; c < t.cols; c++ {
			t.cells[t.cy][c] = blank
		}
		for r := t.cy + 1; r < t.rows; r++ {
			t.clearRow(r)
		}
	case 1:
		for r := 0; r < t.cy; r++ {
			t.clearRow(r)
		}
		for c := 0; c <= t.cx && c < t.cols; c++ {
			t.cells[t.cy][c] = blank
		}
	case 2, 3:
		for r := 0; r < t.rows; r++ {
			t.clearRow(r)
		}
	}
	t.wrapNext = false
}

// eraseLine implements EL: 0 = cursor to end of line, 1 = start of line to
// cursor, 2 = whole line.
func (t *AnsiTerm) eraseLine(mode int) {
	blank := t.blank()
	from, to := 0, t.cols-1
	switch mode {
	case 0:
		from = t.cx
	case 1:
		to = t.cx
	case 2:
	default:
		return
	}
	for c := from; c <= to && c < t.cols; c++ {
		t.cells[t.cy][c] = blank
	}
	t.wrapNext = false
}

// eraseChars implements ECH: blank n cells from the cursor without moving it
// and without shifting the rest of the line.
func (t *AnsiTerm) eraseChars(n int) {
	blank := t.blank()
	for c := t.cx; c < t.cx+n && c < t.cols; c++ {
		t.cells[t.cy][c] = blank
	}
	t.wrapNext = false
}

// deleteChars implements DCH: drop n cells at the cursor, pulling the rest
// of the line left and blanking the tail.
func (t *AnsiTerm) deleteChars(n int) {
	row := t.cells[t.cy]
	if n > t.cols-t.cx {
		n = t.cols - t.cx
	}
	copy(row[t.cx:], row[t.cx+n:])
	blank := t.blank()
	for c := t.cols - n; c < t.cols; c++ {
		row[c] = blank
	}
	t.wrapNext = false
}

// insertChars implements ICH: shift the line right from the cursor by n,
// blanking the opened cells and dropping what falls off the right edge.
func (t *AnsiTerm) insertChars(n int) {
	row := t.cells[t.cy]
	if n > t.cols-t.cx {
		n = t.cols - t.cx
	}
	copy(row[t.cx+n:], row[t.cx:])
	blank := t.blank()
	for c := t.cx; c < t.cx+n; c++ {
		row[c] = blank
	}
	t.wrapNext = false
}

// insertLines implements IL: open n blank lines at the cursor row, scrolling
// the rest of the scroll region down. Ignored outside the region.
func (t *AnsiTerm) insertLines(n int) {
	if t.cy < t.top || t.cy > t.bot {
		return
	}
	saved := t.top
	t.top = t.cy
	t.scrollDown(n)
	t.top = saved
	t.cx = 0
	t.wrapNext = false
}

// deleteLines implements DL: drop n lines at the cursor row, pulling the
// rest of the scroll region up. Ignored outside the region.
func (t *AnsiTerm) deleteLines(n int) {
	if t.cy < t.top || t.cy > t.bot {
		return
	}
	saved := t.top
	t.top = t.cy
	t.scrollUp(n)
	t.top = saved
	t.cx = 0
	t.wrapNext = false
}

// ---------------------------------------------------------------------------
// SGR
// ---------------------------------------------------------------------------

// sgr applies a Select Graphic Rendition sequence to the current attributes.
func (t *AnsiTerm) sgr() {
	if len(t.params) == 0 {
		t.attr = t.blank()
		return
	}
	for i := 0; i < len(t.params); i++ {
		p := t.params[i]
		switch {
		case p == 0:
			t.attr = t.blank()
		case p == 1:
			t.attr.Bold = true
		case p == 3:
			t.attr.Italic = true
		case p == 4:
			t.attr.Underline = true
		case p == 7:
			t.attr.Reverse = true
		case p == 21 || p == 22:
			t.attr.Bold = false
		case p == 23:
			t.attr.Italic = false
		case p == 24:
			t.attr.Underline = false
		case p == 27:
			t.attr.Reverse = false
		case p >= 30 && p <= 37:
			t.attr.Fg = p - 30
		case p == 38:
			i = t.extColor(i, &t.attr.Fg)
		case p == 39:
			t.attr.Fg = AnsiDefaultColor
		case p >= 40 && p <= 47:
			t.attr.Bg = p - 40
		case p == 48:
			i = t.extColor(i, &t.attr.Bg)
		case p == 49:
			t.attr.Bg = AnsiDefaultColor
		case p >= 90 && p <= 97:
			t.attr.Fg = p - 90 + 8
		case p >= 100 && p <= 107:
			t.attr.Bg = p - 100 + 8
		}
		// Faint, blink, conceal and strike-through have no cell flag here.
	}
}

// extColor decodes the 38/48 extended color forms — ";5;idx" (palette) and
// ";2;r;g;b" (24-bit) — starting at parameter i, and returns the index of
// the last parameter it consumed.
func (t *AnsiTerm) extColor(i int, dst *int) int {
	if i+1 >= len(t.params) {
		return i
	}
	switch t.params[i+1] {
	case 5:
		if i+2 < len(t.params) {
			*dst = ansiClamp(t.params[i+2], 0, 255)
			return i + 2
		}
	case 2:
		if i+4 < len(t.params) {
			r := ansiClamp(t.params[i+2], 0, 255)
			g := ansiClamp(t.params[i+3], 0, 255)
			b := ansiClamp(t.params[i+4], 0, 255)
			*dst = AnsiRGBBase + r<<16 + g<<8 + b
			return i + 4
		}
	}
	return i + 1
}

// ansiBasePalette holds the 16 standard xterm colors (normal then bright).
var ansiBasePalette = [16][3]uint8{
	{0, 0, 0}, {205, 49, 49}, {13, 188, 121}, {229, 229, 16},
	{36, 114, 200}, {188, 63, 188}, {17, 168, 205}, {229, 229, 229},
	{102, 102, 102}, {241, 76, 76}, {35, 209, 139}, {245, 245, 67},
	{59, 142, 234}, {214, 112, 214}, {41, 184, 219}, {255, 255, 255},
}

// AnsiColorRGB resolves a Cell color to concrete channels. ok is false for
// AnsiDefaultColor, where the caller supplies its own default.
func AnsiColorRGB(c int) (r, g, b uint8, ok bool) {
	switch {
	case c >= AnsiRGBBase:
		v := c - AnsiRGBBase
		return uint8(v >> 16), uint8(v >> 8), uint8(v), true
	case c < 0:
		return 0, 0, 0, false
	case c < 16:
		p := ansiBasePalette[c]
		return p[0], p[1], p[2], true
	case c < 232:
		// 6x6x6 color cube.
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		i := c - 16
		return levels[i/36], levels[(i/6)%6], levels[i%6], true
	case c < 256:
		v := uint8(8 + (c-232)*10)
		return v, v, v, true
	}
	return 0, 0, 0, false
}

func ansiClampMin(v, lo int) int {
	if v < lo {
		return lo
	}
	return v
}

func ansiClamp(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
