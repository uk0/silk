package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// TimelineItem represents a single step/event on the timeline.
type TimelineItem struct {
	Title    string
	Subtitle string
	Status   int // 0=pending, 1=active, 2=done
}

// Timeline displays a vertical or horizontal sequence of steps/events.
type Timeline struct {
	Widget
	items    []TimelineItem
	vertical bool
}

func init() {
	core.RegisterFactory("gui.Timeline", core.TypeOf((*Timeline)(nil)))
}

func NewTimeline() *Timeline {
	p := new(Timeline)
	p.Init(p)
	p.vertical = true
	return p
}

func (this *Timeline) IsVertical() bool { return this.vertical }

func (this *Timeline) SetVertical(b bool) {
	this.vertical = b
	this.Self().Update()
}

func (this *Timeline) Items() []TimelineItem { return this.items }

func (this *Timeline) SetItems(items []TimelineItem) {
	this.items = items
	this.Self().Update()
}

func (this *Timeline) AddItem(title, subtitle string, status int) {
	this.items = append(this.items, TimelineItem{Title: title, Subtitle: subtitle, Status: status})
	this.Self().Update()
}

func (this *Timeline) SetStatus(idx, status int) {
	if idx >= 0 && idx < len(this.items) {
		this.items[idx].Status = status
		this.Self().Update()
	}
}

func (this *Timeline) statusColor(status int) paint.Color {
	switch status {
	case 2: // done
		return paint.Color{52, 199, 89, 255} // green
	case 1: // active
		return paint.Color{66, 133, 244, 255} // blue
	default: // pending
		return paint.Color{200, 200, 200, 255} // gray
	}
}

// --- Drawing ---

func (this *Timeline) Draw(g paint.Painter) {
	if len(this.items) == 0 {
		t := Theme()
		_, h := this.Size()
		g.SetFont(t.Font)
		g.SetBrush1(paint.Color{180, 185, 200, 255})
		g.DrawText1(4, h/2+4, "Timeline")
		return
	}

	if this.vertical {
		this.drawVertical(g)
	} else {
		this.drawHorizontal(g)
	}
}

func (this *Timeline) drawVertical(g paint.Painter) {
	t := Theme()
	f := t.Font
	g.SetFont(f)
	fe := f.FontExtents()

	circleR := 8.0
	circleX := 20.0
	stepH := 60.0
	textX := circleX + circleR + 12

	g.Save()

	for i, item := range this.items {
		cy := float64(i)*stepH + stepH/2
		color := this.statusColor(item.Status)

		// connecting line to next item
		if i < len(this.items)-1 {
			g.MoveTo(circleX, cy+circleR)
			g.LineTo(circleX, cy+stepH-circleR)
			g.SetPen1(paint.Color{210, 210, 210, 255}, 1)
			g.Stroke()
		}

		// circle
		g.Arc(circleX, cy, circleR, 0, 2*math.Pi)
		g.SetBrush1(color)
		g.Fill()

		// inner dot for active
		if item.Status == 1 {
			g.Arc(circleX, cy, circleR+3, 0, 2*math.Pi)
			g.SetPen1(paint.Color{66, 133, 244, 80}, 2)
			g.Stroke()
		}

		// checkmark for done
		if item.Status == 2 {
			g.SetPen1(paint.Color{255, 255, 255, 255}, 1.5)
			g.MoveTo(circleX-3, cy)
			g.LineTo(circleX-1, cy+3)
			g.Stroke()
			g.MoveTo(circleX-1, cy+3)
			g.LineTo(circleX+4, cy-3)
			g.Stroke()
		}

		// title text
		ext := f.TextExtents(item.Title)
		ty := cy + ext.YBearing/2 - ext.YBearing
		ty = cy - fe.Height/4
		g.SetBrush1(t.TextColor)
		g.Translate(textX-ext.XBearing, ty)
		g.DrawText(item.Title)
		g.Translate(-(textX - ext.XBearing), -ty)

		// subtitle
		if item.Subtitle != "" {
			subExt := f.TextExtents(item.Subtitle)
			subY := cy + fe.Height*0.8
			g.SetBrush1(paint.Color{150, 150, 150, 255})
			g.Translate(textX-subExt.XBearing, subY)
			g.DrawText(item.Subtitle)
			g.Translate(-(textX - subExt.XBearing), -subY)
		}
	}

	g.Restore()

	_ = fe
}

func (this *Timeline) drawHorizontal(g paint.Painter) {
	t := Theme()
	f := t.Font
	g.SetFont(f)
	fe := f.FontExtents()
	_, h := this.Size()

	circleR := 8.0
	circleY := h / 3
	stepW := 100.0

	g.Save()

	for i, item := range this.items {
		cx := float64(i)*stepW + stepW/2
		color := this.statusColor(item.Status)

		// connecting line
		if i < len(this.items)-1 {
			g.MoveTo(cx+circleR, circleY)
			g.LineTo(cx+stepW-circleR, circleY)
			g.SetPen1(paint.Color{210, 210, 210, 255}, 1)
			g.Stroke()
		}

		// circle
		g.Arc(cx, circleY, circleR, 0, 2*math.Pi)
		g.SetBrush1(color)
		g.Fill()

		// active pulse ring
		if item.Status == 1 {
			g.Arc(cx, circleY, circleR+3, 0, 2*math.Pi)
			g.SetPen1(paint.Color{66, 133, 244, 80}, 2)
			g.Stroke()
		}

		// done checkmark
		if item.Status == 2 {
			g.SetPen1(paint.Color{255, 255, 255, 255}, 1.5)
			g.MoveTo(cx-3, circleY)
			g.LineTo(cx-1, circleY+3)
			g.Stroke()
			g.MoveTo(cx-1, circleY+3)
			g.LineTo(cx+4, circleY-3)
			g.Stroke()
		}

		// title below circle
		ext := f.TextExtents(item.Title)
		tx := cx - ext.Width/2 - ext.XBearing
		ty := circleY + circleR + 4 + fe.Height
		g.SetBrush1(t.TextColor)
		g.Translate(tx, ty)
		g.DrawText(item.Title)
		g.Translate(-tx, -ty)

		// subtitle below title
		if item.Subtitle != "" {
			subExt := f.TextExtents(item.Subtitle)
			stx := cx - subExt.Width/2 - subExt.XBearing
			sty := ty + fe.Height + 2
			g.SetBrush1(paint.Color{150, 150, 150, 255})
			g.Translate(stx, sty)
			g.DrawText(item.Subtitle)
			g.Translate(-stx, -sty)
		}
	}

	g.Restore()

	_ = fe
}

func (this *Timeline) SizeHints() SizeHints {
	n := len(this.items)
	if n == 0 {
		n = 1
	}
	if this.vertical {
		return SizeHints{Width: 200, Height: float64(n) * 60, Policy: GrowHorizontal | GrowVertical}
	}
	return SizeHints{Width: float64(n) * 100, Height: 100, Policy: GrowHorizontal | GrowVertical}
}

func (this *Timeline) EnumProperties(list core.IPropertyList) {
	list.AddProperty("垂直", this.IsVertical, this.SetVertical)
}
