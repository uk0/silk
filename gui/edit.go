package gui

import (
	"silk/core"
	//	"silk/factory"
	"silk/paint"
	"math"
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
}

func NewEdit() *Edit {
	p := new(Edit)
	p.Init(p)
	return p
}

func (this *Edit) Init(iw IWidget) {
	this.ScrollArea.Init(iw)
	this.padding = Theme().EditPadding
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
		Theme().DrawEditFrame(g, 0, 0, this.w, this.h,
			iw.HasFocus(), iw.IsHover(), this.readonly)
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
}

func (this *Edit) OnKeyDown(key int, repeat bool) {
	switch key {
	case KeyBackSpace:
		if this.readonly {
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
		r, c := this.CaretRowCol()
		this.SetCaretRowCol(r, c-1)
		this.caretXSaved = false
	case KeyRight:
		r, c := this.CaretRowCol()
		this.SetCaretRowCol(r, c+1)
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
		r, _ := this.CaretRowCol()
		this.SetCaretRowCol(r, 0)
	case KeyEnd:
		r, _ := this.CaretRowCol()
		this.SetCaretRowCol(r, 1<<30)
	case KeyEnter:
		if this.ml && !this.readonly {
			this.OnTextInput(DefultLineEnd)
		} else if !this.ml && !this.readonly {
			this.Submit()
		}
	}
}

func (this *Edit) paste() {
	i, err := Clipboard.Data("text/plain")
	if err == nil {
		this.OnTextInput(i.(string))
	}
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
	h := f.FontExtents().Height + Theme().Margin
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
}

// SetValidator installs (or clears, when nil) the input validator. The
// new validator is applied to existing text only when ValidatorFixup is
// called explicitly — switching validators mid-flight does not
// retroactively reject the current value, matching Qt's behaviour.
func (this *Edit) SetValidator(v Validator) {
	this.validator = v
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
