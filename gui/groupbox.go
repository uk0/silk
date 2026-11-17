package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.GroupBox", core.TypeOf((*GroupBox)(nil)))
}

// GroupBox is a titled container with an optional content input gate.
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
	caption   groupBoxCaptionCache
}

func NewGroupBox(title string) *GroupBox {
	p := new(GroupBox)
	p.Init(p)
	p.title = title
	return p
}

func (this *GroupBox) Init(self IWidget) {
	this.Widget.Init(self)
	this.checked = true
}

func (this *GroupBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
}

func (this *GroupBox) Title() string {
	return this.title
}

func (this *GroupBox) SetTitle(s string) {
	if this.title == s {
		return
	}
	this.title = s
	this.InvalidateSizeHints()
	this.Self().Update()
}

func (this *GroupBox) SetContent(w IWidget) {
	if this.content != nil && this.content.Parent() == this.Self() {
		this.content.SetParent(nil)
	}
	this.content = w
	if w != nil {
		w.SetParent(this.Self())
	}
	this.applyEnabled()
	this.InvalidateLayout()
}

func (this *GroupBox) Content() IWidget {
	return this.content
}

func (this *GroupBox) AddWidget(iw IWidget) {
	this.SetContent(iw)
}

// SetCheckable turns the title check box on or off. Switching into checkable
// mode applies the current checked state to the content so the enabled state
// is consistent from the start; switching out removes only the group's gate
// and preserves each child's explicitly disabled state.
func (this *GroupBox) SetCheckable(b bool) {
	if this.checkable == b {
		return
	}
	this.checkable = b
	this.applyEnabled()
	this.InvalidateLayout()
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

// IsChildEnabled gates only the currently owned content, leaving the header
// interactive and preserving every child's explicit enabled flag.
func (this *GroupBox) IsChildEnabled(child IWidget) bool {
	if child == nil {
		return false
	}
	return !this.checkable || this.checked || child != this.content || child.Parent() != this.Self()
}

func (this *GroupBox) applyEnabled() {
	if this.content != nil && this.content.Parent() == this.Self() {
		this.content.NakedWidget().InvalidateEnabledState()
	}
}

// titleCheckRect returns the rectangle of the title check box indicator (only
// meaningful when checkable). It is a small rounded square sitting at the left
// of the title row, vertically centred on the title band.
func (this *GroupBox) titleCheckRect() (x, y, size float64) {
	t := Theme()
	size = groupBoxCheckSize(t.IconSize)
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
	height := this.captionMetrics().lineHeight
	if this.checkable {
		height = max(height, groupBoxCheckSize(Theme().IconSize))
	}
	return height + 4
}

// borderPadding returns the inset from the border edge.
func (this *GroupBox) borderPadding() float64 {
	return 6.0
}

func (this *GroupBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()
	if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return
	}
	previousFont := g.Font()
	g.Save()
	defer func() {
		g.Restore()
		// Legacy Cairo Restore keeps Go-side styles. Do not let a caption's
		// font leak into a caller even on that compatibility path.
		g.SetFont(previousFont)
	}()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetFont(t.Font)
	metrics := this.captionMetrics()
	titleH := this.titleHeight()
	pad := this.borderPadding()
	textX := pad + 6.0
	textY := (titleH-metrics.ascent-metrics.descent)*0.5 + metrics.ascent

	// When checkable, reserve room at the left of the title row for the check
	// box indicator and push the title text to its right. checkW stays 0 in the
	// default (non-checkable) path so the layout below is unchanged.
	var checkW float64
	if this.checkable && groupBoxCheckSize(t.IconSize) > 0 {
		_, _, cbSize := this.titleCheckRect()
		checkW = cbSize + this.titleCheckGap()
		textX += checkW
	}
	// Fit only the rendered caption, keeping its authored text and natural
	// hints intact. The gap ends before the rounded right corner.
	text, ext := this.fitCaption(max(0, w-pad-4.5-3-textX))
	gapStart, gapEnd := textX-checkW-3, textX+ext.Width+3
	groupBoxBorderPath(g, pad+0.5, math.Floor(titleH*0.5)+0.5, w-pad-0.5, h-0.5,
		gapStart, gapEnd, text != "" || checkW > 0)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
	// The path already has a gap. A FormColor rectangle here would erase a
	// different parent background, even for an entirely empty caption.

	// Draw the title check box indicator just left of the title text.
	if this.checkable && checkW > 0 {
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
	if text != "" {
		g.DrawText(text)
	}
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
	if !this.checkable || !this.IsEnabled() {
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
	if this.content == nil || this.content.Parent() != this.Self() || !this.content.IsVisible() {
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
	return this.measureGroupWidth(0)
}

func (this *GroupBox) HeightForWidth(width float64) float64 {
	return this.SizeHintsForWidth(width).Height
}

func (this *GroupBox) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(this, width); ok {
		return hint
	}
	return preserveInheritedHints(this, this.measureGroupWidth(width))
}

func (this *GroupBox) measureGroupWidth(width float64) SizeHints {
	titleH := this.titleHeight()
	pad := this.borderPadding()

	minW := 100.0
	if this.title != "" || this.checkable {
		captionW := this.captionMetrics().ext.Width + 2*pad + 13.5
		if this.checkable {
			captionW += groupBoxCheckSize(Theme().IconSize) + this.titleCheckGap()
		}
		minW = max(minW, captionW)
	}
	minH := titleH + pad*2 + 20
	policy := GrowHorizontal | GrowVertical

	if this.content != nil && this.content.Parent() == this.Self() && this.content.IsVisible() {
		innerWidth := 0.0
		if width > 0 {
			innerWidth = max(0, layoutDimension(width)-2*(pad+4))
		}
		hints := MeasureForWidth(this.content, innerWidth)
		cw := hints.Width + 2*(pad+4)
		ch := hints.Height + titleH + 4 + pad + 4
		minW = math.Max(minW, cw)
		minH = math.Max(minH, ch)
		policy |= hints.Policy & ShrinkHorizontal
	}

	return SizeHints{Width: minW, Height: minH, Policy: policy}
}
