package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// SwitchGroup is a group of toggle buttons where only one can be active (segmented control).
type SwitchGroup struct {
	Widget
	items    []string
	selected int
	cbChange func(int, string)
}

func init() {
	core.RegisterFactory("gui.SwitchGroup", core.TypeOf((*SwitchGroup)(nil)))
}

func NewSwitchGroup() *SwitchGroup {
	p := new(SwitchGroup)
	p.Init(p)
	p.selected = 0
	return p
}

func (this *SwitchGroup) Items() []string { return this.items }

func (this *SwitchGroup) SetItems(items []string) {
	this.items = items
	if this.selected >= len(items) {
		this.selected = 0
	}
	this.Self().Update()
}

func (this *SwitchGroup) Selected() int { return this.selected }

func (this *SwitchGroup) SetSelected(idx int) {
	if idx >= 0 && idx < len(this.items) && idx != this.selected {
		this.selected = idx
		this.Self().Update()
	}
}

func (this *SwitchGroup) SelectedText() string {
	if this.selected >= 0 && this.selected < len(this.items) {
		return this.items[this.selected]
	}
	return ""
}

func (this *SwitchGroup) SigChange(fn func(int, string)) {
	this.cbChange = fn
}

// --- Events ---

func (this *SwitchGroup) OnMouseEnter() {
	this.Self().Update()
}

func (this *SwitchGroup) OnMouseLeave() {
	this.Self().Update()
}

func (this *SwitchGroup) OnLeftDown(x, y float64) {
	if len(this.items) == 0 {
		return
	}
	w, _ := this.Size()
	segW := w / float64(len(this.items))
	idx := int(x / segW)
	if idx >= len(this.items) {
		idx = len(this.items) - 1
	}
	if idx < 0 {
		idx = 0
	}
	if idx != this.selected {
		this.selected = idx
		if this.cbChange != nil {
			this.cbChange(idx, this.items[idx])
		}
		this.Self().Update()
	}
}

func (this *SwitchGroup) drawRoundedRect(g paint.Painter, x, y, w, h, r float64) {
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

// --- Drawing ---

func (this *SwitchGroup) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	n := len(this.items)
	if n == 0 {
		g.SetFont(t.Font)
		g.SetBrush1(paint.Color{180, 185, 200, 255})
		g.DrawText1(4, h/2+4, "SwitchGroup")
		return
	}
	r := 4.0
	segW := w / float64(n)

	g.Save()

	// outer rounded border
	this.drawRoundedRect(g, 0, 0, w, h, r)
	g.SetBrush1(t.FormColor)
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	f := t.Font
	g.SetFont(f)

	primaryColor := t.HighLightColor

	for i, text := range this.items {
		sx := float64(i) * segW

		if i == this.selected {
			// selected segment background
			if i == 0 && n == 1 {
				this.drawRoundedRect(g, sx, 0, segW, h, r)
			} else if i == 0 {
				// left rounded, right square
				g.MoveTo(sx+r, 0)
				g.LineTo(sx+segW, 0)
				g.LineTo(sx+segW, h)
				g.LineTo(sx+r, h)
				g.Arc(sx+r, h-r, r, math.Pi/2, math.Pi)
				g.LineTo(sx, r)
				g.Arc(sx+r, r, r, math.Pi, 3*math.Pi/2)
				g.LineTo(sx+r, 0)
			} else if i == n-1 {
				// left square, right rounded
				g.MoveTo(sx, 0)
				g.LineTo(sx+segW-r, 0)
				g.Arc(sx+segW-r, r, r, -math.Pi/2, 0)
				g.LineTo(sx+segW, h-r)
				g.Arc(sx+segW-r, h-r, r, 0, math.Pi/2)
				g.LineTo(sx, h)
				g.LineTo(sx, 0)
			} else {
				g.Rectangle(sx, 0, segW, h)
			}
			g.SetBrush1(primaryColor)
			g.Fill()
		}

		// separator line between segments
		if i > 0 && i != this.selected && i-1 != this.selected {
			g.MoveTo(sx, 4)
			g.LineTo(sx, h-4)
			g.SetPen1(t.BorderColor, 0.5)
			g.Stroke()
		}

		// text
		ext := f.TextExtents(text)
		tx := sx + (segW-ext.Width)/2 - ext.XBearing
		ty := 0.5*(h+ext.YBearing) - ext.YBearing
		if i == this.selected {
			g.SetBrush1(paint.Color{255, 255, 255, 255})
		} else {
			g.SetBrush1(t.TextColor)
		}
		g.Translate(tx, ty)
		g.DrawText(text)
		g.Translate(-tx, -ty)
	}

	g.Restore()
}

func (this *SwitchGroup) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	w := 0.0
	for _, text := range this.items {
		ext := t.Font.TextExtents(text)
		w += ext.Width + 24
	}
	if w < 120 {
		w = 120
	}
	h := fe.Height + 12
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *SwitchGroup) EnumProperties(list core.IPropertyList) {
	list.AddProperty("选中", this.Selected, this.SetSelected)
}
