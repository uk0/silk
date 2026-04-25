package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
)

// BarChartItem represents a single bar.
type BarChartItem struct {
	Label string
	Value float64
	Color paint.Color
}

// BarChart renders vertical or horizontal bars.
type BarChart struct {
	Widget
	title      string
	items      []BarChartItem
	maxValue   float64
	autoScale  bool
	showValues bool
	horizontal bool
	barGap     float64
	bgColor    paint.Color
}

func init() {
	core.RegisterFactory("gui.BarChart", core.TypeOf((*BarChart)(nil)))
}

// NewBarChart creates a ready-to-use BarChart widget.
func NewBarChart() *BarChart {
	p := new(BarChart)
	p.Init(p)
	p.autoScale = true
	p.showValues = true
	p.barGap = 6
	p.bgColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
	p.maxValue = 100
	return p
}

// AddBar appends a bar.
func (this *BarChart) AddBar(label string, value float64, color paint.Color) {
	this.items = append(this.items, BarChartItem{Label: label, Value: value, Color: color})
	if this.autoScale {
		this.recalcScale()
	}
	this.Self().Update()
}

// ClearBars removes all bars.
func (this *BarChart) ClearBars() {
	this.items = nil
	this.Self().Update()
}

// SetTitle sets the chart title.
func (this *BarChart) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

// Title returns the current title.
func (this *BarChart) Title() string { return this.title }

// SetAutoScale enables or disables auto scaling of the value axis.
func (this *BarChart) SetAutoScale(b bool) {
	this.autoScale = b
	if b {
		this.recalcScale()
	}
	this.Self().Update()
}

// AutoScale reports whether auto-scaling is active.
func (this *BarChart) AutoScale() bool { return this.autoScale }

// SetHorizontal switches between vertical and horizontal bars.
func (this *BarChart) SetHorizontal(b bool) {
	this.horizontal = b
	this.Self().Update()
}

// Horizontal reports bar orientation.
func (this *BarChart) Horizontal() bool { return this.horizontal }

// SetShowValues controls value label drawing.
func (this *BarChart) SetShowValues(b bool) {
	this.showValues = b
	this.Self().Update()
}

// ShowValues reports whether value labels are drawn.
func (this *BarChart) ShowValues() bool { return this.showValues }

// SetMaxValue manually sets the maximum value axis.
func (this *BarChart) SetMaxValue(v float64) {
	this.autoScale = false
	this.maxValue = v
	this.Self().Update()
}

// EnumProperties exposes inspectable properties.
func (this *BarChart) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("自动缩放", this.AutoScale, this.SetAutoScale)
	list.AddProperty("水平方向", this.Horizontal, this.SetHorizontal)
	list.AddProperty("显示数值", this.ShowValues, this.SetShowValues)
}

func (this *BarChart) recalcScale() {
	mx := 0.0
	for _, it := range this.items {
		if it.Value > mx {
			mx = it.Value
		}
	}
	if mx == 0 {
		mx = 100
	}
	this.maxValue = mx * 1.1
}

// SizeHints returns the preferred size.
func (this *BarChart) SizeHints() SizeHints {
	return SizeHints{Width: 300, Height: 200, Policy: GrowHorizontal | GrowVertical}
}

// Draw renders the bar chart.
func (this *BarChart) Draw(g paint.Painter) {
	w, h := this.Size()
	margin := 50.0
	topMargin := 28.0
	bottomMargin := 30.0
	rightMargin := 12.0

	// Background
	g.SetBrush1(this.bgColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Title
	if this.title != "" {
		g.SetFont(paint.NewFont(Theme().Font.Family(), 14, true, false))
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(margin, 18, this.title)
	}

	n := len(this.items)
	if n == 0 {
		return
	}

	chartW := w - margin - rightMargin
	chartH := h - topMargin - bottomMargin
	if chartW <= 0 || chartH <= 0 {
		return
	}

	smallFont := paint.NewFont(Theme().Font.Family(), 10, false, false)
	axisColor := paint.Color{R: 80, G: 80, B: 80, A: 255}

	maxV := this.maxValue
	if maxV == 0 {
		maxV = 1
	}

	if this.horizontal {
		// Horizontal bars
		g.SetPen1(axisColor, 1)
		g.MoveTo(margin, topMargin)
		g.LineTo(margin, h-bottomMargin)
		g.Stroke()

		barH := (chartH - float64(n-1)*this.barGap) / float64(n)
		if barH < 2 {
			barH = 2
		}
		for i, it := range this.items {
			y := topMargin + float64(i)*(barH+this.barGap)
			bw := chartW * it.Value / maxV
			g.SetBrush1(it.Color)
			g.Rectangle(margin, y, bw, barH)
			g.Fill()

			// Label
			g.SetFont(smallFont)
			g.SetBrush1(Theme().TextColor)
			g.DrawText1(3, y+barH*0.65, it.Label)

			// Value
			if this.showValues {
				g.DrawText1(margin+bw+4, y+barH*0.65, fmt.Sprintf("%.1f", it.Value))
			}
		}
	} else {
		// Vertical bars
		g.SetPen1(axisColor, 1)
		g.MoveTo(margin, topMargin)
		g.LineTo(margin, h-bottomMargin)
		g.LineTo(w-rightMargin, h-bottomMargin)
		g.Stroke()

		// Y axis labels
		g.SetFont(smallFont)
		gridColor := paint.Color{R: 220, G: 220, B: 220, A: 255}
		for i := 0; i <= 4; i++ {
			val := maxV * float64(4-i) / 4
			y := topMargin + chartH*float64(i)/4
			g.SetPen1(gridColor, 0.5)
			g.MoveTo(margin, y)
			g.LineTo(w-rightMargin, y)
			g.Stroke()
			g.SetBrush1(Theme().TextColor)
			g.DrawText1(3, y+4, fmt.Sprintf("%.0f", val))
		}

		barW := (chartW - float64(n-1)*this.barGap) / float64(n)
		if barW < 2 {
			barW = 2
		}
		for i, it := range this.items {
			x := margin + float64(i)*(barW+this.barGap)
			bh := chartH * it.Value / maxV
			y := h - bottomMargin - bh
			g.SetBrush1(it.Color)
			g.Rectangle(x, y, barW, bh)
			g.Fill()

			// Label below
			g.SetFont(smallFont)
			g.SetBrush1(Theme().TextColor)
			g.DrawText1(x+2, h-bottomMargin+13, it.Label)

			// Value on top
			if this.showValues {
				g.DrawText1(x+2, y-4, fmt.Sprintf("%.1f", it.Value))
			}
		}
	}
}
