package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
	"time"
)

// timeSample is one (time, value) pair held in a rolling series ring buffer.
type timeSample struct {
	T time.Time
	V float64
}

// LineChartSeries holds one data series for a LineChart.
//
// A series is either STATIC (Points, X = index — the original behaviour) or
// ROLLING (a fixed-capacity ring buffer of timeSamples fed via AddSample, X =
// time). Rolling fields are zero-valued and ignored for static series, so the
// static path is unchanged.
type LineChartSeries struct {
	Name   string
	Color  paint.Color
	Points []float64 // Y values; X is the index

	// Rolling (live trend) state — unused unless EnableRolling/AddSample is called.
	rolling  bool
	capacity int          // ring buffer size
	ring     []timeSample // backing store, len == capacity
	rhead    int          // index of the oldest sample
	rcount   int          // number of valid samples (<= capacity)
}

// defaultRollingCapacity is applied when AddSample is called on a series that
// has no rolling buffer yet.
const defaultRollingCapacity = 600

// rollingPalette gives auto-created rolling series a distinct default color.
var rollingPalette = []paint.Color{
	{R: 0x1f, G: 0x77, B: 0xb4, A: 255},
	{R: 0xff, G: 0x7f, B: 0x0e, A: 255},
	{R: 0x2c, G: 0xa0, B: 0x2c, A: 255},
	{R: 0xd6, G: 0x27, B: 0x28, A: 255},
}

// pushSample appends one sample, dropping the oldest once capacity is reached.
func (s *LineChartSeries) pushSample(t time.Time, v float64) {
	if s.capacity <= 0 || len(s.ring) < s.capacity {
		return
	}
	if s.rcount < s.capacity {
		s.ring[(s.rhead+s.rcount)%s.capacity] = timeSample{T: t, V: v}
		s.rcount++
		return
	}
	// Full: overwrite the oldest and advance the head.
	s.ring[s.rhead] = timeSample{T: t, V: v}
	s.rhead = (s.rhead + 1) % s.capacity
}

// orderedSamples returns the live samples oldest-first (a fresh slice).
func (s *LineChartSeries) orderedSamples() []timeSample {
	if s.rcount == 0 {
		return nil
	}
	out := make([]timeSample, s.rcount)
	for i := 0; i < s.rcount; i++ {
		out[i] = s.ring[(s.rhead+i)%s.capacity]
	}
	return out
}

// visibleSamples returns the ordered samples that fall inside the trailing
// window measured back from the newest sample. window <= 0 means "show all".
func (s *LineChartSeries) visibleSamples(window time.Duration) []timeSample {
	all := s.orderedSamples()
	if window <= 0 || len(all) == 0 {
		return all
	}
	cutoff := all[len(all)-1].T.Add(-window)
	start := 0
	for start < len(all) && all[start].T.Before(cutoff) {
		start++
	}
	return all[start:]
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
	timeWindow time.Duration // visible X span for rolling series; 0 = full buffer
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

// seriesIndex returns the index of the series named name, or -1.
func (this *LineChart) seriesIndex(name string) int {
	for i := range this.series {
		if this.series[i].Name == name {
			return i
		}
	}
	return -1
}

// EnableRolling turns a series into a live rolling trend backed by a ring
// buffer holding the most recent capacity (time, value) samples. If the named
// series does not exist yet it is created with a default palette color, so a
// tag-bound chart can EnableRolling then AddSample without a prior AddSeries.
func (this *LineChart) EnableRolling(seriesName string, capacity int) {
	if capacity < 1 {
		capacity = 1
	}
	i := this.seriesIndex(seriesName)
	if i < 0 {
		color := rollingPalette[len(this.series)%len(rollingPalette)]
		this.series = append(this.series, LineChartSeries{Name: seriesName, Color: color})
		i = len(this.series) - 1
	}
	s := &this.series[i]
	s.rolling = true
	s.capacity = capacity
	s.ring = make([]timeSample, capacity)
	s.rhead = 0
	s.rcount = 0
	this.Self().Update()
}

// AddSample appends one (time, value) sample to a rolling series, dropping the
// oldest sample once the ring is full. The series is auto-enabled for rolling
// (default capacity) if AddSample is called before EnableRolling.
func (this *LineChart) AddSample(seriesName string, t time.Time, v float64) {
	i := this.seriesIndex(seriesName)
	if i < 0 || !this.series[i].rolling {
		this.EnableRolling(seriesName, defaultRollingCapacity)
		i = this.seriesIndex(seriesName)
	}
	this.series[i].pushSample(t, v)
	if this.autoScale {
		this.recalcScale()
	}
	this.Self().Update()
}

// SetTimeWindow sets the visible X span for rolling series (newest sample at
// the right edge). A zero or negative duration shows the whole buffer.
func (this *LineChart) SetTimeWindow(d time.Duration) {
	this.timeWindow = d
	this.Self().Update()
}

// TimeWindow returns the current visible X span (0 = full buffer).
func (this *LineChart) TimeWindow() time.Duration { return this.timeWindow }

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
	seen := false
	for i := range this.series {
		s := &this.series[i]
		if s.rolling {
			for _, smp := range s.orderedSamples() {
				if smp.V < lo {
					lo = smp.V
				}
				if smp.V > hi {
					hi = smp.V
				}
				seen = true
			}
			continue
		}
		for _, v := range s.Points {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
			seen = true
		}
	}
	if !seen {
		return
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

// rollingX maps a sample timestamp to its X pixel inside the plot area of a
// rolling trend. The newest sample (rightT) sits at the right edge (left+width)
// and a sample one full window older sits at the left edge (left); samples
// older than the window map to x < left and are clipped by the caller. Pure and
// side-effect free so it can be table-tested without a GL context.
func rollingX(sampleT, rightT time.Time, window time.Duration, left, width float64) float64 {
	if window <= 0 {
		return left + width
	}
	frac := 1 - float64(rightT.Sub(sampleT))/float64(window)
	return left + frac*width
}

// rollingBounds returns the shared time axis for all rolling series: the newest
// sample time across them (right edge) and the visible window. ok is false when
// no rolling series holds a sample.
func (this *LineChart) rollingBounds() (rightT time.Time, window time.Duration, ok bool) {
	var newest, oldest time.Time
	found := false
	for i := range this.series {
		s := &this.series[i]
		if !s.rolling || s.rcount == 0 {
			continue
		}
		all := s.orderedSamples()
		o, n := all[0].T, all[len(all)-1].T
		if !found {
			newest, oldest, found = n, o, true
			continue
		}
		if n.After(newest) {
			newest = n
		}
		if o.Before(oldest) {
			oldest = o
		}
	}
	if !found {
		return time.Time{}, 0, false
	}
	if this.timeWindow > 0 {
		return newest, this.timeWindow, true
	}
	w := newest.Sub(oldest)
	if w <= 0 {
		w = time.Second
	}
	return newest, w, true
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

	// Rolling time axis, shared across all rolling series. rollOK is false for a
	// purely static chart, so the static drawing path below stays byte-identical.
	rightT, window, rollOK := this.rollingBounds()
	var cutoff time.Time
	if rollOK {
		cutoff = rightT.Add(-window)
		// X time tick labels (HH:MM:SS): oldest at left, newest at right.
		g.SetFont(smallFont)
		g.SetBrush1(Theme().TextColor)
		for i := 0; i <= 4; i++ {
			frac := float64(i) / 4
			x := margin + chartW*frac
			tt := rightT.Add(-time.Duration(float64(window) * (1 - frac)))
			g.DrawText1(x-22, h-bottomMargin+14, tt.Format("15:04:05"))
		}
	}

	// Draw each series
	for si := range this.series {
		s := &this.series[si]
		if s.rolling {
			if !rollOK || s.rcount < 2 {
				continue
			}
			g.SetPen1(s.Color, 2)
			first := true
			for _, smp := range s.orderedSamples() {
				if smp.T.Before(cutoff) {
					continue
				}
				x := rollingX(smp.T, rightT, window, margin, chartW)
				y := topMargin + chartH*(1-(smp.V-this.minY)/yRange)
				if first {
					g.MoveTo(x, y)
					first = false
				} else {
					g.LineTo(x, y)
				}
			}
			g.Stroke()
			continue
		}
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
