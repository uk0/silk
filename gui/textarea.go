package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"strings"
)

func init() {
	core.RegisterFactory("gui.TextArea", core.TypeOf((*TextArea)(nil)))
}

// TextArea is a multi-line plain-text input — the QPlainTextEdit slot
// in the toolkit. It is the middle widget in the trio:
//
//   - Edit (single line) for one-line inputs (name, email).
//   - TextArea (this file) for paragraphs of plain text — description,
//     notes, comments. NO syntax highlighting, NO line numbers,
//     NO undo/redo, NO completer; just typing, navigation, selection,
//     scroll.
//   - CodeEditor for source code (syntax highlighting, gutter, find/
//     replace, multi-cursor, etc).
//
// The caret and selection model mirrors Edit's — a single anchor +
// cursor pair, Shift+nav extends selection, Ctrl+A selects all — but
// the storage is a slice of lines so Enter genuinely inserts a newline
// instead of dispatching Submit. Lines are stored as plain strings;
// the public API treats Text() / SetText() as the canonical form,
// joined with "\n".
type TextArea struct {
	Widget

	lines       []string
	cursorLine  int
	cursorCol   int // byte offset within lines[cursorLine]
	selLine     int // selection anchor; equals cursor when no selection
	selCol      int
	scrollY     float64 // top visible line index, in line units
	readonly    bool
	placeholder string
	padding     Padding

	cbTextChanged func(interface{}, string)
}

// NewTextArea creates an empty TextArea with one (empty) line and the
// caret at the start.
func NewTextArea() *TextArea {
	p := new(TextArea)
	p.Init(p)
	return p
}

func (this *TextArea) Init(iw IWidget) {
	this.Widget.Init(iw)
	this.lines = []string{""}
	this.padding = Theme().EditPadding
}

// --- pure helpers (no GL, no widget state) -----------------------------

// textAreaInsert returns the new lines slice + caret after inserting s
// at (line, col) into a copy of lines. s may contain "\n" runs; each
// "\n" starts a new line. The returned caret lands just past the
// inserted text. Out-of-range inputs clamp to the buffer end. The
// input slice is NOT mutated — the caller is free to use this in
// pre-flight checks too.
func textAreaInsert(lines []string, line, col int, s string) (out []string, newLine, newCol int) {
	out = append([]string(nil), lines...)
	if len(out) == 0 {
		out = []string{""}
	}
	if line < 0 {
		line = 0
	}
	if line >= len(out) {
		line = len(out) - 1
		col = len(out[line])
	}
	if col < 0 {
		col = 0
	}
	if col > len(out[line]) {
		col = len(out[line])
	}
	parts := strings.Split(s, "\n")
	if len(parts) == 1 {
		// fast path: no newlines, splice into the current line.
		row := out[line]
		out[line] = row[:col] + parts[0] + row[col:]
		return out, line, col + len(parts[0])
	}
	// Multi-line splice: head stays on the original line, tail becomes
	// the suffix on the last new line, parts[1..n-2] become full lines.
	row := out[line]
	head := row[:col] + parts[0]
	tailExisting := row[col:]
	lastPart := parts[len(parts)-1]
	mid := parts[1 : len(parts)-1]
	// Build the new slice: [..., head, mid..., lastPart+tailExisting, ...]
	rebuilt := make([]string, 0, len(out)+len(parts)-1)
	rebuilt = append(rebuilt, out[:line]...)
	rebuilt = append(rebuilt, head)
	rebuilt = append(rebuilt, mid...)
	rebuilt = append(rebuilt, lastPart+tailExisting)
	rebuilt = append(rebuilt, out[line+1:]...)
	return rebuilt, line + len(parts) - 1, len(lastPart)
}

// textAreaDeleteRange returns the new lines + caret after deleting
// everything in [(line0,col0), (line1,col1)). Order-independent: if
// the end point precedes the start, the two are swapped. The returned
// caret lands at the (canonicalised) start. Clamps out-of-range
// coordinates to the buffer end, mirroring textAreaInsert.
func textAreaDeleteRange(lines []string, line0, col0, line1, col1 int) (out []string, newLine, newCol int) {
	out = append([]string(nil), lines...)
	if len(out) == 0 {
		out = []string{""}
	}
	// Canonicalise [start, end): start <= end.
	if line0 > line1 || (line0 == line1 && col0 > col1) {
		line0, col0, line1, col1 = line1, col1, line0, col0
	}
	if line0 < 0 {
		line0 = 0
		col0 = 0
	}
	if line1 >= len(out) {
		line1 = len(out) - 1
		col1 = len(out[line1])
	}
	if col0 < 0 {
		col0 = 0
	}
	if col0 > len(out[line0]) {
		col0 = len(out[line0])
	}
	if col1 < 0 {
		col1 = 0
	}
	if col1 > len(out[line1]) {
		col1 = len(out[line1])
	}
	if line0 == line1 {
		row := out[line0]
		out[line0] = row[:col0] + row[col1:]
		return out, line0, col0
	}
	// Across-lines delete: head of line0 + tail of line1 merge, the
	// middle lines (and any line between) drop out.
	merged := out[line0][:col0] + out[line1][col1:]
	rebuilt := make([]string, 0, len(out)-(line1-line0))
	rebuilt = append(rebuilt, out[:line0]...)
	rebuilt = append(rebuilt, merged)
	rebuilt = append(rebuilt, out[line1+1:]...)
	return rebuilt, line0, col0
}

// --- public API --------------------------------------------------------

// Text returns the full buffer joined with "\n". Mirrors QPlainTextEdit
// :: toPlainText.
func (this *TextArea) Text() string {
	return strings.Join(this.lines, "\n")
}

// SetText replaces the entire buffer. The caret moves to the end and
// the selection collapses, matching QPlainTextEdit semantics. Fires
// SigTextChanged if installed. A subsequent Layout/Update is not
// strictly required — the next Draw picks up the new lines.
func (this *TextArea) SetText(s string) {
	this.lines = strings.Split(s, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.cursorLine = len(this.lines) - 1
	this.cursorCol = len(this.lines[this.cursorLine])
	this.selLine = this.cursorLine
	this.selCol = this.cursorCol
	this.clampScroll()
	this.emitChanged()
	if this.Self() != nil {
		this.Self().Update()
	}
}

// SetPlaceholder sets the muted hint text drawn when the buffer is
// empty and unfocused. Empty by default.
func (this *TextArea) SetPlaceholder(s string) {
	this.placeholder = s
	if this.Self() != nil {
		this.Self().Update()
	}
}

// Placeholder returns the configured hint text.
func (this *TextArea) Placeholder() string {
	return this.placeholder
}

// IsReadOnly reports whether typing-driven mutations are blocked.
func (this *TextArea) IsReadOnly() bool {
	return this.readonly
}

// SetReadOnly toggles the read-only flag. A read-only TextArea still
// accepts focus, scroll and selection so the user can copy text; only
// mutations (typing, Backspace, Delete, paste, Enter) are dropped.
func (this *TextArea) SetReadOnly(b bool) {
	this.readonly = b
	if this.Self() != nil {
		this.Self().Update()
	}
}

// LineCount returns the number of logical lines (always >= 1).
func (this *TextArea) LineCount() int {
	if len(this.lines) == 0 {
		return 1
	}
	return len(this.lines)
}

// SigTextChanged registers the callback fired on every mutation that
// changes Text(). Matches Edit.SigTextChanged: (sender, newText).
func (this *TextArea) SigTextChanged(fn func(interface{}, string)) {
	this.cbTextChanged = fn
}

// SelectAll selects the entire buffer; the caret lands at the end.
func (this *TextArea) SelectAll() {
	this.selLine = 0
	this.selCol = 0
	this.cursorLine = len(this.lines) - 1
	if this.cursorLine < 0 {
		this.cursorLine = 0
	}
	this.cursorCol = len(this.lines[this.cursorLine])
	if this.Self() != nil {
		this.Self().Update()
	}
}

// --- internal mutation helpers ---------------------------------------

// hasSelection reports whether the anchor differs from the caret.
func (this *TextArea) hasSelection() bool {
	return this.selLine != this.cursorLine || this.selCol != this.cursorCol
}

// canonicalSelection returns the selection bounds in start <= end
// order. When no selection is active both bounds equal the caret.
func (this *TextArea) canonicalSelection() (l0, c0, l1, c1 int) {
	l0, c0 = this.selLine, this.selCol
	l1, c1 = this.cursorLine, this.cursorCol
	if l0 > l1 || (l0 == l1 && c0 > c1) {
		l0, c0, l1, c1 = l1, c1, l0, c0
	}
	return
}

// deleteSelection removes the active selection if any and parks the
// caret at the (canonical) start. Returns true when something was
// actually deleted.
func (this *TextArea) deleteSelection() bool {
	if !this.hasSelection() {
		return false
	}
	l0, c0, l1, c1 := this.canonicalSelection()
	this.lines, this.cursorLine, this.cursorCol = textAreaDeleteRange(this.lines, l0, c0, l1, c1)
	this.selLine = this.cursorLine
	this.selCol = this.cursorCol
	return true
}

// insertAtCursor inserts s at the current caret, replacing any active
// selection. Fires SigTextChanged.
func (this *TextArea) insertAtCursor(s string) {
	if this.readonly {
		return
	}
	this.deleteSelection()
	this.lines, this.cursorLine, this.cursorCol = textAreaInsert(this.lines, this.cursorLine, this.cursorCol, s)
	this.selLine = this.cursorLine
	this.selCol = this.cursorCol
	this.scrollToCaret()
	this.emitChanged()
	if this.Self() != nil {
		this.Self().Update()
	}
}

// moveCursor sets the caret to (line, col), clamped to the buffer.
// When extend is false the anchor collapses to the caret.
func (this *TextArea) moveCursor(line, col int, extend bool) {
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	if col < 0 {
		col = 0
	}
	if col > len(this.lines[line]) {
		col = len(this.lines[line])
	}
	this.cursorLine = line
	this.cursorCol = col
	if !extend {
		this.selLine = line
		this.selCol = col
	}
	this.scrollToCaret()
	if this.Self() != nil {
		this.Self().Update()
	}
}

// emitChanged fires the registered SigTextChanged callback, if any.
func (this *TextArea) emitChanged() {
	if this.cbTextChanged != nil {
		this.cbTextChanged(this.Self(), this.Text())
	}
}

// visibleRowHeight returns the per-line pixel height, falling back to
// a sensible constant when no font is available (the test harness has
// no GL context and Theme().Font may be nil).
func (this *TextArea) visibleRowHeight() float64 {
	f := Theme().Font
	if f == nil {
		return 16
	}
	fe := f.FontExtents()
	if fe == nil || fe.Height <= 0 {
		return 16
	}
	return fe.Height
}

// scrollToCaret nudges scrollY just enough to keep the caret line
// inside the viewport. No-op when the widget has zero height (e.g.
// before the first Layout).
func (this *TextArea) scrollToCaret() {
	rh := this.visibleRowHeight()
	if rh <= 0 || this.h <= 0 {
		return
	}
	visible := (this.h - this.padding.T - this.padding.B) / rh
	if visible < 1 {
		visible = 1
	}
	if float64(this.cursorLine) < this.scrollY {
		this.scrollY = float64(this.cursorLine)
	} else if float64(this.cursorLine) > this.scrollY+visible-1 {
		this.scrollY = float64(this.cursorLine) - visible + 1
	}
	this.clampScroll()
}

// clampScroll keeps scrollY non-negative and within the line count.
func (this *TextArea) clampScroll() {
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	max := float64(len(this.lines) - 1)
	if max < 0 {
		max = 0
	}
	if this.scrollY > max {
		this.scrollY = max
	}
}

// --- event handlers --------------------------------------------------

func (this *TextArea) OnLeftDown(x, y float64) {
	this.SetFocus()
	this.placeCaretAt(x, y, IsKeyDown(KeyShift))
}

// placeCaretAt converts a viewport-relative point to a (line, col) and
// moves the caret there. When extend is true the anchor stays put.
func (this *TextArea) placeCaretAt(x, y float64, extend bool) {
	rh := this.visibleRowHeight()
	if rh <= 0 {
		return
	}
	relY := y - this.padding.T
	line := int(this.scrollY + relY/rh)
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	col := this.columnAtX(line, x-this.padding.L)
	this.moveCursor(line, col, extend)
}

// columnAtX picks the byte offset on `line` closest to x (in line-
// local pixel coordinates). Falls back to the line length when no
// font is available.
func (this *TextArea) columnAtX(line int, x float64) int {
	if line < 0 || line >= len(this.lines) {
		return 0
	}
	row := this.lines[line]
	if row == "" || x <= 0 {
		return 0
	}
	f := Theme().Font
	if f == nil {
		return len(row)
	}
	// Walk runes left-to-right and stop when we cross x.
	best := 0
	for i := 1; i <= len(row); i++ {
		ext := f.TextExtents(row[:i])
		if ext == nil {
			return len(row)
		}
		if ext.XAdvance >= x {
			// Choose whichever of i-1/i is closer to x.
			if i > 1 {
				prev := f.TextExtents(row[:i-1])
				if prev != nil && (x-prev.XAdvance) < (ext.XAdvance-x) {
					return i - 1
				}
			}
			return i
		}
		best = i
	}
	return best
}

func (this *TextArea) OnTextInput(s string) {
	if this.readonly {
		return
	}
	this.insertAtCursor(s)
}

func (this *TextArea) OnKeyDown(key int, repeat bool) {
	shift := IsKeyDown(KeyShift)
	ctrl := IsKeyDown(KeyCtrl)
	switch key {
	case KeyEnter:
		if this.readonly {
			return
		}
		this.insertAtCursor("\n")
	case KeyBackSpace:
		if this.readonly {
			return
		}
		if this.deleteSelection() {
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
			return
		}
		// No selection: delete one rune-or-line before the caret.
		if this.cursorCol > 0 {
			// Step back one byte; for ASCII this matches Edit's "one rune"
			// behaviour. Multi-byte runes are rare enough in plain-text
			// forms that we keep the implementation lean here.
			start := this.cursorCol - 1
			this.lines, this.cursorLine, this.cursorCol = textAreaDeleteRange(this.lines, this.cursorLine, start, this.cursorLine, this.cursorCol)
			this.selLine = this.cursorLine
			this.selCol = this.cursorCol
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
		} else if this.cursorLine > 0 {
			// Join with the previous line.
			prevLen := len(this.lines[this.cursorLine-1])
			this.lines, this.cursorLine, this.cursorCol = textAreaDeleteRange(this.lines, this.cursorLine-1, prevLen, this.cursorLine, 0)
			this.selLine = this.cursorLine
			this.selCol = this.cursorCol
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
		}
	case KeyDelete:
		if this.readonly {
			return
		}
		if this.deleteSelection() {
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
			return
		}
		row := this.lines[this.cursorLine]
		if this.cursorCol < len(row) {
			this.lines, this.cursorLine, this.cursorCol = textAreaDeleteRange(this.lines, this.cursorLine, this.cursorCol, this.cursorLine, this.cursorCol+1)
			this.selLine = this.cursorLine
			this.selCol = this.cursorCol
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
		} else if this.cursorLine < len(this.lines)-1 {
			this.lines, this.cursorLine, this.cursorCol = textAreaDeleteRange(this.lines, this.cursorLine, this.cursorCol, this.cursorLine+1, 0)
			this.selLine = this.cursorLine
			this.selCol = this.cursorCol
			this.emitChanged()
			if this.Self() != nil {
				this.Self().Update()
			}
		}
	case KeyLeft:
		if this.cursorCol > 0 {
			this.moveCursor(this.cursorLine, this.cursorCol-1, shift)
		} else if this.cursorLine > 0 {
			this.moveCursor(this.cursorLine-1, len(this.lines[this.cursorLine-1]), shift)
		} else {
			this.moveCursor(0, 0, shift)
		}
	case KeyRight:
		if this.cursorCol < len(this.lines[this.cursorLine]) {
			this.moveCursor(this.cursorLine, this.cursorCol+1, shift)
		} else if this.cursorLine < len(this.lines)-1 {
			this.moveCursor(this.cursorLine+1, 0, shift)
		} else {
			this.moveCursor(this.cursorLine, this.cursorCol, shift)
		}
	case KeyUp:
		if this.cursorLine > 0 {
			this.moveCursor(this.cursorLine-1, this.cursorCol, shift)
		} else {
			this.moveCursor(0, 0, shift)
		}
	case KeyDown:
		if this.cursorLine < len(this.lines)-1 {
			this.moveCursor(this.cursorLine+1, this.cursorCol, shift)
		} else {
			this.moveCursor(this.cursorLine, len(this.lines[this.cursorLine]), shift)
		}
	case KeyHome:
		if ctrl {
			this.moveCursor(0, 0, shift)
		} else {
			this.moveCursor(this.cursorLine, 0, shift)
		}
	case KeyEnd:
		if ctrl {
			last := len(this.lines) - 1
			this.moveCursor(last, len(this.lines[last]), shift)
		} else {
			this.moveCursor(this.cursorLine, len(this.lines[this.cursorLine]), shift)
		}
	case 'A':
		if ctrl {
			this.SelectAll()
		}
	case 'C':
		if ctrl {
			this.copy()
		}
	case 'X':
		if ctrl && !this.readonly {
			this.cut()
		}
	case 'V':
		if ctrl && !this.readonly {
			this.paste()
		}
	}
}

func (this *TextArea) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * defaultWheelScrollLines
	this.clampScroll()
	if this.Self() != nil {
		this.Self().Update()
	}
}

func (this *TextArea) Cursor() *Cursor {
	return cursorIBeam
}

// --- clipboard --------------------------------------------------------

// selectionText returns the canonicalised selection as a plain string.
func (this *TextArea) selectionText() string {
	if !this.hasSelection() {
		return ""
	}
	l0, c0, l1, c1 := this.canonicalSelection()
	if l0 == l1 {
		return this.lines[l0][c0:c1]
	}
	var b strings.Builder
	b.WriteString(this.lines[l0][c0:])
	b.WriteByte('\n')
	for i := l0 + 1; i < l1; i++ {
		b.WriteString(this.lines[i])
		b.WriteByte('\n')
	}
	b.WriteString(this.lines[l1][:c1])
	return b.String()
}

func (this *TextArea) copy() {
	s := this.selectionText()
	if s == "" {
		return
	}
	Clipboard.Clear()
	Clipboard.SetData(s)
}

func (this *TextArea) cut() {
	if !this.hasSelection() {
		return
	}
	s := this.selectionText()
	Clipboard.Clear()
	Clipboard.SetData(s)
	this.deleteSelection()
	this.emitChanged()
	if this.Self() != nil {
		this.Self().Update()
	}
}

func (this *TextArea) paste() {
	i, err := Clipboard.Data("text/plain")
	if err != nil {
		return
	}
	if s, ok := i.(string); ok {
		this.insertAtCursor(s)
	}
}

// --- drawing ----------------------------------------------------------

func (this *TextArea) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	t := Theme()
	iw := this.Self()

	// Background.
	g.Rectangle(0, 0, this.w, this.h)
	if this.readonly {
		g.SetBrush1(t.FormColor)
	} else {
		g.SetBrush1(t.ViewBGColor)
	}
	g.Fill()

	// Frame + focus ring (same look as Edit).
	t.DrawEditFrame(g, 0, 0, this.w, this.h, iw.HasFocus(), iw.IsHover(), this.readonly)

	rh := this.visibleRowHeight()
	if rh <= 0 {
		return
	}

	// Clip the inside region so over-wide lines don't bleed onto the
	// frame.
	m := this.padding
	g.Rectangle(m.L, m.T, this.w-m.L-m.R, this.h-m.T-m.B)
	g.Clip()
	g.Translate(m.L, m.T)

	font := t.Font
	g.SetFont(font)

	// Placeholder when empty and unfocused.
	if !iw.HasFocus() && len(this.lines) == 1 && this.lines[0] == "" && this.placeholder != "" {
		g.SetBrush1(paint.Color{R: 156, G: 163, B: 175, A: 255})
		fe := font.FontExtents()
		if fe != nil {
			g.DrawText1(0, fe.Ascent, this.placeholder)
		}
		return
	}

	// Selection highlight (background) followed by line text.
	l0, c0, l1, c1 := this.canonicalSelection()
	hasSel := this.hasSelection()
	selColor := t.HighLightColor
	if !iw.HasFocus() {
		// Match Edit: a desaturated highlight when the widget is not
		// focused but always-show-selection were enabled. For simplicity
		// here we just leave the same colour — focus state controls
		// whether the caret blinks below.
	}

	firstVisible := int(this.scrollY)
	if firstVisible < 0 {
		firstVisible = 0
	}
	innerH := this.h - m.T - m.B
	lastVisible := firstVisible + int(innerH/rh) + 2
	if lastVisible > len(this.lines) {
		lastVisible = len(this.lines)
	}

	for ln := firstVisible; ln < lastVisible; ln++ {
		row := this.lines[ln]
		y := float64(ln-firstVisible) * rh

		// Per-line selection rectangle.
		if hasSel && ln >= l0 && ln <= l1 {
			startCol := 0
			endCol := len(row)
			if ln == l0 {
				startCol = c0
			}
			if ln == l1 {
				endCol = c1
			}
			x0 := 0.0
			x1 := 0.0
			if font != nil {
				if startCol > 0 {
					if ext := font.TextExtents(row[:startCol]); ext != nil {
						x0 = ext.XAdvance
					}
				}
				if endCol > startCol {
					if ext := font.TextExtents(row[:endCol]); ext != nil {
						x1 = ext.XAdvance
					}
				}
			}
			// Empty-line selection still gets a tiny strip so the user
			// can see where the selection extends.
			if x1 == x0 && ln < l1 {
				x1 = x0 + 4
			}
			if x1 > x0 {
				g.SetBrush1(selColor)
				g.Rectangle(x0, y, x1-x0, rh)
				g.Fill()
			}
		}

		// Line text.
		if row != "" {
			g.SetBrush1(t.TextColor)
			fe := font.FontExtents()
			if fe != nil {
				g.DrawText1(0, y+fe.Ascent, row)
			}
		}

		// Caret (only on the cursor line, when focused, when not
		// read-only). We position it inside the per-line loop so the
		// y/x math is local to this iteration.
		if ln == this.cursorLine && iw.HasFocus() && !this.readonly && !hasSel {
			cx := 0.0
			if font != nil && this.cursorCol > 0 {
				if ext := font.TextExtents(row[:this.cursorCol]); ext != nil {
					cx = ext.XAdvance
				}
			}
			t.DrawCaret(g, cx, y, 2.0, rh)
		}
	}
}

// --- layout / properties ---------------------------------------------

func (this *TextArea) SizeHints() SizeHints {
	return SizeHints{
		Width:  200,
		Height: 80,
		Policy: GrowHorizontal | GrowVertical,
	}
}

func (this *TextArea) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("占位文本", this.Placeholder, this.SetPlaceholder)
	list.AddProperty("只读", this.IsReadOnly, this.SetReadOnly)
}
