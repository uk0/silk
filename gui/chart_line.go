package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
	"math"
)

// LineChartSeries holds one data series for a LineChart.
type LineChartSeries struct {
	Name   string
	Color  paint.Color
	Points []float64 // Y values; X is the index
}

// LineChart renders one or more data series as connected line segments
// inside a coordinate grid.
type LineChart struct {
	Widget
	title      string
	series     []LineChartSeries
	minY, maxY float64
	autoScale  bool
	showGrid   bool
	showLegend bool
	bgColor    paint.Color
	gridColor  paint.Color
	axisColor  paint.Color
}

func init() {
	core.RegisterFactory("gui.LineChart", core.TypeOf((*LineChart)(nil)))
}

// NewLineChart creates a ready-to-use LineChart widget.
func NewLineChart() *LineChart {
	p := new(LineChart)
	p.Init(p)
	p.autoScale = true
	p.showGrid = true
	p.showLegend = true
	p.bgColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
	p.gridColor = paint.Color{R: 220, G: 220, B: 220, A: 255}
	p.axisColor = paint.Color{R: 80, G: 80, B: 80, A: 255}
	p.minY = 0
	p.maxY = 100
	return p
}

// AddSeries appends a named data series.
func (this *LineChart) AddSeries(name string, color paint.Color, data []float64) {
	this.series = append(this.series, LineChartSeries{Name: name, Color: color, Points: data})
	if this.autoScale {
		this.recalcScale()
	}
	this.Self().Update()
}

// ClearSeries removes all series.
func (this *LineChart) ClearSeries() {
	this.series = nil
	this.Self().Update()
}

// SetTitle sets the chart title rendered at the top.
func (this *LineChart) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

// Title returns the current chart title.
func (this *LineChart) Title() string { return this.title }

// SetAutoScale enables or disables automatic Y range computation.
func (this *LineChart) SetAutoScale(b bool) {
	this.autoScale = b
	if b {
		this.recalcScale()
	}
	this.Self().Update()
}

// AutoScale reports whether auto-scaling is active.
func (this *LineChart) AutoScale() bool { return this.autoScale }

// SetShowGrid controls grid line drawing.
func (this *LineChart) SetShowGrid(b bool) {
	this.showGrid = b
	this.Self().Update()
}

// ShowGrid reports whether the grid is drawn.
func (this *LineChart) ShowGrid() bool { return this.showGrid }

// SetShowLegend controls legend drawing.
func (this *LineChart) SetShowLegend(b bool) {
	this.showLegend = b
	this.Self().Update()
}

// ShowLegend reports whether the legend is drawn.
func (this *LineChart) ShowLegend() bool { return this.showLegend }

// SetYRange sets an explicit Y range (disables auto-scale).
func (this *LineChart) SetYRange(min, max float64) {
	this.autoScale = false
	this.minY = min
	this.maxY = max
	this.Self().Update()
}

// EnumProperties exposes inspectable properties.
func (this *LineChart) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("自动缩放", this.AutoScale, this.SetAutoScale)
	list.AddProperty("显示网格", this.ShowGrid, this.SetShowGrid)
	list.AddProperty("显示图例", this.ShowLegend, this.SetShowLegend)
}

// recalcScale recomputes minY/maxY from all series data.
func (this *LineChart) recalcScale() {
	if len(this.series) == 0 {
		return
	}
	lo := math.MaxFloat64
	hi := -math.MaxFloat64
	for _, s := range this.series {
		for _, v := range s.Points {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	if lo == hi {
		lo -= 1
		hi += 1
	}
	pad := (hi - lo) * 0.05
	this.minY = lo - pad
	this.maxY = hi + pad
}

// SizeHints returns the preferred size.
func (this *LineChart) SizeHints() SizeHints {
	return SizeHints{Width: 300, Height: 200, Policy: GrowHorizontal | GrowVertical}
}

// Draw renders the chart.
func (this *LineChart) Draw(g paint.Painter) {
	w, h := this.Size()
	margin := 50.0
	topMargin := 28.0
	bottomMargin := 22.0
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

	chartW := w - margin - rightMargin
	chartH := h - topMargin - bottomMargin
	if chartW <= 0 || chartH <= 0 {
		return
	}

	// Grid
	if this.showGrid {
		g.SetPen1(this.gridColor, 0.5)
		for i := 0; i <= 4; i++ {
			y := topMargin + chartH*float64(i)/4
			g.MoveTo(margin, y)
			g.LineTo(w-rightMargin, y)
			g.Stroke()
		}
	}

	// Axes
	g.SetPen1(this.axisColor, 1)
	g.MoveTo(margin, topMargin)
	g.LineTo(margin, h-bottomMargin)
	g.LineTo(w-rightMargin, h-bottomMargin)
	g.Stroke()

	// Y axis labels
	yRange := this.maxY - this.minY
	if yRange == 0 {
		yRange = 1
	}
	smallFont := paint.NewFont(Theme().Font.Family(), 10, false, false)
	g.SetFont(smallFont)
	for i := 0; i <= 4; i++ {
		val := this.minY + (this.maxY-this.minY)*float64(4-i)/4
		y := topMargin + chartH*float64(i)/4
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(3, y+4, fmt.Sprintf("%.1f", val))
	}

	// Draw each series
	for _, s := range this.series {
		if len(s.Points) < 2 {
			continue
		}
		g.SetPen1(s.Color, 2)

		n := len(s.Points)
		for i, v := range s.Points {
			x := margin + chartW*float64(i)/float64(n-1)
			y := topMargin + chartH*(1-(v-this.minY)/yRange)
			if i == 0 {
				g.MoveTo(x, y)
			} else {
				g.LineTo(x, y)
			}
		}
		g.Stroke()
	}

	// Legend
	if this.showLegend && len(this.series) > 0 {
		lx := w - rightMargin - 110
		ly := topMargin + 5
		for i, s := range this.series {
			g.SetBrush1(s.Color)
			g.Rectangle(lx, ly+float64(i)*16, 12, 12)
			g.Fill()
			g.SetBrush1(Theme().TextColor)
			g.SetFont(smallFont)
			g.DrawText1(lx+16, ly+float64(i)*16+10, s.Name)
		}
	}
}
