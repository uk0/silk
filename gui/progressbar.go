package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
	"time"
)

// indeterminateCycle is the period of one full back-and-forth sweep of
// the marquee chunk. 1.2s reads as "working" without looking frantic.
const indeterminateCycle = 1200 * time.Millisecond

// ProgressBar displays a value between 0.0 and 1.0 as a filled bar.
//
// In indeterminate ("busy") mode the value is ignored and the bar
// instead shows a small chunk sliding back and forth across the track
// to signal ongoing work of unknown duration (cf. Qt's QProgressBar
// indeterminate state). A looping heartbeat Animation keeps
// HasActiveAnimations() true so the window event loop keeps redrawing
// while busy — the same idiom Spinner uses.
type ProgressBar struct {
	Widget
	value    float64
	barColor paint.Color
	bgColor  paint.Color
	showText bool

	indeterminate bool
	startTime     time.Time
	anim          *Animation
}

func init() {
	core.RegisterFactory("gui.ProgressBar", core.TypeOf((*ProgressBar)(nil)))
}

func NewProgressBar() *ProgressBar {
	p := new(ProgressBar)
	p.Init(p)
	p.barColor = paint.Color{66, 133, 244, 255} // blue
	p.bgColor = paint.Color{220, 220, 220, 255} // light gray
	p.showText = true
	return p
}

func (this *ProgressBar) Value() float64 {
	return this.value
}

func (this *ProgressBar) SetValue(v float64) {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
	}
}

func (this *ProgressBar) SetBarColor(c paint.Color) {
	this.barColor = c
	this.Self().Update()
}

func (this *ProgressBar) BarColor() paint.Color {
	return this.barColor
}

func (this *ProgressBar) SetBgColor(c paint.Color) {
	this.bgColor = c
	this.Self().Update()
}

func (this *ProgressBar) SetShowText(b bool) {
	this.showText = b
	this.Self().Update()
}

func (this *ProgressBar) IsShowText() bool {
	return this.showText
}

// SetIndeterminate toggles "busy" mode. When turning on, it records the
// phase start time and arms a looping heartbeat Animation so
// HasActiveAnimations() reports true and the window keeps ticking;
// turning off stops the heartbeat so the event loop can idle. The value
// path is untouched — flipping back to determinate resumes the previous
// fill exactly.
func (this *ProgressBar) SetIndeterminate(on bool) {
	if this.indeterminate == on {
		return
	}
	this.indeterminate = on
	if on {
		this.startTime = time.Now()
		this.startAnim()
	} else {
		this.stopAnim()
	}
	this.Self().Update()
}

// IsIndeterminate reports whether the bar is in busy mode.
func (this *ProgressBar) IsIndeterminate() bool {
	return this.indeterminate
}

// startAnim arms a looping heartbeat Animation if none is running. It
// does no interpolation — its only job is to keep
// HasActiveAnimations() true so the event loop stays in redraw mode
// while busy. Draw computes the chunk position from elapsed time, not
// from the animation value, so the sweep is tick-rate independent.
func (this *ProgressBar) startAnim() {
	if this.anim != nil && this.anim.State() == AnimRunning {
		return
	}
	this.anim = NewAnimation(0, 1, indeterminateCycle).SetLoop(true).SetEase(EaseLinear)
	this.anim.Start()
}

// stopAnim ends the heartbeat so HasActiveAnimations can drop to false.
// Idempotent — safe to call when already stopped.
func (this *ProgressBar) stopAnim() {
	if this.anim == nil {
		return
	}
	this.anim.Stop()
	this.anim = nil
}

// marqueeOffset returns the left x of the sliding chunk for a ping-pong
// sweep across a track of trackWidth, given a chunk of chunkWidth and
// the elapsed/cycle times (seconds). At phase 0 the chunk sits at the
// left edge (0); at the half-cycle it sits at the right edge
// (trackWidth - chunkWidth); a full cycle returns to 0. The result is
// clamped to [0, trackWidth-chunkWidth] so the chunk never overruns the
// track. Pure (no clock, no GL) so it's unit-testable.
func marqueeOffset(elapsedSeconds, cycleSeconds, trackWidth, chunkWidth float64) float64 {
	span := trackWidth - chunkWidth
	if span <= 0 || cycleSeconds <= 0 {
		return 0
	}
	// Fractional position in the cycle, then a triangle wave 0→1→0 so
	// the chunk travels left→right→left.
	phase := math.Mod(elapsedSeconds/cycleSeconds, 1.0)
	tri := phase * 2
	if tri > 1 {
		tri = 2 - tri
	}
	x := tri * span
	if x < 0 {
		x = 0
	} else if x > span {
		x = span
	}
	return x
}

func (this *ProgressBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("显示文本", this.IsShowText, this.SetShowText)
	list.AddProperty("忙碌", this.IsIndeterminate, this.SetIndeterminate)
}

// --- Drawing ---

func (this *ProgressBar) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(this.bgColor)
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// indeterminate: slide a chunk across the track and skip the
	// value fill / percentage text entirely.
	if this.indeterminate {
		cw := w * 0.3 // chunk is ~30% of the track
		if cw > 0 && w > 0 {
			elapsed := time.Since(this.startTime).Seconds()
			x := marqueeOffset(elapsed, indeterminateCycle.Seconds(), w, cw)
			g.Rectangle(x, 0, cw, h)
			g.SetBrush1(this.barColor)
			g.Fill()
		}
		return
	}

	// filled portion
	fw := w * this.value
	if fw > 0 {
		g.Rectangle(0, 0, fw, h)
		g.SetBrush1(this.barColor)
		g.Fill()
	}

	// percentage text
	if this.showText {
		text := fmt.Sprintf("%d%%", int(this.value*100+0.5))
		g.SetFont(t.Font)
		ext := g.Font().TextExtents(text)
		xt := (w-ext.Width)*0.5 - ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.SetBrush1(t.TextColor)
		g.Translate(xt, yt)
		g.DrawText(text)
		g.Translate(-xt, -yt)
	}
}

// --- SizeHints ---

func (this *ProgressBar) SizeHints() SizeHints {
	return SizeHints{Width: 120, Height: 20, Policy: GrowHorizontal | GrowVertical}
}
