package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// Link is a hyperlink label widget.
type Link struct {
	Widget
	text    string
	url     string
	color   paint.Color
	hover   bool
	cbClick func(string)
}

func init() {
	core.RegisterFactory("gui.Link", core.TypeOf((*Link)(nil)))
}

func NewLink(text, url string) *Link {
	p := new(Link)
	p.Init(p)
	p.text = text
	p.url = url
	return p
}

// Init carries the colour, not NewLink: the designer and the .tdoc loader
// build widgets through the core factory, which reflects on Init and never
// sees the constructor. Draw paints the text with this.color, so a zero
// colour draws it fully transparent.
func (this *Link) Init(self IWidget) {
	this.Widget.Init(self)
	this.color = paint.Color{66, 133, 244, 255} // blue
}

// linkHoverColor brightens the link for the hover state. The blue channel
// widens to int before the bump because the old uint8 arithmetic wrapped —
// the default blue (B=244) came out at 28, so hovering darkened the link
// instead of lifting it, and the "> 255" guard could never fire on a uint8.
func linkHoverColor(c paint.Color) paint.Color {
	b := int(c.B) + 40
	if b > 255 {
		b = 255
	}
	c.B = uint8(b)
	return c
}

func (this *Link) Text() string       { return this.text }
func (this *Link) URL() string        { return this.url }
func (this *Link) Color() paint.Color { return this.color }

func (this *Link) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *Link) SetURL(s string) {
	this.url = s
}

func (this *Link) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

func (this *Link) SigClick(fn func(string)) {
	this.cbClick = fn
}

// --- Events ---

func (this *Link) OnMouseEnter() {
	this.hover = true
	this.Self().Update()
}

func (this *Link) OnMouseLeave() {
	this.hover = false
	this.Self().Update()
}

func (this *Link) OnLeftDown(x, y float64) {
	if this.cbClick != nil {
		this.cbClick(this.url)
	}
}

// --- Drawing ---

func (this *Link) Draw(g paint.Painter) {
	t := Theme()
	f := t.Font
	g.SetFont(f)
	_, h := this.Size()

	if this.text == "" {
		// Designer placeholder: show "Link" in blue with underline
		placeholder := "Link"
		ext := f.TextExtents(placeholder)
		fe := f.FontExtents()
		tx := -ext.XBearing
		ty := 0.5*(h+ext.YBearing) - ext.YBearing
		g.SetBrush1(paint.Color{66, 133, 244, 255})
		g.Translate(tx, ty)
		g.DrawText(placeholder)
		g.Translate(-tx, -ty)
		underlineY := ty + fe.Descent
		g.SetPen1(paint.Color{66, 133, 244, 255}, 0.5)
		g.MoveTo(tx, underlineY)
		g.LineTo(tx+ext.Width, underlineY)
		g.Stroke()
		return
	}

	ext := f.TextExtents(this.text)
	fe := f.FontExtents()

	tx := -ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing

	// text color
	if this.hover {
		g.SetBrush1(linkHoverColor(this.color))
	} else {
		g.SetBrush1(this.color)
	}

	// draw text
	g.Translate(tx, ty)
	g.DrawText(this.text)
	g.Translate(-tx, -ty)

	// draw underline
	underlineY := ty + fe.Descent
	if this.hover {
		g.SetPen1(this.color, 1)
	} else {
		g.SetPen1(this.color, 0.5)
	}
	g.MoveTo(tx, underlineY)
	g.LineTo(tx+ext.Width, underlineY)
	g.Stroke()
}

func (this *Link) SizeHints() SizeHints {
	t := Theme()
	f := t.Font
	fe := f.FontExtents()
	ext := f.TextExtents(this.text)
	return SizeHints{Width: ext.Width + 2, Height: fe.Height + 4, Policy: GrowHorizontal | GrowVertical}
}

func (this *Link) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("链接", this.URL, this.SetURL)
}
