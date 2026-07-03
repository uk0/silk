package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignCenter
	AlignRight
)

func init() {
	core.RegisterFactory("gui.Label", core.TypeOf((*Label)(nil)))
}

// Label is a non-editable text display widget.
type Label struct {
	Widget
	text      string
	textColor paint.Color
	font      paint.Font
	align     TextAlign
	wrap      bool

	// SizeHints cache. Inputs: text + font + theme revision.
	// Cache hits avoid 2 cairo extents allocations per Label.
	cachedHints  SizeHints
	hintText     string
	hintFont     paint.Font
	hintThemeRev uint64
	hintsValid   bool
}

func NewLabel(text string) *Label {
	p := new(Label)
	p.Init(p)
	p.text = text
	return p
}

func (this *Label) Text() string {
	return this.text
}

func (this *Label) SetText(s string) {
	if this.text == s {
		return
	}
	this.text = s
	this.hintsValid = false
	this.Self().Update()
}

func (this *Label) SetTextColor(c paint.Color) {
	this.textColor = c
	this.Self().Update()
}

func (this *Label) SetFont(f paint.Font) {
	this.font = f
	this.hintsValid = false
	this.Self().Update()
}

func (this *Label) SetAlign(a TextAlign) {
	if this.align == a {
		return
	}
	this.align = a
	this.Self().Update()
}

func (this *Label) SetWrap(b bool) {
	if this.wrap == b {
		return
	}
	this.wrap = b
	this.Self().Update()
}

func (this *Label) effectiveFont() paint.Font {
	if this.font != nil {
		return this.font
	}
	return Theme().Font
}

func (this *Label) effectiveTextColor() paint.Color {
	if this.textColor.A != 0 {
		return this.textColor
	}
	return Theme().TextColor
}

func (this *Label) Draw(g paint.Painter) {
	if this.text == "" {
		return
	}

	f := this.effectiveFont()
	g.SetFont(f)
	g.SetBrush1(this.effectiveTextColor())

	ext := f.TextExtents(this.text)
	fe := f.FontExtents()
	w, h := this.Self().Size()

	var xt float64
	switch this.align {
	case AlignLeft:
		xt = -ext.XBearing
	case AlignCenter:
		xt = (w-ext.Width)*0.5 - ext.XBearing
	case AlignRight:
		xt = w - ext.Width - ext.XBearing
	}

	yt := (h+fe.Ascent-fe.Descent)*0.5 - fe.Descent
	yt -= ext.YBearing + fe.Ascent - fe.Descent
	yt = 0.5*(h+ext.YBearing) - ext.YBearing

	g.Translate(xt, yt)
	g.DrawText(this.text)
	g.Translate(-xt, -yt)
}

func (this *Label) SizeHints() SizeHints {
	f := this.effectiveFont()

	// Fast path: cache key is (text, font pointer, theme revision). The text
	// string is compared by value (cheap when interned literals), font by
	// pointer identity. Theme revision catches font/style changes that the
	// label inherits from Theme().
	if this.hintsValid &&
		this.hintText == this.text &&
		this.hintFont == f &&
		this.hintThemeRev == themeRev {
		return this.cachedHints
	}

	fe := f.FontExtents()
	ext := f.TextExtents(this.text)
	hints := SizeHints{Width: ext.Width, Height: fe.Height, Policy: GrowHorizontal | GrowVertical}

	this.hintText = this.text
	this.hintFont = f
	this.hintThemeRev = themeRev
	this.cachedHints = hints
	this.hintsValid = true
	return hints
}

func (this *Label) Align() TextAlign {
	return this.align
}

func (this *Label) Wrap() bool {
	return this.wrap
}

func (this *Label) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("对齐", this.Align, this.SetAlign)
	list.AddProperty("换行", this.Wrap, this.SetWrap)
}
