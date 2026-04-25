package gui

import (
	"silk/core"
	"silk/paint"
	"fmt"
	"math"
)

// PieSlice represents one sector of a PieChart.
type PieSlice struct {
	Label string
	Value float64
	Color paint.Color
}

// PieChart renders data as a circular pie.
type PieChart struct {
	Widget
	title       string
	slices      []PieSlice
	showLabels  bool
	showPercent bool
	bgColor     paint.Color
}

func init() {
	core.RegisterFactory("gui.PieChart", core.TypeOf((*PieChart)(nil)))
}

// NewPieChart creates a ready-to-use PieChart widget.
func NewPieChart() *PieChart {
	p := new(PieChart)
	p.Init(p)
	p.showLabels = true
	p.showPercent = true
	p.bgColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
	return p
}

// AddSlice appends a sector.
func (this *PieChart) AddSlice(label string, value float64, color paint.Color) {
	this.slices = append(this.slices, PieSlice{Label: label, Value: value, Color: color})
	this.Self().Update()
}

// ClearSlices removes all sectors.
func (this *PieChart) ClearSlices() {
	this.slices = nil
	this.Self().Update()
}

// SetTitle sets the chart title.
func (this *PieChart) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

// Title returns the current title.
func (this *PieChart) Title() string { return this.title }

// SetShowLabels controls label drawing.
func (this *PieChart) SetShowLabels(b bool) {
	this.showLabels = b
	this.Self().Update()
}

// ShowLabels reports whether labels are drawn.
func (this *PieChart) ShowLabels() bool { return this.showLabels }

// SetShowPercent controls percentage display.
func (this *PieChart) SetShowPercent(b bool) {
	this.showPercent = b
	this.Self().Update()
}

// ShowPercent reports whether percentages are shown.
func (this *PieChart) ShowPercent() bool { return this.showPercent }

// EnumProperties exposes inspectable properties.
func (this *PieChart) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("显示标签", this.ShowLabels, this.SetShowLabels)
	list.AddProperty("显示百分比", this.ShowPercent, this.SetShowPercent)
}

// SizeHints returns the preferred size.
func (this *PieChart) SizeHints() SizeHints {
	return SizeHints{Width: 250, Height: 250, Policy: GrowHorizontal | GrowVertical}
}

// Draw renders the pie chart.
func (this *PieChart) Draw(g paint.Painter) {
	w, h := this.Size()
	topMargin := 28.0

	// Background
	g.SetBrush1(this.bgColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Title
	if this.title != "" {
		g.SetFont(paint.NewFont(Theme().Font.Family(), 14, true, false))
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(10, 18, this.title)
	}

	n := len(this.slices)
	if n == 0 {
		return
	}

	// Total value
	total := 0.0
	for _, s := range this.slices {
		total += s.Value
	}
	if total == 0 {
		return
	}

	// Center and radius
	availH := h - topMargin - 10
	cx := w / 2
	cy := topMargin + availH/2
	radius := math.Min(w, availH) * 0.38
	if radius < 10 {
		return
	}

	smallFont := paint.NewFont(Theme().Font.Family(), 10, false, false)

	// Draw sectors
	startAngle := -math.Pi / 2 // start at top
	for i, s := range this.slices {
		_ = i
		sweep := 2 * math.Pi * s.Value / total
		endAngle := startAngle + sweep

		// Sector path: center -> arc -> center
		g.MoveTo(cx, cy)
		g.Arc(cx, cy, radius, startAngle, endAngle)
		g.LineTo(cx, cy)
		g.SetBrush1(s.Color)
		g.Fill()

		// Label
		if this.showLabels || this.showPercent {
			midAngle := startAngle + sweep/2
			lx := cx + radius*1.15*math.Cos(midAngle)
			ly := cy + radius*1.15*math.Sin(midAngle)
			label := ""
			if this.showLabels {
				label = s.Label
			}
			if this.showPercent {
				pct := s.Value / total * 100
				if label != "" {
					label += " "
				}
				label += fmt.Sprintf("%.1f%%", pct)
			}
			g.SetFont(smallFont)
			g.SetBrush1(Theme().TextColor)
			g.DrawText1(lx, ly+4, label)
		}

		startAngle = endAngle
	}

	// Optional: draw thin border around each slice for visual separation
	startAngle = -math.Pi / 2
	g.SetPen1(this.bgColor, 1.5)
	for _, s := range this.slices {
		sweep := 2 * math.Pi * s.Value / total
		endAngle := startAngle + sweep
		g.MoveTo(cx, cy)
		g.LineTo(cx+radius*math.Cos(startAngle), cy+radius*math.Sin(startAngle))
		g.Stroke()
		startAngle = endAngle
	}
}
