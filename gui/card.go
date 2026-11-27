package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// Card 卡片容器控件，带圆角边框和可选标题
type Card struct {
	Widget
	title   string
	content IWidget
	padding float64
	radius  float64
	shadow  bool
}

func init() {
	core.RegisterFactory("gui.Card", core.TypeOf((*Card)(nil)))
}

func NewCard(title string) *Card {
	p := new(Card)
	p.Init(p)
	p.title = title
	p.padding = 12
	p.radius = 8
	p.shadow = true
	return p
}

func (this *Card) Title() string    { return this.title }
func (this *Card) Padding() float64 { return this.padding }
func (this *Card) Radius() float64  { return this.radius }
func (this *Card) HasShadow() bool  { return this.shadow }

func (this *Card) SetTitle(s string) {
	if this.title == s {
		return
	}
	this.title = s
	this.InvalidateLayout()
}

func (this *Card) SetPadding(v float64) {
	if this.padding == v {
		return
	}
	this.padding = v
	this.InvalidateLayout()
}

func (this *Card) SetRadius(v float64) {
	this.radius = v
	this.Self().Update()
}

func (this *Card) SetShadow(b bool) {
	this.shadow = b
	this.Self().Update()
}

func (this *Card) SetContent(w IWidget) {
	if !setContentSlot(this.Self(), &this.content, w) {
		return
	}
	this.InvalidateLayout()
}

func (this *Card) releaseOwnedChild(w IWidget) {
	if this.content == w {
		this.content = nil
	}
}

func (this *Card) Content() IWidget {
	return this.content
}

func (this *Card) AddWidget(iw IWidget) {
	this.SetContent(iw)
}

func (this *Card) titleHeight() float64 {
	if this.title == "" {
		return 0
	}
	t := Theme()
	fe := t.Font.FontExtents()
	return fe.Height + 12
}

// --- Drawing ---

func (this *Card) drawRoundedRect(g paint.Painter, x, y, w, h, r float64) {
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(x+r, y)
}

func (this *Card) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	r := this.radius

	g.Save()

	if this.shadow {
		paint.DrawShadowRect(g, 0, 0, w, h, r, 4, paint.Color{0, 0, 0, 80})
	}

	// main card background
	this.drawRoundedRect(g, 0, 0, w, h, r)
	g.SetBrush1(t.ViewBGColor)
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// title bar
	titleH := this.titleHeight()
	if this.title == "" && this.child == nil {
		// Designer placeholder
		g.SetFont(t.Font)
		g.SetBrush1(paint.Color{180, 185, 200, 255})
		g.DrawText1(this.padding, h/2+4, "Card")
	}
	if this.title != "" {
		// title background
		g.MoveTo(r, 0)
		g.LineTo(w-r, 0)
		g.Arc(w-r, r, r, -math.Pi/2, 0)
		g.LineTo(w, titleH)
		g.LineTo(0, titleH)
		g.LineTo(0, r)
		g.Arc(r, r, r, math.Pi, 3*math.Pi/2)
		g.LineTo(r, 0)
		g.SetBrush1(t.FormColor)
		g.Fill()

		// title separator line
		g.MoveTo(0, titleH)
		g.LineTo(w, titleH)
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

		// title text
		g.SetFont(t.Font)
		ext := t.Font.TextExtents(this.title)
		tx := this.padding
		ty := 0.5*(titleH+ext.YBearing) - ext.YBearing
		g.SetBrush1(t.TextColor)
		g.Translate(tx-ext.XBearing, ty)
		g.DrawText(this.title)
		g.Translate(-(tx - ext.XBearing), -ty)
	}

	g.Restore()
}

// ContentBounds returns the current local client rectangle of the content
// slot in logical pixels, excluding the title band and padding.
func (this *Card) ContentBounds() geom.Rect {
	pad := this.padding
	titleH := this.titleHeight()
	w, h := this.Self().Size()
	return geom.Rect{X: pad, Y: titleH + pad, Width: max(0, w-2*pad), Height: max(0, h-titleH-2*pad)}
}

func (this *Card) Layout() {
	if this.content != nil {
		this.content.SetBounds1(this.ContentBounds())
	}
}

func (this *Card) SizeHints() SizeHints {
	pad := this.padding
	titleH := this.titleHeight()
	minW := 120.0
	minH := titleH + pad*2 + 40

	if this.content != nil {
		hints := this.content.SizeHints()
		cw := hints.Width + pad*2
		ch := hints.Height + titleH + pad*2
		minW = math.Max(minW, cw)
		minH = math.Max(minH, ch)
	}

	return SizeHints{Width: minW, Height: minH, Policy: GrowHorizontal | GrowVertical}
}

func (this *Card) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("内边距", this.Padding, this.SetPadding)
	list.AddProperty("圆角", this.Radius, this.SetRadius)
	list.AddProperty("阴影", this.HasShadow, this.SetShadow)
}
