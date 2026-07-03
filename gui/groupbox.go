package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.GroupBox", core.TypeOf((*GroupBox)(nil)))
}

// GroupBox is a container widget with a titled border,
// equivalent to QGroupBox in Qt.
type GroupBox struct {
	Widget
	title   string
	content IWidget
	// checkable mode (QGroupBox.setCheckable): when true a small check box is
	// drawn in the title row; toggling it enables/disables the group and its
	// content. The default is not checkable, in which case the box behaves
	// exactly as before. A checkable group defaults to checked (Qt behaviour).
	checkable bool
	checked   bool
	cbToggled func(bool)
}

func NewGroupBox(title string) *GroupBox {
	p := new(GroupBox)
	p.Init(p)
	p.title = title
	p.checked = true // Qt defaults a checkable group to checked / enabled
	return p
}

func (this *GroupBox) Init(self IWidget) {
	this.Widget.Init(self)
}

func (this *GroupBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
}

func (this *GroupBox) Title() string {
	return this.title
}

func (this *GroupBox) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

func (this *GroupBox) SetContent(w IWidget) {
	if this.content != nil {
		this.content.SetParent(nil)
	}
	this.content = w
	if w != nil {
		w.SetParent(this.Self())
	}
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

func (this *GroupBox) Content() IWidget {
	return this.content
}

func (this *GroupBox) AddWidget(iw IWidget) {
	this.SetContent(iw)
}

// SetCheckable turns the title check box on or off. Switching into checkable
// mode applies the current checked state to the content so the enabled state
// is consistent from the start; switching out re-enables the content.
func (this *GroupBox) SetCheckable(b bool) {
	if this.checkable == b {
		return
	}
	this.checkable = b
	if b {
		this.applyEnabled()
	} else if this.content != nil {
		this.content.SetEnabled(true)
	}
	this.Self().Update()
}

func (this *GroupBox) IsCheckable() bool {
	return this.checkable
}

func (this *GroupBox) IsChecked() bool {
	return this.checked
}

// SetChecked sets the checked state. It only does work (propagating enabled
// state, firing SigToggled, repainting) when the value actually changes, so a
// redundant set is a no-op and never re-fires the callback.
func (this *GroupBox) SetChecked(b bool) {
	if this.checked == b {
		return
	}
	this.checked = b
	if this.checkable {
		this.applyEnabled()
	}
	if this.cbToggled != nil {
		this.cbToggled(this.checked)
	}
	this.Self().Update()
}

// SigToggled registers a callback fired whenever the checked state changes
// (QGroupBox::toggled). The new state is passed to the callback.
func (this *GroupBox) SigToggled(fn func(bool)) {
	this.cbToggled = fn
}

// applyEnabled mirrors the checked state onto the content widget so that an
// unchecked group disables its child. No-op when there is no content.
func (this *GroupBox) applyEnabled() {
	if this.content != nil {
		this.content.SetEnabled(this.checked)
	}
}

// titleCheckRect returns the rectangle of the title check box indicator (only
// meaningful when checkable). It is a small rounded square sitting at the left
// of the title row, vertically centred on the title band.
func (this *GroupBox) titleCheckRect() (x, y, size float64) {
	t := Theme()
	size = math.Floor(t.IconSize * 0.75)
	x = this.borderPadding() + 6.0
	y = this.titleHeight()*0.5 - size*0.5
	return
}

// titleCheckGap is the horizontal space between the title check box and the
// title text when checkable.
func (this *GroupBox) titleCheckGap() float64 {
	return 4.0
}

// titleHeight returns the vertical space reserved for the title text.
func (this *GroupBox) titleHeight() float64 {
	t := Theme()
	fe := t.Font.FontExtents()
	return fe.Height + 4
}

// borderPadding returns the inset from the border edge.
func (this *GroupBox) borderPadding() float64 {
	return 6.0
}

func (this *GroupBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()
	titleH := this.titleHeight()
	pad := this.borderPadding()
	halfTitle := titleH * 0.5

	// Draw title text
	g.SetFont(t.Font)
	fe := t.Font.FontExtents()

	titleText := this.title
	if titleText == "" {
		titleText = " "
	}
	ext := t.Font.TextExtents(titleText)

	textX := pad + 6.0
	textY := halfTitle + fe.Ascent*0.5

	// When checkable, reserve room at the left of the title row for the check
	// box indicator and push the title text to its right. checkW stays 0 in the
	// default (non-checkable) path so the layout below is unchanged.
	var checkW float64
	if this.checkable {
		_, _, cbSize := this.titleCheckRect()
		checkW = cbSize + this.titleCheckGap()
		textX += checkW
	}
	// gapStart is the left edge of the border gap / background; it sits left of
	// the indicator when checkable and equals the original textX-3 otherwise.
	gapStart := textX - checkW - 3

	// Draw rounded border, leaving a gap for the title
	r := 4.0 // corner radius
	borderY := halfTitle
	borderH := h - halfTitle

	g.Save()

	// Top-left corner to title gap
	g.MoveTo(gapStart, borderY)
	g.LineTo(pad+r, borderY)
	g.Arc(pad+r, borderY+r, r, -math.Pi/2, math.Pi)
	g.LineTo(pad, borderY+borderH-r)
	g.Arc(pad+r, borderY+borderH-r, r, math.Pi, math.Pi/2)

	// Bottom edge
	g.LineTo(w-pad-r, borderY+borderH)
	g.Arc(w-pad-r, borderY+borderH-r, r, math.Pi/2, 0)

	// Right edge
	g.LineTo(w-pad, borderY+r)
	g.Arc(w-pad-r, borderY+r, r, 0, -math.Pi/2)

	// Top-right to title gap end
	g.LineTo(textX+ext.Width+3, borderY)

	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Draw title text with background fill to create gap effect
	g.Rectangle(gapStart, 0, textX+ext.Width+3-gapStart, titleH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Draw the title check box indicator just left of the title text.
	if this.checkable {
		this.drawTitleCheck(g)
	}

	// Draw the actual title text. When checkable and unchecked, dim the title
	// to signal the disabled state (matching the disabled content).
	if this.checkable && !this.checked {
		g.SetBrush1(t.MenuGrayTextColor)
	} else {
		g.SetBrush1(t.TextColor)
	}
	g.SetFont(t.Font)
	g.Translate(textX-ext.XBearing, textY)
	g.DrawText(this.title)

	g.Restore()
}

// drawTitleCheck renders the small title check box: a rounded square that is
// filled with the highlight colour and carries a check glyph when checked, or
// drawn as an empty outline when unchecked. It mirrors the check-mark idiom
// used by the standard check box without pulling in a full CheckBox.
func (this *GroupBox) drawTitleCheck(g paint.Painter) {
	t := Theme()
	x, y, s := this.titleCheckRect()
	roundedRect(g, x, y, s, s, 3)
	if this.checked {
		g.SetBrush1(t.HighLightColor)
		g.Fill()
		// Check glyph: two strokes forming a tick, inset within the box.
		g.MoveTo(x+s*0.24, y+s*0.52)
		g.LineTo(x+s*0.42, y+s*0.70)
		g.LineTo(x+s*0.76, y+s*0.30)
		g.SetPen1(t.FormColor, 1.5)
		g.Stroke()
	} else {
		g.SetBrush1(t.FormColor)
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
	}
}

// OnLeftDown toggles the group when checkable and the press lands on the title
// row (the check box indicator or the title text beside it). It is a no-op when
// not checkable, so a plain group keeps its previous behaviour of having no
// click handling. Toggling fires SigToggled and takes focus, like a check box.
func (this *GroupBox) OnLeftDown(x, y float64) {
	if !this.checkable {
		return
	}
	if y < 0 || y > this.titleHeight() {
		return
	}
	cbx, _, cbSize := this.titleCheckRect()
	ext := Theme().Font.TextExtents(this.title)
	// Clickable span: from the indicator's left edge to the end of the title.
	right := cbx + cbSize + this.titleCheckGap() + ext.Width
	if x < cbx || x > right {
		return
	}
	this.SetFocus()
	this.SetChecked(!this.checked)
}

func (this *GroupBox) Layout() {
	if this.content == nil {
		return
	}
	titleH := this.titleHeight()
	pad := this.borderPadding()
	w, h := this.Self().Size()

	cx := pad + 4
	cy := titleH + 4
	cw := w - 2*(pad+4)
	ch := h - titleH - 4 - pad - 4

	if cw < 0 {
		cw = 0
	}
	if ch < 0 {
		ch = 0
	}

	this.content.SetBounds(cx, cy, cw, ch)
}

func (this *GroupBox) SizeHints() SizeHints {
	titleH := this.titleHeight()
	pad := this.borderPadding()

	minW := 100.0
	minH := titleH + pad*2 + 20

	if this.content != nil {
		hints := this.content.SizeHints()
		cw := hints.Width + 2*(pad+4)
		ch := hints.Height + titleH + 4 + pad + 4
		minW = math.Max(minW, cw)
		minH = math.Max(minH, ch)
	}

	return SizeHints{Width: minW, Height: minH, Policy: GrowHorizontal | GrowVertical}
}
