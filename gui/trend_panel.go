package gui

import (
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// TrendPanel is a historical-trend playback control bar sitting above a live
// LineChart, for SCADA / 组态 trend screens. The panel OWNS the chart: it
// creates a *LineChart, parents it, and lays it out in the area below the
// control bar. The host feeds samples through Chart() (Chart().EnableRolling /
// Chart().AddSample) and reacts to the operator's playback intents through the
// Sig* callbacks.
//
// It is deliberately backend-free. The panel holds only plain view-model state
// (playing / live flags and the selected range index) and never imports the
// historian / playback packages that actually seek, buffer or replay samples.
// A click on a control mutates the local flag and fires the matching Sig
// callback; the host wires that intent to its historian/playback backend and
// drives the chart. This keeps gui light and the panel GL-free testable.
type TrendPanel struct {
	Widget

	chart    *LineChart
	playing  bool
	live     bool
	rangeSel int // index into trendRanges of the active range, -1 when none

	cbPlay  func()
	cbPause func()
	cbMode  func(live bool)
	cbRange func(d time.Duration)
}

// trendRanges is the fixed set of trailing time-window choices offered by the
// range selector, shown left-to-right. Each click emits its Dur via SigRange.
var trendRanges = []struct {
	Label string
	Dur   time.Duration
}{
	{"1m", time.Minute},
	{"10m", 10 * time.Minute},
	{"1h", time.Hour},
}

func init() {
	core.RegisterFactory("gui.TrendPanel", core.TypeOf((*TrendPanel)(nil)))
}

// NewTrendPanel creates a trend panel with an owned, ready-to-feed LineChart.
func NewTrendPanel() *TrendPanel {
	p := new(TrendPanel)
	p.Init(p)
	return p
}

func (this *TrendPanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.live = true // default to live/realtime mode
	this.rangeSel = -1
	this.chart = NewLineChart()
	this.chart.SetParent(this.Self())
	this.Layout()
}

// Chart returns the owned LineChart so the host can feed it samples via the
// chart's existing EnableRolling / AddSample API.
func (this *TrendPanel) Chart() *LineChart { return this.chart }

// IsPlaying reports whether the panel is in the playing state.
func (this *TrendPanel) IsPlaying() bool { return this.playing }

// IsLive reports whether the panel is in live (实时) mode; false is history (历史).
func (this *TrendPanel) IsLive() bool { return this.live }

// SigPlay registers the callback fired when the operator clicks 播放.
func (this *TrendPanel) SigPlay(fn func()) { this.cbPlay = fn }

// SigPause registers the callback fired when the operator clicks 暂停.
func (this *TrendPanel) SigPause(fn func()) { this.cbPause = fn }

// SigModeChanged registers the callback fired when the operator toggles the
// 实时/历史 mode. It receives the new live state.
func (this *TrendPanel) SigModeChanged(fn func(live bool)) { this.cbMode = fn }

// SigRange registers the callback fired when the operator picks a time range.
// It receives the chosen trailing window duration.
func (this *TrendPanel) SigRange(fn func(d time.Duration)) { this.cbRange = fn }

// --- Control-bar geometry (pure, so hit-testing is unit-testable headless) ---

const (
	trendBarH    = 30.0 // control-bar band height
	trendBtnPadY = 4.0  // top/bottom inset of a control inside the bar
	trendBtnW    = 52.0 // play / pause button width
	trendModeW   = 88.0 // mode toggle width
	trendRangeW  = 40.0 // each range-label width
	trendGap     = 6.0  // gap between controls
	trendPadX    = 8.0  // left padding of the first control
)

func trendPlayX() float64  { return trendPadX }
func trendPauseX() float64 { return trendPadX + trendBtnW + trendGap }
func trendModeX() float64  { return trendPadX + 2*(trendBtnW+trendGap) }

// trendRangeX is the left edge of the i-th range label.
func trendRangeX(i int) float64 {
	return trendModeX() + trendModeW + trendGap + float64(i)*(trendRangeW+trendGap)
}

// trendControl identifies which control a hit-test landed on.
type trendControl int

const (
	trendNone trendControl = iota
	trendPlay
	trendPause
	trendMode
	trendRange
)

// controlAt maps a click to a control. For trendRange the second result is the
// index into trendRanges; otherwise it is -1. Pure geometry (only the layout
// constants), so click routing is testable without a window.
func (this *TrendPanel) controlAt(x, y float64) (trendControl, int) {
	if y < 0 || y >= trendBarH {
		return trendNone, -1
	}
	inX := func(x0, w float64) bool { return x >= x0 && x < x0+w }
	switch {
	case inX(trendPlayX(), trendBtnW):
		return trendPlay, -1
	case inX(trendPauseX(), trendBtnW):
		return trendPause, -1
	case inX(trendModeX(), trendModeW):
		return trendMode, -1
	}
	for i := range trendRanges {
		if inX(trendRangeX(i), trendRangeW) {
			return trendRange, i
		}
	}
	return trendNone, -1
}

// --- Events ---

// OnLeftDown routes a click to the control under it: play/pause update the
// playing flag, the mode toggle flips live, a range label selects a window.
// Each fires its matching Sig callback with the current intent. Clicks below
// the control bar fall on the chart, which handles its own input.
func (this *TrendPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	ctl, idx := this.controlAt(x, y)
	switch ctl {
	case trendPlay:
		this.playing = true
		this.Self().Update()
		if this.cbPlay != nil {
			this.cbPlay()
		}
	case trendPause:
		this.playing = false
		this.Self().Update()
		if this.cbPause != nil {
			this.cbPause()
		}
	case trendMode:
		this.live = !this.live
		this.Self().Update()
		if this.cbMode != nil {
			this.cbMode(this.live)
		}
	case trendRange:
		this.rangeSel = idx
		this.Self().Update()
		if this.cbRange != nil {
			this.cbRange(trendRanges[idx].Dur)
		}
	}
}

// --- Layout ---

// Layout keeps the chart filling the panel below the control bar. Called on
// every resize via Widget.OnResize -> ILayout.
func (this *TrendPanel) Layout() {
	if this.chart == nil {
		return
	}
	w, h := this.Self().Size()
	ch := h - trendBarH
	if w < 0 {
		w = 0
	}
	if ch < 0 {
		ch = 0
	}
	this.chart.SetBounds(0, trendBarH, w, ch)
}

func (this *TrendPanel) SizeHints() SizeHints {
	return SizeHints{
		MinWidth:  360,
		MinHeight: 160,
		Policy:    GrowHorizontal | GrowVertical,
	}
}

// --- Drawing ---

// Draw paints the control bar (buttons + active states) using theme colours;
// the owned LineChart draws itself as a child.
func (this *TrendPanel) Draw(g paint.Painter) {
	t := Theme()
	w, _ := this.Size()

	// Control-bar band with a hairline separating it from the chart below.
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, trendBarH)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.Line(0, trendBarH-0.5, w, trendBarH-0.5)
	g.Stroke()

	g.SetFont(t.Font)

	// Play / pause reflect the playing state; the active one is highlighted.
	this.drawControl(g, trendPlayX(), trendBtnW, "播放", this.playing)
	this.drawControl(g, trendPauseX(), trendBtnW, "暂停", !this.playing)

	// Mode toggle shows the current mode and highlights when live.
	modeLabel := "历史"
	if this.live {
		modeLabel = "实时"
	}
	this.drawControl(g, trendModeX(), trendModeW, modeLabel, this.live)

	// Range labels; the selected one is highlighted.
	for i, r := range trendRanges {
		this.drawControl(g, trendRangeX(i), trendRangeW, r.Label, i == this.rangeSel)
	}
}

// drawControl paints one pill-shaped control. active pills fill with the theme
// highlight and use the on-highlight text colour; inactive pills use the view
// background with a hairline border and the normal text colour, so the bar
// reads correctly in both the light and dark IDE themes.
func (this *TrendPanel) drawControl(g paint.Painter, x, wBtn float64, label string, active bool) {
	t := Theme()
	y := trendBtnPadY
	h := trendBarH - trendBtnPadY*2

	roundedRect(g, x, y, wBtn, h, 4)
	if active {
		g.SetBrush1(t.HighLightColor)
		g.FillPreserve()
		g.SetPen1(t.HighLightColor, 1)
		g.Stroke()
		g.SetBrush1(t.MenuActiveTextColor)
	} else {
		g.SetBrush1(t.ViewBGColor)
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
		g.SetBrush1(t.TextColor)
	}

	ext := t.Font.TextExtents(label)
	tx := x + (wBtn-ext.Width)/2 - ext.XBearing
	ty := y + 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx, ty)
	g.DrawText(label)
	g.Translate(-tx, -ty)
}
