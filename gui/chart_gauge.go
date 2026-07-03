package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// GaugeZone defines a colored arc region on a Gauge.
type GaugeZone struct {
	Start, End float64
	Color      paint.Color
}

// Gauge renders a semi-circular meter with a needle.
type Gauge struct {
	Widget
	value    float64 // current value
	min, max float64
	title    string
	unit     string // e.g. "°C", "%", "RPM"
	zones    []GaugeZone
	bgColor  paint.Color
}

func init() {
	core.RegisterFactory("gui.Gauge", core.TypeOf((*Gauge)(nil)))
}

// NewGauge creates a ready-to-use Gauge widget.
func NewGauge() *Gauge {
	p := new(Gauge)
	p.Init(p)
	p.min = 0
	p.max = 100
	p.bgColor = paint.Color{R: 255, G: 255, B: 255, A: 255}
	return p
}

// SetValue sets the needle position.
func (this *Gauge) SetValue(v float64) {
	if v < this.min {
		v = this.min
	}
	if v > this.max {
		v = this.max
	}
	this.value = v
	this.Self().Update()
}

// Value returns the current value.
func (this *Gauge) Value() float64 { return this.value }

// SetRange sets the minimum and maximum values.
func (this *Gauge) SetRange(min, max float64) {
	this.min = min
	this.max = max
	this.Self().Update()
}

// SetTitle sets the gauge title.
func (this *Gauge) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

// Title returns the current title.
func (this *Gauge) Title() string { return this.title }

// SetUnit sets the value unit string.
func (this *Gauge) SetUnit(s string) {
	this.unit = s
	this.Self().Update()
}

// Unit returns the current unit.
func (this *Gauge) Unit() string { return this.unit }

// AddZone adds a colored arc region.
func (this *Gauge) AddZone(start, end float64, color paint.Color) {
	this.zones = append(this.zones, GaugeZone{Start: start, End: end, Color: color})
	this.Self().Update()
}

// ClearZones removes all zones.
func (this *Gauge) ClearZones() {
	this.zones = nil
	this.Self().Update()
}

// EnumProperties exposes inspectable properties.
func (this *Gauge) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("单位", this.Unit, this.SetUnit)
}

// SizeHints returns the preferred size.
func (this *Gauge) SizeHints() SizeHints {
	return SizeHints{Width: 220, Height: 150, Policy: GrowHorizontal | GrowVertical}
}

// Draw renders the gauge.
func (this *Gauge) Draw(g paint.Painter) {
	w, h := this.Size()

	// Background
	g.SetBrush1(this.bgColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Title
	if this.title != "" {
		g.SetFont(paint.NewFont(Theme().Font.Family(), 12, true, false))
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(10, 16, this.title)
	}

	cx := w / 2
	cy := h * 0.68
	radius := math.Min(w, h) * 0.40
	if radius < 10 {
		return
	}

	rng := this.max - this.min
	if rng == 0 {
		rng = 1
	}

	outerR := radius
	innerR := radius * 0.70

	// Draw colored zones as thick arcs
	for _, z := range this.zones {
		startFrac := (z.Start - this.min) / rng
		endFrac := (z.End - this.min) / rng
		// Clamp to [0,1]
		if startFrac < 0 {
			startFrac = 0
		}
		if endFrac > 1 {
			endFrac = 1
		}
		// Map fraction to angle: pi (left) to 0 (right)
		a1 := math.Pi - endFrac*math.Pi
		a2 := math.Pi - startFrac*math.Pi

		// Outer arc from a1 to a2
		g.MoveTo(cx+outerR*math.Cos(a1), cy-outerR*math.Sin(a1))
		g.Arc(cx, cy, outerR, -a1, -a2)
		// Inner arc backwards from a2 to a1
		g.LineTo(cx+innerR*math.Cos(a2), cy-innerR*math.Sin(a2))
		g.ArcNegative(cx, cy, innerR, -a2, -a1)
		g.LineTo(cx+outerR*math.Cos(a1), cy-outerR*math.Sin(a1))
		g.SetBrush1(z.Color)
		g.Fill()
	}

	// If no zones, draw a default gray arc
	if len(this.zones) == 0 {
		g.SetPen1(paint.Color{R: 200, G: 200, B: 200, A: 255}, (outerR-innerR)*0.8)
		g.Arc(cx, cy, (outerR+innerR)/2, -math.Pi, 0)
		g.Stroke()
	}

	// Tick marks
	g.SetPen1(paint.Color{R: 80, G: 80, B: 80, A: 255}, 1)
	smallFont := paint.NewFont(Theme().Font.Family(), 9, false, false)
	for i := 0; i <= 10; i++ {
		frac := float64(i) / 10
		angle := math.Pi - frac*math.Pi
		ox := cx + outerR*1.02*math.Cos(angle)
		oy := cy - outerR*1.02*math.Sin(angle)
		ix := cx + outerR*0.92*math.Cos(angle)
		iy := cy - outerR*0.92*math.Sin(angle)
		g.MoveTo(ox, oy)
		g.LineTo(ix, iy)
		g.Stroke()

		if i%2 == 0 {
			val := this.min + rng*frac
			lx := cx + outerR*1.12*math.Cos(angle) - 10
			ly := cy - outerR*1.12*math.Sin(angle) + 4
			g.SetFont(smallFont)
			g.SetBrush1(Theme().TextColor)
			g.DrawText1(lx, ly, fmt.Sprintf("%.0f", val))
		}
	}

	// Needle
	frac := (this.value - this.min) / rng
	angle := math.Pi - frac*math.Pi
	nx := cx + outerR*0.85*math.Cos(angle)
	ny := cy - outerR*0.85*math.Sin(angle)
	g.SetPen1(paint.Color{R: 220, G: 50, B: 50, A: 255}, 2)
	g.MoveTo(cx, cy)
	g.LineTo(nx, ny)
	g.Stroke()

	// Center dot
	g.SetBrush1(paint.Color{R: 60, G: 60, B: 60, A: 255})
	g.Arc(cx, cy, 5, 0, 2*math.Pi)
	g.Fill()

	// Value text
	g.SetFont(paint.NewFont(Theme().Font.Family(), 16, true, false))
	g.SetBrush1(Theme().TextColor)
	text := fmt.Sprintf("%.1f%s", this.value, this.unit)
	g.DrawText1(cx-20, cy+22, text)

	// Min / Max labels
	g.SetFont(smallFont)
	g.SetBrush1(paint.Color{R: 120, G: 120, B: 120, A: 255})
	g.DrawText1(cx-outerR-5, cy+14, fmt.Sprintf("%.0f", this.min))
	g.DrawText1(cx+outerR-10, cy+14, fmt.Sprintf("%.0f", this.max))
}
