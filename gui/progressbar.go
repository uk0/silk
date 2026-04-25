package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
)

// ProgressBar displays a value between 0.0 and 1.0 as a filled bar
type ProgressBar struct {
	Widget
	value    float64
	barColor paint.Color
	bgColor  paint.Color
	showText bool
}

func init() {
	core.RegisterFactory("gui.ProgressBar", core.TypeOf((*ProgressBar)(nil)))
}

func NewProgressBar() *ProgressBar {
	p := new(ProgressBar)
	p.Init(p)
	p.barColor = paint.Color{66, 133, 244, 255}  // blue
	p.bgColor = paint.Color{220, 220, 220, 255}   // light gray
	p.showText = true
	return p
}

func (this *ProgressBar) Value() float64 {
	return this.value
}

func (this *ProgressBar) SetValue(v float64) {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
	}
}

func (this *ProgressBar) SetBarColor(c paint.Color) {
	this.barColor = c
	this.Self().Update()
}

func (this *ProgressBar) BarColor() paint.Color {
	return this.barColor
}

func (this *ProgressBar) SetBgColor(c paint.Color) {
	this.bgColor = c
	this.Self().Update()
}

func (this *ProgressBar) SetShowText(b bool) {
	this.showText = b
	this.Self().Update()
}

func (this *ProgressBar) IsShowText() bool {
	return this.showText
}

func (this *ProgressBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("显示文本", this.IsShowText, this.SetShowText)
}

// --- Drawing ---

func (this *ProgressBar) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(this.bgColor)
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// filled portion
	fw := w * this.value
	if fw > 0 {
		g.Rectangle(0, 0, fw, h)
		g.SetBrush1(this.barColor)
		g.Fill()
	}

	// percentage text
	if this.showText {
		text := fmt.Sprintf("%d%%", int(this.value*100+0.5))
		g.SetFont(t.Font)
		ext := g.Font().TextExtents(text)
		xt := (w-ext.Width)*0.5 - ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.SetBrush1(t.TextColor)
		g.Translate(xt, yt)
		g.DrawText(text)
		g.Translate(-xt, -yt)
	}
}

// --- SizeHints ---

func (this *ProgressBar) SizeHints() SizeHints {
	return SizeHints{Width: 120, Height: 20, Policy: GrowHorizontal | GrowVertical}
}
