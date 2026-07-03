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
	p.color = paint.Color{66, 133, 244, 255} // blue
	return p
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
		// lighter color on hover
		hoverColor := paint.Color{
			R: this.color.R,
			G: this.color.G,
			B: this.color.B + 40,
			A: this.color.A,
		}
		if this.color.B+40 > 255 {
			hoverColor.B = 255
		}
		g.SetBrush1(hoverColor)
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
