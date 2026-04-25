package gui

import (
	"silk/core"
	"silk/paint"
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
}

func NewGroupBox(title string) *GroupBox {
	p := new(GroupBox)
	p.Init(p)
	p.title = title
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

	// Draw rounded border, leaving a gap for the title
	r := 4.0 // corner radius
	borderY := halfTitle
	borderH := h - halfTitle

	g.Save()

	// Top-left corner to title gap
	g.MoveTo(textX-3, borderY)
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
	g.Rectangle(textX-3, 0, ext.Width+6, titleH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Draw the actual title text
	g.SetBrush1(t.TextColor)
	g.SetFont(t.Font)
	g.Translate(textX-ext.XBearing, textY)
	g.DrawText(this.title)

	g.Restore()
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
