package gui

import (
	"github.com/uk0/silk/core"
	//	"github.com/uk0/silk/factory"
	"github.com/uk0/silk/paint"
	"math"
	"strings"
	"unicode"
)

// defaultWheelScrollLines is the number of lines to scroll per wheel notch.
// Matches the Windows default of 3 lines per WHEEL_DELTA.
const defaultWheelScrollLines = 3

func init() {
	core.RegisterFactory("gui.Edit", core.TypeOf((*Edit)(nil))) //((*Edit)(nil)))
}

// 文本编辑框
type IEdit interface {
	IWidget
	SetText(s string)
	Text() string
	SelectAll()
}

// 文本编辑框
type Edit struct {
	ScrollArea
	TextBlock
	readonly       bool
	mouseDown      bool
	alwaysDrawSel  bool
	caretXSaved    bool
	clickSelection bool
	sel0, sel1     int
	savedCaretX    float64
	downX          float64
	downY          float64
	padding        Padding
	noFrame        bool

	cbTextEdited  func(interface{}, string)
	cbTextChanged func(interface{}, string)
	cbVerify      func(interface{}, string) bool
	cbSubmit      func(interface{}, string)

	// validator, when non-nil, gates every typed keystroke and every
	// SetText call. Mirrors Qt's QLineEdit::setValidator semantics:
	// keystrokes that would produce an Invalid result are dropped on
	// the floor; SetText with an Invalid argument is a no-op so a
	// programmatic caller can't put the widget into an unreachable
	// state. Intermediate is permitted in both paths so the user can
	// keep typing toward a valid value.
	validator Validator

	// valid caches the last classification of the buffer: true when no
	// validator is installed or the text is Acceptable, false for
	// Intermediate / Invalid. revalidate() keeps it fresh on every text
	// mutation and on SetValidator; it drives the error border in Draw and
	// IsValid() without re-running the validator each frame.
	valid bool

	// validationError holds the human-readable reason the field is invalid,
	// sourced from the validator's ErrorMessage (when it implements
	// ErrorMessager) or a generic fallback. Empty while valid.
	validationError string

	// completer, when non-nil, refreshes its Suggestions list on every
	// keystroke so the host (or an attached popup) can render the
	// current candidate set. The Edit itself does not draw a popup — a
	// follow-up widget (CompleterPopup) consumes Completer.Suggestions
	// to render the list. This split keeps the data layer testable
	// without GL state and lets hosts attach custom popup geometry.
	completer *Completer

	// completionPrefixStart is the byte offset in Text() where the
	// active "completion prefix" begins. Set on every Filter call so
	// AcceptCompletion knows what range to replace when the user picks
	// a candidate. Defaults to the entire current text on a fresh edit.
	completionPrefixStart int

	// maxLength caps the buffer at this many runes when > 0. Zero (the
	// default) means unlimited. Typing past the cap is silently rejected
	// in OnTextInput; pastes longer than the remaining headroom are
	// truncated to fit in pasteString. SetMaxLength(N) with N below the
	// current rune length truncates the buffer to N runes and fires the
	// change callback — the API call is a deliberate limit reset, not a
	// stray keystroke, so the trim is loud (callback) but not user-
	// visible (no toast).
	maxLength int
}

func NewEdit() *Edit {
	p := new(Edit)
	p.Init(p)
	return p
}

func (this *Edit) Init(iw IWidget) {
	this.ScrollArea.Init(iw)
	this.padding = Theme().EditPadding
	// Valid until a validator proves otherwise; keeps the error border off
	// for the common no-validator case regardless of field zero values.
	this.valid = true
}

func (this *Edit) Draw(g paint.Painter) {
	//	core.Debug("(this *Edit) Draw()", this.w, this.h)
	//	this.prepare()

	g.Save()
	defer g.Restore()

	iw := this.Self()

	g.Rectangle(0, 0, this.w, this.h)
	if this.IsReadOnly() {
		g.SetBrush1(Theme().FormColor)
	} else {
		g.SetBrush1(Theme().ViewBGColor)
	}
	g.Fill()

	if !this.noFrame {
		if this.validator != nil && !this.valid {
			// Text fails its validator: draw the frame in error red instead
			// of the normal focus/hover/border accents.
			this.drawErrorFrame(g)
		} else {
			Theme().DrawEditFrame(g, 0, 0, this.w, this.h,
				iw.HasFocus(), iw.IsHover(), this.readonly)
		}
	}

	t := Theme()
	m := this.padding
	g.Translate(m.L, m.T)

	sel0 := this.sel0
	sel1 := this.sel1
	if sel0 > sel1 {
		t := sel0
		sel0 = sel1
		sel1 = t
	}
	fe := this.TextBlock.fontExtents()
	//	rh := this.RowHeight()
	sx, sy := this.ScrollPos()
	//core.Debug(sx, sy)
	if sx > 0 || sy > 0 {
		g.Translate(-sx, -sy*this.RowHeight())
	}
	focus := this.HasFocus()
	var selColor paint.Color
	if focus {
		selColor = t.HighLightColor
	} else {
		ihsl := paint.HSLModel.Convert(t.HighLightColor)
		hsl := ihsl.(paint.HSL)
		hsl.S = 0
		selColor = paint.ColorModel.Convert(hsl).(paint.Color)
	}
	g.SetFont(this.Font())
	g.SetBrush1(Theme().TextColor)

	_, vh := this.ViewportSize()
	rh := this.RowHeight()
	vrs := math.Floor(vh / rh)

	ln := int(sy)

	if ln < 0 {
		//	return
	}

	end := int(vrs) + ln + 2
	if end > len(this.rows) {
		end = len(this.rows)
	}

	if end == 0 {
		if focus && !this.readonly {
			t.DrawCaret(g, 0, 0, 2.0, fe.Height)
		}
		return
	}

	for ; ln < end; ln++ {
		//	core.Debug(ln, len(this.rows))
		row := this.rows[ln]

		gs := row.glyphs
		if sel0 < row.end && sel1 >= row.begin {
			a := sel0 - row.begin
			a1 := a
			if a < 0 {
				a = 0
			}
			b := sel1 - row.begin
			gl := len(gs)
			b1 := b
			if b > gl {
				b = gl
			}
			if b > a {
				// 正向选择
				if focus || this.alwaysDrawSel {
					// 选择范围背景
					g.SetBrush1(selColor)
					x1 := gs[a].X
					y1 := float64(ln) * fe.Height
					var w1 float64
					if b1 > gl {
						w1 = this.w - m.R - m.L - x1
					} else {
						w1 = gs[b-1].X + float64(gs[b-1].A) - gs[a].X
					}
					h1 := fe.Height
					g.Rectangle(x1, y1, w1, h1)
					g.Fill()
				}
				g.SetBrush1(t.TextColor)
				g.DrawGlyphs(gs[a:b])
			}
			if a1 >= 0 {
				// 反向选择
				if focus && !this.readonly && this.sel0 == sel1 {
					// caret画在前面
					t.DrawCaret(g, gs[a1].X, float64(ln)*fe.Height, 2.0, fe.Height)
				}
				g.SetBrush1(t.TextColor)
				g.DrawGlyphs(gs[:a1])
			}
			if b1 < gl {
				// 正向选择
				if focus && !this.readonly && this.sel0 == sel0 {
					// caret画在后面
					t.DrawCaret(g, gs[b1].X, float64(ln)*fe.Height, 2.0, fe.Height)
				}
				g.SetBrush1(t.TextColor)
				g.DrawGlyphs(gs[b1:gl])
			}
		} else {
			g.DrawGlyphs(gs)
		}
	}
}

// errorFrameColor is the border colour drawn around an Edit whose text
// fails its validator. No theme slot exists for an error colour, so we use
// a fixed red-500 matching the theme's tailwind-derived palette (cf. the
// blue-300 hover accent in DrawEditFrame) rather than reach into theme.go.
var errorFrameColor = paint.Color{239, 68, 68, 255} // tailwind red-500

// drawErrorFrame paints the Edit's rounded border in errorFrameColor using
// the same geometry Theme().DrawEditFrame uses, so an invalid field reads
// as a red-outlined version of the normal frame. Focus / hover accents are
// intentionally dropped — the error state takes visual priority.
func (this *Edit) drawErrorFrame(g paint.Painter) {
	radius := 4.0
	roundedRect(g, 0, 0, this.w, this.h, radius)
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.FillPreserve()
	g.SetPen1(errorFrameColor, 2)
	g.Stroke()
}

func (this *Edit) OnMouseEnter() {
	//	this.prepare()
	this.Self().Update()
	//core.Debug("(this *Edit) OnMouseEnter()")
}

func (this *Edit) OnMouseLeave() {
	this.Self().Update()
	//core.Debug("(this *Edit) OnMouseLeave()")
}

func (this *Edit) OnLeftDown(x, y float64) {
	//	this.prepare()
	m := this.padding
	sx, sy := this.ScrollPos()
	x += sx - m.L
	y += sy*this.RowHeight() - m.T
	downCaret := this.TextBlock.PointToPos(x, y)
	a, b := this.Selection()
	if downCaret >= a && downCaret < b {
		this.clickSelection = true
	} else {
		this.clickSelection = false
		this.sel0 = downCaret
		this.sel1 = this.sel0
	}
	this.downX = x
	this.downY = y
	this.caretXSaved = false
	this.mouseDown = true
	this.SetFocus()
	//core.Debug("(this *Edit) OnLeftDown()")
}

func (this *Edit) OnMouseMove(x, y float64) {
	if this.mouseDown {
		m := this.padding
		sx, sy := this.ScrollPos()
		x += sx - m.L
		y += sy*this.RowHeight() - m.T

		if this.clickSelection {
			if math.Abs(x-this.downX) > 4 || math.Abs(y-this.downY) > 4 {
				this.PopCapture()
				selText := this.SelectionText()
				selText1 := []rune(selText)
				if len(selText1) > 20 {
					selText1 = append(selText1[:18], []rune("...")...)
				}
				pixmap := paint.TextToPixmap(string(selText1), this.Font(),
					Theme().TextColor, true)

				//pixmap := paint.IconTextToPixmap(LoadIcon("clipboard"), 64, false,
				//	string(selText1), this.Font(), Theme().TextColor)
				act := this.DoDragDrop(pixmap, DndMove|DndCopy, selText)
				if act == DndMove {
					this.DeleteSelection()
				}
				//this.clickSelection = true
				this.mouseDown = false
			}
		} else {
			this.sel1 = this.TextBlock.PointToPos(x, y)
			this.caretXSaved = false
			this.ScrollToCaret()

		}
		this.Self().Update()
	}
}

func (this *Edit) OnLeftUp(x, y float64) {
	//	this.pushed = false
	if this.mouseDown {
		m := this.padding
		sx, sy := this.ScrollPos()
		x += sx - m.L
		y += sy*this.RowHeight() - m.T
		this.sel1 = this.TextBlock.PointToPos(x, y)
		if this.clickSelection {
			this.sel0 = this.sel1
		}
		this.caretXSaved = false
		this.mouseDown = false
		//if this.sel0 > this.sel1 {
		//	t := this.sel0
		//	this.sel0 = this.sel1
		//	this.sel1 = t
		//}
		this.ScrollToCaret()
		this.Self().Update()
	}
	//core.Debug("(this *Edit) OnLeftUp()")
}

func (this *Edit) OnMouseWheel(x, y, z float64) {
	if this.ml {
		this.SetScrollY(this.ScrollY() - z*defaultWheelScrollLines)
	}
}

func (this *Edit) OnMouseStop(x, y float64) {
	//this.pushed = false
	this.Self().Update()
	//core.Debug("(this *Edit) OnMouseStop()")
}

func (this *Edit) OnTextInput(s string) {
	if this.readonly {
		return
	}
	// MaxLength gate (typing path only): when the buffer is already at
	// the cap and nothing is selected (so the keystroke would grow the
	// buffer), drop the keystroke silently. The paste path is handled
	// separately by pasteString, which truncates the inserted slice
	// instead of rejecting it wholesale.
	if this.maxLength > 0 {
		begin, end := this.sel0, this.sel1
		if begin > end {
			begin, end = end, begin
		}
		grow := len([]rune(s)) - (end - begin)
		if grow > 0 && this.RunesCount()+grow > this.maxLength {
			return
		}
	}
	// Validator gate: if installed, classify the candidate text BEFORE
	// committing. Invalid drops the keystroke; Intermediate / Acceptable
	// allow the change. We compute the candidate by simulating the same
	// range replace replace() would perform, without mutating state.
	if this.validator != nil {
		begin, end := this.sel0, this.sel1
		if begin > end {
			begin, end = end, begin
		}
		text := this.Text()
		if begin > len(text) {
			begin = len(text)
		}
		if end > len(text) {
			end = len(text)
		}
		candidate := text[:begin] + s + text[end:]
		if this.validator.Validate(candidate) == Invalid {
			return
		}
	}
	this.replace(this.sel0, this.sel1, s)
	this.caretXSaved = false
	this.emitEdited()
	// Refresh completer suggestions if installed. The "active prefix"
	// is the substring starting at completionPrefixStart up to caret;
	// it falls back to the full text on a fresh edit.
	this.refreshCompleter()
}

// SetCompleter installs (or clears, when nil) the input completer.
// Subsequent text edits trigger Filter; hosts read Suggestions() and
// call AcceptCompletion(idx) when the user picks a candidate.
//
// Switching completers does not retroactively re-filter — the next
// keystroke or an explicit refreshCompleter call updates the list.
func (this *Edit) SetCompleter(c *Completer) {
	this.completer = c
}

// Completer returns the installed Completer, or nil.
func (this *Edit) Completer() *Completer { return this.completer }

// CompletionPrefix returns the substring currently being completed —
// from completionPrefixStart to the caret position. Hosts call this to
// highlight the prefix in their popup or to compute completion bounds.
func (this *Edit) CompletionPrefix() string {
	text := this.Text()
	caret := this.sel0
	if caret > len(text) {
		caret = len(text)
	}
	start := this.completionPrefixStart
	if start < 0 || start > caret {
		start = caret
	}
	return text[start:caret]
}

// SetCompletionPrefixStart pins the byte offset where the active
// completion prefix begins. Defaults to 0 (whole text). Hosts that
// implement word-by-word completion (e.g. a code editor where each
// new identifier resets the prefix start) call this on whitespace /
// boundary keystrokes.
func (this *Edit) SetCompletionPrefixStart(start int) {
	this.completionPrefixStart = start
}

// AcceptCompletion replaces the active prefix with the suggestion at
// idx in the current Suggestions list. No-op when idx is out of range
// or no completer is installed. Returns true on a successful replace.
//
// After replacement, completionPrefixStart moves to the position
// immediately after the inserted candidate, so a subsequent keystroke
// starts a fresh completion against whatever the user types next.
func (this *Edit) AcceptCompletion(idx int) bool {
	if this.completer == nil {
		return false
	}
	suggestions := this.completer.Suggestions()
	if idx < 0 || idx >= len(suggestions) {
		return false
	}
	pick := suggestions[idx]
	caret := this.sel0
	text := this.Text()
	if caret > len(text) {
		caret = len(text)
	}
	start := this.completionPrefixStart
	if start < 0 || start > caret {
		start = caret
	}
	// Replace [start:caret] with pick; leave any text past the caret
	// (unusual but possible if the user moved the caret mid-typing)
	// untouched so we don't surprise-clobber it.
	newText := text[:start] + pick + text[caret:]
	this.TextBlock.SetText(newText)
	newCaret := start + len(pick)
	this.sel0 = newCaret
	this.sel1 = newCaret
	this.completionPrefixStart = newCaret
	this.emitChanged()
	this.Layout()
	return true
}

// refreshCompleter is called from OnTextInput / SetText after the
// underlying buffer changes. No-op when no completer is set.
func (this *Edit) refreshCompleter() {
	if this.completer == nil {
		return
	}
	this.completer.Filter(this.CompletionPrefix())
}

func (this *Edit) OnKeyDown(key int, repeat bool) {
	switch key {
	case KeyBackSpace:
		if this.readonly {
			return
		}
		// Ctrl+Backspace deletes the word before the caret; a plain
		// Backspace with no selection eats one rune. With a selection
		// active either form just removes the selection (handled by the
		// sel0==sel1 guard staying false).
		if this.sel0 == this.sel1 && IsKeyDown(KeyCtrl) {
			this.deleteWordBefore()
			return
		}
		if this.sel0 == this.sel1 {
			this.sel0 = this.sel1 - 1
		}
		this.replace(this.sel0, this.sel1, "")
		this.emitEdited()
	case KeyDelete:
		if this.readonly {
			return
		}
		// Ctrl+Delete deletes the word after the caret; otherwise fall
		// back to the existing forward-delete / delete-selection logic.
		if this.sel0 == this.sel1 && IsKeyDown(KeyCtrl) {
			this.deleteWordAfter()
			return
		}
		this.DeleteSelection()
		this.emitEdited()
	case 'C':
		if IsKeyDown(KeyCtrl) {
			this.Copy()
		}
	case 'V':
		if this.readonly {
			return
		}
		if IsKeyDown(KeyCtrl) {
			this.paste()
		}
	case 'X':
		if IsKeyDown(KeyCtrl) {
			if this.readonly {
				this.Copy()
			} else {
				this.cut()
			}
		}
	case 'A':
		if IsKeyDown(KeyCtrl) {
			this.SelectAll()
		}
	case KeyLeft:
		// Ctrl/Alt+Left jumps a word; Shift extends the selection.
		shift := IsKeyDown(KeyShift)
		if IsKeyDown(KeyCtrl) || IsKeyDown(KeyMenu) {
			this.moveWordLeft(shift)
		} else if shift {
			this.moveCaret(this.sel1-1, true)
		} else {
			r, c := this.CaretRowCol()
			this.SetCaretRowCol(r, c-1)
		}
		this.caretXSaved = false
	case KeyRight:
		// Ctrl/Alt+Right jumps a word; Shift extends the selection.
		shift := IsKeyDown(KeyShift)
		if IsKeyDown(KeyCtrl) || IsKeyDown(KeyMenu) {
			this.moveWordRight(shift)
		} else if shift {
			this.moveCaret(this.sel1+1, true)
		} else {
			r, c := this.CaretRowCol()
			this.SetCaretRowCol(r, c+1)
		}
		this.caretXSaved = false
	case KeyPageUp:
		fallthrough
	case KeyUp:
		x, y := this.PosToPoint(this.CaretPos())
		if !this.caretXSaved {
			this.savedCaretX = x
		} else if x < this.savedCaretX {
			x = this.savedCaretX
		}
		if key == KeyUp {
			y -= this.RowHeight()
		} else {
			y -= this.h - this.RowHeight()
		}
		if y < 0.5*this.RowHeight() {
			y = 0.5 * this.RowHeight()
		}
		this.SetCaretPos(this.PointToPos(x, y))
		this.caretXSaved = true
		//this.Ow().Update()
	case KeyPageDown:
		fallthrough
	case KeyDown:
		x, y := this.PosToPoint(this.CaretPos())
		if !this.caretXSaved {
			this.savedCaretX = x
		} else if x < this.savedCaretX {
			x = this.savedCaretX
		}
		if key == KeyDown {
			y += this.RowHeight()
		} else {
			y += this.h - this.RowHeight()
		}
		maxY := (float64(this.SoftRowsCount()) - 0.5) * this.RowHeight()
		if y > maxY {
			y = maxY
		}
		this.SetCaretPos(this.PointToPos(x, y))
		this.caretXSaved = true
		//this.Ow().Update()
	case KeyHome:
		// Shift+Home extends the selection to the line start; plain Home
		// collapses the caret there (existing behaviour).
		if IsKeyDown(KeyShift) {
			this.moveCaret(this.RowColToPos(this.posRow(), 0), true)
		} else {
			r, _ := this.CaretRowCol()
			this.SetCaretRowCol(r, 0)
		}
	case KeyEnd:
		if IsKeyDown(KeyShift) {
			this.moveCaret(this.RowColToPos(this.posRow(), 1<<30), true)
		} else {
			r, _ := this.CaretRowCol()
			this.SetCaretRowCol(r, 1<<30)
		}
	case KeyEnter:
		if this.ml && !this.readonly {
			this.OnTextInput(DefultLineEnd)
		} else if !this.ml && !this.readonly {
			this.Submit()
		}
	}
}

// posRow returns the row index of the current caret. Single-line edits
// always report 0; multi-line ones report the soft row the caret sits on.
func (this *Edit) posRow() int {
	r, _ := this.PosToRowCol(this.sel1)
	return r
}

// moveCaret moves the caret (sel1) to pos. When extend is false the
// selection collapses (anchor sel0 follows the caret); when true the
// anchor is left in place so the selection grows/shrinks. pos is clamped
// into [0, RunesCount()]. Mirrors SetCaretPos' scroll + repaint side
// effects so a Shift-navigation paints exactly like a plain caret move.
func (this *Edit) moveCaret(pos int, extend bool) {
	rc := this.RunesCount()
	if pos < 0 {
		pos = 0
	} else if pos > rc {
		pos = rc
	}
	this.sel1 = pos
	if !extend {
		this.sel0 = pos
	}
	this.caretXSaved = false
	this.ScrollToCaret()
	this.Self().Update()
}

// runesNoSentinel returns the logical text runes (the buffer minus the
// trailing "\n" sentinel TextBlock keeps). Used by the word-motion
// helpers; returns nil for an empty edit.
func (this *Edit) runesNoSentinel() []rune {
	rc := this.RunesCount()
	if rc <= 0 {
		return nil
	}
	return this.text[:rc]
}

// moveWordLeft moves the caret to the previous word boundary; extend
// keeps the selection anchor (Shift+Ctrl/Alt+Left).
func (this *Edit) moveWordLeft(extend bool) {
	this.moveCaret(prevWordBoundary(this.runesNoSentinel(), this.sel1), extend)
}

// moveWordRight moves the caret to the next word boundary; extend keeps
// the selection anchor (Shift+Ctrl/Alt+Right).
func (this *Edit) moveWordRight(extend bool) {
	this.moveCaret(nextWordBoundary(this.runesNoSentinel(), this.sel1), extend)
}

// deleteWordBefore removes the word to the left of the caret (Ctrl+
// Backspace). No-op at the start of the text.
func (this *Edit) deleteWordBefore() {
	if this.readonly {
		return
	}
	begin := prevWordBoundary(this.runesNoSentinel(), this.sel1)
	if begin == this.sel1 {
		return
	}
	this.replace(begin, this.sel1, "")
	this.caretXSaved = false
	this.emitEdited()
}

// deleteWordAfter removes the word to the right of the caret (Ctrl+
// Delete). No-op at the end of the text.
func (this *Edit) deleteWordAfter() {
	if this.readonly {
		return
	}
	end := nextWordBoundary(this.runesNoSentinel(), this.sel1)
	if end == this.sel1 {
		return
	}
	this.replace(this.sel1, end, "")
	this.caretXSaved = false
	this.emitEdited()
}

// isWordRune reports whether r counts as part of a word for caret
// word-motion. Matches the editor's double-click convention: letters,
// digits and the underscore form words; everything else is punctuation.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// prevWordBoundary returns the rune index of the start of the word at or
// before caret, suitable for Ctrl+Left / Ctrl+Backspace. It first skips
// any whitespace immediately left of the caret, then skips a run of
// like-classed runes (word runes, or a run of punctuation). Pure: no GL
// or widget state, so it is unit-testable in isolation. caret is clamped
// into [0, len(runes)].
func prevWordBoundary(runes []rune, caret int) int {
	if caret > len(runes) {
		caret = len(runes)
	}
	i := caret
	// Skip whitespace to the left.
	for i > 0 && unicode.IsSpace(runes[i-1]) {
		i--
	}
	if i == 0 {
		return 0
	}
	// Skip the run the caret now abuts — all word runes, or all
	// non-word/non-space runes (a punctuation cluster).
	if isWordRune(runes[i-1]) {
		for i > 0 && isWordRune(runes[i-1]) {
			i--
		}
	} else {
		for i > 0 && !isWordRune(runes[i-1]) && !unicode.IsSpace(runes[i-1]) {
			i--
		}
	}
	return i
}

// nextWordBoundary returns the rune index of the start of the next word
// after caret, suitable for Ctrl+Right / Ctrl+Delete. It skips the run
// the caret sits in (word runes, or a punctuation cluster) and then any
// trailing whitespace, landing on the first rune of the following word.
// Pure: no GL or widget state. caret is clamped into [0, len(runes)].
func nextWordBoundary(runes []rune, caret int) int {
	if caret < 0 {
		caret = 0
	}
	n := len(runes)
	i := caret
	if i >= n {
		return n
	}
	// Skip the current run.
	if isWordRune(runes[i]) {
		for i < n && isWordRune(runes[i]) {
			i++
		}
	} else if !unicode.IsSpace(runes[i]) {
		for i < n && !isWordRune(runes[i]) && !unicode.IsSpace(runes[i]) {
			i++
		}
	}
	// Skip trailing whitespace to reach the next word.
	for i < n && unicode.IsSpace(runes[i]) {
		i++
	}
	return i
}

func (this *Edit) paste() {
	i, err := Clipboard.Data("text/plain")
	if err == nil {
		this.pasteString(i.(string))
	}
}

// sanitizePasteForSingleLine cleans clipboard text before it enters a
// single-line Edit. The rule: strip every CR (it's a layout artifact —
// CRLF or bare CR — that the single-line renderer can't draw), and
// replace every LF with a space (an embedded newline in a single-line
// field must not turn into two visual rows or render a tofu glyph).
// Leading and trailing whitespace are LEFT INTACT — a user who pastes
// " hello " into a search box meant the spaces, and trimming would
// surprise them. Pure helper: no widget state, unit-testable.
func sanitizePasteForSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// pasteString commits clipboard text at the current selection. For a
// single-line Edit the input is first run through
// sanitizePasteForSingleLine (CR stripped, LF -> space); multi-line
// Edits keep the raw payload. When maxLength is set, the inserted slice
// is truncated to fit the remaining headroom — existing buffer content
// is never trimmed, so a user pasting too much only loses the tail of
// the new payload, not what they already typed.
func (this *Edit) pasteString(s string) {
	if this.readonly {
		return
	}
	if !this.ml {
		s = sanitizePasteForSingleLine(s)
	}
	if this.maxLength > 0 {
		begin, end := this.sel0, this.sel1
		if begin > end {
			begin, end = end, begin
		}
		headroom := this.maxLength - (this.RunesCount() - (end - begin))
		if headroom <= 0 {
			return
		}
		rs := []rune(s)
		if len(rs) > headroom {
			s = string(rs[:headroom])
		}
	}
	this.OnTextInput(s)
}

// MaxLength returns the configured rune cap, or 0 when the buffer is
// unlimited (the default).
func (this *Edit) MaxLength() int {
	return this.maxLength
}

// SetMaxLength sets the rune cap. n <= 0 clears any existing cap. When
// n is positive and the current buffer already exceeds n, the buffer is
// truncated to n runes and the change callback fires — an explicit API
// call is a deliberate limit reset, not a stray keystroke we should
// silently swallow.
func (this *Edit) SetMaxLength(n int) {
	if n < 0 {
		n = 0
	}
	this.maxLength = n
	if n == 0 {
		return
	}
	if this.RunesCount() <= n {
		return
	}
	// Truncate via TextBlock so the trailing-"\n" sentinel is preserved.
	rs := []rune(this.Text())
	this.TextBlock.SetText(string(rs[:n]))
	if this.sel0 > n {
		this.sel0 = n
	}
	if this.sel1 > n {
		this.sel1 = n
	}
	this.emitChanged()
	this.Layout()
}

func (this *Edit) Copy() {
	s := this.SelectionText()
	if s != "" {
		Clipboard.Clear()
		Clipboard.SetData(s)
	}
}

func (this *Edit) cut() {
	if this.sel0 == this.sel1 {
		return
	}
	caret, s := this.replace(this.sel0, this.sel1, "")
	if s != "" {
		Clipboard.Clear()
		Clipboard.SetData(s)
		this.sel0, this.sel1 = caret, caret
		this.caretXSaved = false
	}
	this.emitEdited()
}

func (this *Edit) SelectAll() {
	this.SetSelection(0, this.RunesCount())
}

func (this *Edit) SelectionText() string {
	return this.selText(this.sel0, this.sel1)
}

func (this *Edit) Selection() (begin, end int) {
	sel0 := this.sel0
	sel1 := this.sel1
	if sel0 > sel1 {
		t := sel0
		sel0 = sel1
		sel1 = t
	}
	return sel0, sel1
}

func (this *Edit) Select(begin, end int) {
	this.SetSelection(begin, end)
}

func (this *Edit) SetSelection(begin, end int) {
	rc := this.RunesCount()
	if begin > end {
		t := begin
		begin = end
		end = t
	}
	if begin < 0 {
		begin = 0
	} else if begin > rc {
		begin = rc
	}
	if end < begin {
		end = begin
	} else if end > rc {
		end = rc
	}
	this.sel0 = begin
	this.sel1 = end
	this.caretXSaved = false
	//	this.needPrepare()
	this.Self().Update()
}

func (this *Edit) CaretPos() (pos int) {
	return this.sel1
}

func (this *Edit) ScrollToCaret() {
	r, _ := this.CaretRowCol()
	_, sy := this.ScrollPos()
	_, vh := this.ViewportSize()

	rh := this.RowHeight()
	vrs := math.Floor(vh / rh)
	//core.Debug(r, sy, vrs)
	if float64(r) > sy+vrs-1 {
		this.SetScrollY(float64(r) - vrs + 1)
	} else if float64(r) < sy {
		this.SetScrollY(float64(r))
	}

	this.Self().Update()
}

func (this *Edit) ViewportSize() (w, h float64) {
	w, h = this.ScrollArea.ViewportSizePx()
	m := this.padding
	w -= m.L + m.R
	h -= m.B + m.T

	return
}

func (this *Edit) SetCaretPos(pos int) {
	if this.text == nil {
		this.sel0, this.sel1 = 0, 0
		this.Self().Update()
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(this.text)-1 {
		pos = len(this.text) - 1
	}
	this.sel0, this.sel1 = pos, pos
	this.caretXSaved = false
	this.ScrollToCaret()
	this.Self().Update()
}

func (this *Edit) CaretRowCol() (row, col int) {
	return this.PosToRowCol(this.sel1)
}

func (this *Edit) SetCaretRowCol(row, col int) {
	this.SetCaretPos(this.RowColToPos(row, col))
}

func (this *Edit) Layout() {
	this.Update()
	m := this.padding
	w := this.w - m.L - m.R
	h := this.h - m.B - m.T

	if h <= 0 {
		h = 1
	}

	if w <= 0 {
		w = 1
	}

	sw := Theme().ScrollWidth
	this.TextBlock.Layout(w - sw)

	//	width, height := this.Self().Size()

	if !this.ml {
		if this.vs != nil {
			this.vs.Detach()
			this.vs = nil
		}
		this.HorzScrollBar().SetVisible(false)
		return
	}
	rh := this.RowHeight()
	rc := this.SoftRowsCount()
	sh := float64(rc + 1)
	ph := math.Floor(h / rh)
	//core.Debug(rh, rc, sh, h, ph)
	if ph < 1 {
		ph = 1
	}
	vs := this.VertScrollBar()
	//	vs.SetBounds(width-sw, 0, sw, height-sw)
	vs.SetRange(0, sh-ph)
	vs.SetDelta(1, ph)

	//	hs := this.HorzScrollBar()
	//	hs.SetBounds(0, height-sw, width-sw, sw)
	//hs := this.getVScroll()
	//hs.SetRect(width-sw, 0, sw, height-sw)
	//hs.SetRange(0, sh-ph)
	//hs.SetDelta(1, ph)

	//if this.view != nil {
	//	this.view.SetRect(0, 0, width-sw, height-sw)
	//}
	this.ScrollArea.Layout()

}

func (this *Edit) Cursor() *Cursor {
	return cursorIBeam
}

//func (this *Edit) prepare() {
//	if this.text != nil && this.rows == nil {
//		m := Theme().EditMargin
//		w := this.w - m.L - m.R
//		h := this.h - m.B - m.T
//		//		core.Debug("(this *Edit) prepare() ", w, h)
//		fe := this.fontExtents()
//		ln := int((this.sy+h)/fe.Height) + 1
//		this.TextBlock.prepare(w, ln)
//	}
//}

//func (this *Edit) SoftRowsCount() int {
////	this.prepare()
//	return this.TextBlock.SoftRowsCount()
//}

func (this *Edit) SetAlwaysShowSelection(b bool) {
	this.alwaysDrawSel = b
	this.Self().Update()
}
func (this *Edit) AlwaysDrawSelection() bool {
	return this.alwaysDrawSel
}

func (this *Edit) SetReadOnly(b bool) {
	this.readonly = b
	this.Self().Update()
}

func (this *Edit) IsReadOnly() bool {
	return this.readonly
}

func (this *Edit) SizeHints() SizeHints {
	f := this.Font()
	m := this.padding
	h := f.FontExtents().Height + m.T + m.B
	return SizeHints{
		Width:  this.w,
		Height: h,
		Policy: GrowHorizontal | GrowVertical}
}

func (this *Edit) Replace(begin, end int, s string) (caret int, old string) {
	caret, old = this.replace(begin, end, s)
	this.emitChanged()
	return
}

func (this *Edit) replace(begin, end int, s string) (caret int, old string) {
	caret, old = this.TextBlock.Replace(begin, end, s)
	this.sel0, this.sel1 = caret, caret
	this.Layout()
	this.ScrollToCaret()
	this.revalidate()
	return
}

func (this *Edit) SetText(s string) {
	if this.validator != nil && this.validator.Validate(s) == Invalid {
		// Mirrors QLineEdit: programmatic SetText with an Invalid value
		// is a no-op so callers can't bypass the validator. To force the
		// text through, clear the validator first or call ValidatorFixup.
		return
	}
	this.TextBlock.SetText(s)
	this.emitChanged()
	this.Layout()
	this.revalidate()
}

// SetValidator installs (or clears, when nil) the input validator. The
// new validator is applied to existing text only when ValidatorFixup is
// called explicitly — switching validators mid-flight does not
// retroactively reject the current value, matching Qt's behaviour.
func (this *Edit) SetValidator(v Validator) {
	this.validator = v
	// Reclassify the current text against the new validator so IsValid() /
	// the error border reflect it immediately, before any further edit.
	this.revalidate()
}

// Validator returns the currently installed validator, or nil when no
// validator is gating input.
func (this *Edit) Validator() Validator {
	return this.validator
}

// HasAcceptableInput reports whether the current text is in the
// Acceptable state per the installed validator. Returns true when no
// validator is installed (free-form text is always "acceptable").
// Mirrors QLineEdit::hasAcceptableInput; hosts use it to enable / disable
// Submit actions when the user's input is mid-edit (Intermediate).
func (this *Edit) HasAcceptableInput() bool {
	if this.validator == nil {
		return true
	}
	return this.validator.Validate(this.Text()) == Acceptable
}

// ValidatorFixup applies the installed validator's Fixupper, if any,
// to the current text. Typically called on lose-focus so a partial
// "12" auto-completes to "12.00" when DoubleValidator{Decimals: 2} is
// installed. Safe to call when no validator or no Fixupper is set —
// the text is left unchanged.
func (this *Edit) ValidatorFixup() {
	if this.validator == nil {
		return
	}
	fx, ok := this.validator.(Fixupper)
	if !ok {
		return
	}
	fixed := fx.Fixup(this.Text())
	if fixed == this.Text() {
		return
	}
	this.TextBlock.SetText(fixed)
	this.emitChanged()
	this.Layout()
	this.revalidate()
}

// revalidate reclassifies the current buffer against the installed
// validator and refreshes the cached valid / validationError fields. A nil
// validator means "always valid" (free-form text). Called after every text
// mutation (replace, SetText, ValidatorFixup) and when a validator is
// installed, so IsValid() and the error border never read a stale result.
// Pure state — no GL side effects, safe on headless paths.
func (this *Edit) revalidate() {
	if this.validator == nil {
		this.valid = true
		this.validationError = ""
		return
	}
	text := this.Text()
	if this.validator.Validate(text) == Acceptable {
		this.valid = true
		this.validationError = ""
		return
	}
	this.valid = false
	if m, ok := this.validator.(ErrorMessager); ok {
		this.validationError = m.ErrorMessage(text)
	} else {
		this.validationError = "invalid input"
	}
}

// IsValid reports whether the current text satisfies the installed
// validator. True when no validator is installed (free-form text is always
// valid) or the text is Acceptable; false for Intermediate / Invalid. Reads
// the cached result — O(1), no re-validation. A form gates its Submit
// button on this via AllValid.
func (this *Edit) IsValid() bool {
	if this.validator == nil {
		return true
	}
	return this.valid
}

// ValidationError returns the human-readable reason the field is invalid,
// or "" when the field is valid or has no validator. Populated from the
// validator's ErrorMessage when it implements ErrorMessager, else a generic
// "invalid input". Hosts render it beside the field or via ShowToolTip.
func (this *Edit) ValidationError() string {
	if this.validator == nil {
		return ""
	}
	return this.validationError
}

// AllValid reports whether every supplied Edit currently satisfies its
// validator (see Edit.IsValid). A submit handler calls it to gate a form:
// keep OK disabled until AllValid(fields...) is true. Nil entries are
// skipped so callers may pass a sparse slice; zero edits → true.
func AllValid(edits ...*Edit) bool {
	for _, e := range edits {
		if e == nil {
			continue
		}
		if !e.IsValid() {
			return false
		}
	}
	return true
}

func (this *Edit) SetFont(font paint.Font) {
	this.TextBlock.SetFont(font)
	this.Layout()
}

func (this *Edit) SetMultiLine(b bool) {
	this.TextBlock.SetMultiLine(b)
	this.Layout()
}

func (this *Edit) SetWrap(b bool) {
	this.TextBlock.SetWrap(b)
	this.Layout()
}

func (this *Edit) DeleteSelection() {
	if this.sel0 == this.sel1 {
		this.sel1 = this.sel0 + 1
	}
	this.replace(this.sel0, this.sel1, "")
}

func (this *Edit) OnDragEnter(x, y float64, dnd IDndContext) {
	if this.readonly {
		return
	}
	core.Debug("(this *Edit) OnDragEnter")
	if dnd.HasFormat("text/plain") {
		core.Debug("(this *Edit) OnDragEnter: has text/plain")
		dnd.SetAction(DndCopy)
	}
}

//func (this *Edit) OnDragLeave() {

//}

func (this *Edit) OnDragMove(x, y float64, dnd IDndContext) {
	if this.readonly {
		return
	}
	if dnd.HasFormat("text/plain") {
		dnd.SetAction(DndCopy)
	}
}

func (this *Edit) OnDrop(x, y float64, dnd IDndContext) {
	if dnd.HasFormat("text/plain") {
		dnd.SetAction(DndCopy)
	}
}

func (this *Edit) SetPadding(m Padding) {
	this.padding = m
	this.Layout()
}

func (this *Edit) Padding() Padding {
	return this.padding
}

func (this *Edit) NoFrame() bool {
	return this.noFrame
}

func (this *Edit) SetNoFrame(b bool) {
	this.noFrame = b
	this.Update()
}

func (this *Edit) emitEdited() {
	if this.cbTextChanged == nil && this.cbTextEdited == nil {
		return
	}
	str := this.Text()
	if this.cbTextChanged != nil {
		this.cbTextChanged(this.Self(), str)
	}
	if this.cbTextEdited != nil {
		this.cbTextEdited(this.Self(), str)
	}
}

func (this *Edit) SigTextEdited(fn func(interface{}, string)) {
	this.cbTextEdited = fn
}

func (this *Edit) emitChanged() {
	if this.cbTextChanged != nil {
		this.cbTextChanged(this.Self(), this.Text())
	}
}

func (this *Edit) SigTextChanged(fn func(interface{}, string)) {
	this.cbTextChanged = fn
}

func (this *Edit) SigVerify(fn func(interface{}, string) bool) {
	this.cbVerify = fn
}

func (this *Edit) SigSubmit(fn func(interface{}, string)) {
	this.cbSubmit = fn
}

func (this *Edit) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("只读", this.IsReadOnly, this.SetReadOnly)
	list.AddProperty("换行", this.Warp, this.SetWrap)
}

func (this *Edit) Submit() bool {
	if this.cbSubmit == nil {
		return false
	}
	txt := this.Text()
	if this.cbVerify != nil && !this.cbVerify(this.Self(), txt) {
		this.SelectAll()
		return false
	}
	this.cbSubmit(this.Self(), txt)
	return true
}
