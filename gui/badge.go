package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
	"math"
)

// Badge 徽标控件，显示数字或小红点
type Badge struct {
	Widget
	count   int
	maxCount int
	dot     bool // 仅显示小红点，不显示数字
	color   paint.Color
	content IWidget
}

func init() {
	core.RegisterFactory("gui.Badge", core.TypeOf((*Badge)(nil)))
}

func NewBadge() *Badge {
	p := new(Badge)
	p.Init(p)
	p.color = paint.Color{239, 68, 68, 255} // red
	p.maxCount = 99
	return p
}

func (this *Badge) Count() int     { return this.count }
func (this *Badge) MaxCount() int  { return this.maxCount }
func (this *Badge) IsDot() bool    { return this.dot }

func (this *Badge) SetCount(n int) {
	this.count = n
	this.Self().Update()
}

func (this *Badge) SetMaxCount(n int) {
	this.maxCount = n
	this.Self().Update()
}

func (this *Badge) SetDot(b bool) {
	this.dot = b
	this.Self().Update()
}

func (this *Badge) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

func (this *Badge) SetContent(w IWidget) {
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

func (this *Badge) Content() IWidget { return this.content }

func (this *Badge) AddWidget(iw IWidget) {
	this.SetContent(iw)
}

func (this *Badge) displayText() string {
	if this.count > this.maxCount {
		return fmt.Sprintf("%d+", this.maxCount)
	}
	return fmt.Sprintf("%d", this.count)
}

// --- Drawing ---

func (this *Badge) Draw(g paint.Painter) {
	w, _ := this.Size()

	// content is drawn by the framework as a child

	if this.count <= 0 && !this.dot {
		// Designer placeholder: draw a small gray circle with "0"
		g.Save()
		r := 8.0
		cx := w - r - 2
		cy := r + 2
		g.Arc(cx, cy, r, 0, 2*math.Pi)
		g.SetBrush1(paint.Color{200, 200, 200, 255})
		g.Fill()
		t := Theme()
		g.SetFont(t.Font)
		g.SetBrush1(paint.Color{255, 255, 255, 255})
		ext := t.Font.TextExtents("0")
		tx := cx - ext.Width/2 - ext.XBearing
		ty := cy - ext.Height/2 - ext.YBearing
		g.Translate(tx, ty)
		g.DrawText("0")
		g.Translate(-tx, -ty)
		g.Restore()
		return
	}

	g.Save()

	if this.dot {
		// small red dot at top-right
		dotR := 4.0
		dotX := w - dotR - 2
		dotY := dotR + 2
		g.Arc(dotX, dotY, dotR, 0, 2*math.Pi)
		g.SetBrush1(this.color)
		g.Fill()
	} else {
		// badge with number
		t := Theme()
		text := this.displayText()
		g.SetFont(t.Font)
		ext := t.Font.TextExtents(text)
		fe := t.Font.FontExtents()

		badgeH := fe.Height + 4
		badgeW := ext.Width + 10
		if badgeW < badgeH {
			badgeW = badgeH
		}
		r := badgeH / 2

		bx := w - badgeW/2 - 2
		by := 2.0

		// rounded pill background
		g.MoveTo(bx-badgeW/2+r, by)
		g.LineTo(bx+badgeW/2-r, by)
		g.Arc(bx+badgeW/2-r, by+r, r, -math.Pi/2, math.Pi/2)
		g.LineTo(bx-badgeW/2+r, by+badgeH)
		g.Arc(bx-badgeW/2+r, by+r, r, math.Pi/2, 3*math.Pi/2)
		g.LineTo(bx-badgeW/2+r, by)
		g.SetBrush1(this.color)
		g.Fill()

		// text
		g.SetBrush1(paint.Color{255, 255, 255, 255})
		tx := bx - ext.Width/2 - ext.XBearing
		ty := by + 0.5*(badgeH+ext.YBearing) - ext.YBearing
		g.Translate(tx, ty)
		g.DrawText(text)
		g.Translate(-tx, -ty)
	}

	g.Restore()
}

func (this *Badge) Layout() {
	if this.content == nil {
		return
	}
	w, h := this.Self().Size()
	this.content.SetBounds(0, 0, w, h)
}

func (this *Badge) SizeHints() SizeHints {
	if this.content != nil {
		hints := this.content.SizeHints()
		return SizeHints{
			Width:  hints.Width + 10,
			Height: hints.Height + 10,
			Policy: hints.Policy,
		}
	}
	return SizeHints{Width: 32, Height: 32, Policy: GrowHorizontal | GrowVertical}
}

func (this *Badge) EnumProperties(list core.IPropertyList) {
	list.AddProperty("数量", this.Count, this.SetCount)
	list.AddProperty("最大数量", this.MaxCount, this.SetMaxCount)
	list.AddProperty("仅圆点", this.IsDot, this.SetDot)
}
