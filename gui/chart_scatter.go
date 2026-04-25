package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
	"math"
)

// ScatterPoint holds one (X, Y) data point.
type ScatterPoint struct {
	X, Y float64
}

// ScatterSeries holds a named set of scatter points.
type ScatterSeries struct {
	Name   string
	Color  paint.Color
	Points []ScatterPoint
	Size   float64 // dot radius (default 3)
}

// ScatterPlot renders one or more series as scattered dots.
type ScatterPlot struct {
	Widget
	title                      string
	series                     []ScatterSeries
	minX, maxX, minY, maxY     float64
	autoScale                  bool
	showGrid                   bool
	showLegend                 bool
	bgColor                    paint.Color
	gridColor                  paint.Color
	axisColor                  paint.Color
}

func init() {
	core.RegisterFactory("gui.ScatterPlot", core.TypeOf((*ScatterPlot)(nil)))
}

// NewScatterPlot creates a ready-to-use ScatterPlot widget.
func NewScatterPlot() *ScatterPlot {
	p := new(ScatterPlot)
	p.Init(p)
	p.autoScale = true
	p.showGrid = true
	p.showLegend = true
	p.bgColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
	p.gridColor = paint.Color{R: 220, G: 220, B: 220, A: 255}
	p.axisColor = paint.Color{R: 80, G: 80, B: 80, A: 255}
	return p
}

// AddSeries appends a scatter series.
func (this *ScatterPlot) AddSeries(name string, color paint.Color, points []ScatterPoint) {
	sz := 3.0
	this.series = append(this.series, ScatterSeries{Name: name, Color: color, Points: points, Size: sz})
	if this.autoScale {
		this.recalcScale()
	}
	this.Self().Update()
}

// AddSeriesWithSize appends a scatter series with a custom dot radius.
func (this *ScatterPlot) AddSeriesWithSize(name string, color paint.Color, points []ScatterPoint, size float64) {
	this.series = append(this.series, ScatterSeries{Name: name, Color: color, Points: points, Size: size})
	if this.autoScale {
		this.recalcScale()
	}
	this.Self().Update()
}

// ClearSeries removes all series.
func (this *ScatterPlot) ClearSeries() {
	this.series = nil
	this.Self().Update()
}

// SetTitle sets the chart title.
func (this *ScatterPlot) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

// Title returns the current title.
func (this *ScatterPlot) Title() string { return this.title }

// SetAutoScale enables or disables automatic axis range computation.
func (this *ScatterPlot) SetAutoScale(b bool) {
	this.autoScale = b
	if b {
		this.recalcScale()
	}
	this.Self().Update()
}

// AutoScale reports whether auto-scaling is active.
func (this *ScatterPlot) AutoScale() bool { return this.autoScale }

// SetShowGrid controls grid line drawing.
func (this *ScatterPlot) SetShowGrid(b bool) {
	this.showGrid = b
	this.Self().Update()
}

// ShowGrid reports whether the grid is drawn.
func (this *ScatterPlot) ShowGrid() bool { return this.showGrid }

// SetShowLegend controls legend drawing.
func (this *ScatterPlot) SetShowLegend(b bool) {
	this.showLegend = b
	this.Self().Update()
}

// ShowLegend reports whether the legend is drawn.
func (this *ScatterPlot) ShowLegend() bool { return this.showLegend }

// SetRange sets explicit axis ranges (disables auto-scale).
func (this *ScatterPlot) SetRange(minX, maxX, minY, maxY float64) {
	this.autoScale = false
	this.minX = minX
	this.maxX = maxX
	this.minY = minY
	this.maxY = maxY
	this.Self().Update()
}

// EnumProperties exposes inspectable properties.
func (this *ScatterPlot) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("自动缩放", this.AutoScale, this.SetAutoScale)
	list.AddProperty("显示网格", this.ShowGrid, this.SetShowGrid)
	list.AddProperty("显示图例", this.ShowLegend, this.SetShowLegend)
}

func (this *ScatterPlot) recalcScale() {
	if len(this.series) == 0 {
		this.minX, this.maxX = 0, 10
		this.minY, this.maxY = 0, 10
		return
	}
	loX, hiX := math.MaxFloat64, -math.MaxFloat64
	loY, hiY := math.MaxFloat64, -math.MaxFloat64
	for _, s := range this.series {
		for _, pt := range s.Points {
			if pt.X < loX {
				loX = pt.X
			}
			if pt.X > hiX {
				hiX = pt.X
			}
			if pt.Y < loY {
				loY = pt.Y
			}
			if pt.Y > hiY {
				hiY = pt.Y
			}
		}
	}
	if loX == hiX {
		loX -= 1
		hiX += 1
	}
	if loY == hiY {
		loY -= 1
		hiY += 1
	}
	padX := (hiX - loX) * 0.05
	padY := (hiY - loY) * 0.05
	this.minX = loX - padX
	this.maxX = hiX + padX
	this.minY = loY - padY
	this.maxY = hiY + padY
}

// SizeHints returns the preferred size.
func (this *ScatterPlot) SizeHints() SizeHints {
	return SizeHints{Width: 300, Height: 200, Policy: GrowHorizontal | GrowVertical}
}

// Draw renders the scatter plot.
func (this *ScatterPlot) Draw(g paint.Painter) {
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

	xRange := this.maxX - this.minX
	yRange := this.maxY - this.minY
	if xRange == 0 {
		xRange = 1
	}
	if yRange == 0 {
		yRange = 1
	}

	smallFont := paint.NewFont(Theme().Font.Family(), 10, false, false)

	// Grid
	if this.showGrid {
		g.SetPen1(this.gridColor, 0.5)
		for i := 0; i <= 4; i++ {
			y := topMargin + chartH*float64(i)/4
			g.MoveTo(margin, y)
			g.LineTo(w-rightMargin, y)
			g.Stroke()
			x := margin + chartW*float64(i)/4
			g.MoveTo(x, topMargin)
			g.LineTo(x, h-bottomMargin)
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
	g.SetFont(smallFont)
	for i := 0; i <= 4; i++ {
		val := this.minY + (this.maxY-this.minY)*float64(4-i)/4
		y := topMargin + chartH*float64(i)/4
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(3, y+4, fmt.Sprintf("%.1f", val))
	}

	// X axis labels
	for i := 0; i <= 4; i++ {
		val := this.minX + (this.maxX-this.minX)*float64(i)/4
		x := margin + chartW*float64(i)/4
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(x-8, h-bottomMargin+14, fmt.Sprintf("%.1f", val))
	}

	// Draw dots
	for _, s := range this.series {
		g.SetBrush1(s.Color)
		r := s.Size
		if r <= 0 {
			r = 3
		}
		for _, pt := range s.Points {
			px := margin + chartW*(pt.X-this.minX)/xRange
			py := topMargin + chartH*(1-(pt.Y-this.minY)/yRange)
			g.Arc(px, py, r, 0, 2*math.Pi)
			g.Fill()
		}
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
