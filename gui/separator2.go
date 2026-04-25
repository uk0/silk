package gui

import (
	"silk/core"
	"silk/paint"
)

// LabelSeparator is an enhanced separator with optional label text (e.g. "-- OR --").
type LabelSeparator struct {
	Widget
	text     string
	vertical bool
}

func init() {
	core.RegisterFactory("gui.LabelSeparator", core.TypeOf((*LabelSeparator)(nil)))
}

func NewLabelSeparator(text string) *LabelSeparator {
	p := new(LabelSeparator)
	p.Init(p)
	p.text = text
	return p
}

func (this *LabelSeparator) Text() string { return this.text }

func (this *LabelSeparator) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *LabelSeparator) IsVertical() bool { return this.vertical }

func (this *LabelSeparator) SetVertical(b bool) {
	this.vertical = b
	this.Self().Update()
}

// --- Drawing ---

func (this *LabelSeparator) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	lineColor := paint.Color{210, 210, 210, 255}

	if this.text == "" {
		// plain separator line
		if this.vertical {
			cx := w / 2
			g.MoveTo(cx, 0)
			g.LineTo(cx, h)
		} else {
			cy := h / 2
			g.MoveTo(0, cy)
			g.LineTo(w, cy)
		}
		g.SetPen1(lineColor, 1)
		g.Stroke()
		return
	}

	f := t.Font
	g.SetFont(f)
	ext := f.TextExtents(this.text)

	if this.vertical {
		// vertical: line above, text, line below
		cx := w / 2
		textH := ext.Width // rotated text width becomes height
		gap := 8.0
		textY := (h - textH) / 2

		// top line
		g.MoveTo(cx, 0)
		g.LineTo(cx, textY-gap)
		g.SetPen1(lineColor, 1)
		g.Stroke()

		// text centered
		tx := (w - ext.Width) / 2 - ext.XBearing
		ty := 0.5*(h+ext.YBearing) - ext.YBearing
		g.SetBrush1(paint.Color{150, 150, 150, 255})
		g.Translate(tx, ty)
		g.DrawText(this.text)
		g.Translate(-tx, -ty)

		// bottom line
		g.MoveTo(cx, textY+textH+gap)
		g.LineTo(cx, h)
		g.SetPen1(lineColor, 1)
		g.Stroke()
	} else {
		// horizontal: line - text - line
		cy := h / 2
		gap := 12.0
		textW := ext.Width
		textX := (w - textW) / 2

		// left line
		g.MoveTo(0, cy)
		g.LineTo(textX-gap, cy)
		g.SetPen1(lineColor, 1)
		g.Stroke()

		// text centered
		tx := textX - ext.XBearing
		ty := 0.5*(h+ext.YBearing) - ext.YBearing
		g.SetBrush1(paint.Color{150, 150, 150, 255})
		g.Translate(tx, ty)
		g.DrawText(this.text)
		g.Translate(-tx, -ty)

		// right line
		g.MoveTo(textX+textW+gap, cy)
		g.LineTo(w, cy)
		g.SetPen1(lineColor, 1)
		g.Stroke()
	}
}

func (this *LabelSeparator) SizeHints() SizeHints {
	t := Theme()
	if this.text != "" {
		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.text)
		if this.vertical {
			return SizeHints{Width: ext.Width + 16, Height: 80, Policy: GrowHorizontal | GrowVertical}
		}
		return SizeHints{Width: ext.Width + 60, Height: fe.Height + 8, Policy: GrowHorizontal | GrowVertical}
	}
	sz := Theme().SeparatorSize
	return SizeHints{Height: sz, Width: sz, Policy: GrowVertical | GrowHorizontal}
}

func (this *LabelSeparator) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("垂直", this.IsVertical, this.SetVertical)
}
