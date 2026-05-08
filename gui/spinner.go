package gui

import (
	"math"
	"time"

	"silk/core"
	"silk/paint"
)

func init() {
	core.RegisterFactory("gui.Spinner", core.TypeOf((*Spinner)(nil)))
}

// Spinner is an animated busy indicator: eight dots arranged on a
// circle, with the "current" dot at full alpha and trailing dots
// fading toward transparent. The pattern rotates once per
// CycleDuration so the user perceives the widget as alive.
//
// Usage:
//
//	sp := gui.NewSpinner()
//	sp.SetSize(24, 24)
//	parent.AddChild(sp)
//	// later, when the long-running op finishes:
//	sp.SetBusy(false)
//
// Spinners draw nothing while !busy, so it's safe to leave one in
// the layout permanently and just toggle SetBusy(true/false) when an
// operation runs.
//
// Internally a single looping Animation drives redraws — its
// onUpdate is a no-op, but its presence makes
// gui.HasActiveAnimations() report true so the window event loop
// keeps ticking. The phase that Draw uses is computed straight from
// time.Since(startTime), so the visual is independent of the
// animation tick rate (it stays smooth even if AnimationTick lands
// on irregular intervals).
type Spinner struct {
	Widget

	color    paint.Color
	dotCount int
	cycle    time.Duration

	busy      bool
	startTime time.Time
	anim      *Animation
}

// NewSpinner creates a spinner with sensible defaults: theme accent
// colour, 8 dots, 1-second rotation cycle. The widget starts in the
// busy state so adding it to a layout immediately animates — call
// SetBusy(false) afterwards if you want to start hidden.
func NewSpinner() *Spinner {
	s := new(Spinner)
	s.Init(s)
	s.color = Theme().HighLightColor
	s.dotCount = 8
	s.cycle = time.Second
	s.busy = true
	s.startTime = time.Now()
	s.startAnim()
	return s
}

// SetColor overrides the dot colour. Defaults to the theme accent so
// most callers shouldn't need this — it exists for spinners on
// non-default surface colours (e.g. dark dialogs) where the theme
// accent provides poor contrast.
func (this *Spinner) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

// Color returns the configured dot colour.
func (this *Spinner) Color() paint.Color {
	return this.color
}

// SetDotCount changes the number of dots on the circle. 8 is the
// default — fewer dots look chunkier, more dots look smoother. Below
// 3 the visual collapses to a single travelling dot.
func (this *Spinner) SetDotCount(n int) {
	if n < 3 {
		n = 3
	}
	if n == this.dotCount {
		return
	}
	this.dotCount = n
	this.Self().Update()
}

// DotCount returns the current dot count.
func (this *Spinner) DotCount() int {
	return this.dotCount
}

// SetCycleDuration sets how long one full rotation takes. Default
// 1 second; smaller values look more urgent, larger values calmer.
// Negative or zero falls back to 1 second so Draw never divides by
// zero when computing phase.
func (this *Spinner) SetCycleDuration(d time.Duration) {
	if d <= 0 {
		d = time.Second
	}
	this.cycle = d
}

// CycleDuration returns the rotation period.
func (this *Spinner) CycleDuration() time.Duration {
	return this.cycle
}

// IsBusy reports whether the spinner is currently animating.
func (this *Spinner) IsBusy() bool {
	return this.busy
}

// SetBusy toggles the animation. true starts the rotation and resets
// the phase so the spinner doesn't appear to jump when reactivated;
// false stops the underlying Animation so the window event loop can
// idle when nothing else needs ticking.
func (this *Spinner) SetBusy(b bool) {
	if this.busy == b {
		return
	}
	this.busy = b
	if b {
		this.startTime = time.Now()
		this.startAnim()
	} else {
		this.stopAnim()
	}
	this.Self().Update()
}

// startAnim spins up a heartbeat Animation if none is active. The
// animation does no actual interpolation work — its sole purpose is
// to register with animManager so HasActiveAnimations() returns true
// and the window event loop stays in active-redraw mode while the
// spinner is busy.
func (this *Spinner) startAnim() {
	if this.anim != nil && this.anim.State() == AnimRunning {
		return
	}
	this.anim = NewAnimation(0, 1, this.cycle).SetLoop(true).SetEase(EaseLinear)
	this.anim.Start()
}

// stopAnim ends the heartbeat so HasActiveAnimations can drop to
// false (assuming no other widget is animating). Safe to call
// multiple times — each call is idempotent.
func (this *Spinner) stopAnim() {
	if this.anim == nil {
		return
	}
	this.anim.Stop()
	this.anim = nil
}

// EnumProperties exposes the user-tunable properties for the
// designer's property sheet. Cycle duration is left out because
// the property sheet doesn't render time.Duration cleanly yet.
func (this *Spinner) EnumProperties(list core.IPropertyList) {
	list.AddProperty("点数", this.DotCount, this.SetDotCount)
	list.AddProperty("运行中", this.IsBusy, this.SetBusy)
}

// SizeHints reports a default 24×24 with growth disabled — spinners
// are usually small icons next to a label, not stretched across a
// container. Callers that want a bigger spinner SetSize after add.
func (this *Spinner) SizeHints() SizeHints {
	return SizeHints{Width: 24, Height: 24}
}

// Draw paints the eight-dot rotating pattern when busy; renders
// nothing when not busy so a stopped spinner leaves no visual
// residue. The phase is computed straight from elapsed time so the
// rotation rate stays constant regardless of how often
// AnimationTick fires.
func (this *Spinner) Draw(g paint.Painter) {
	if !this.busy || this.dotCount < 3 {
		return
	}
	w, h := this.Self().Size()
	cx := w * 0.5
	cy := h * 0.5
	r := math.Min(w, h)*0.5 - 1
	if r <= 0 {
		return
	}
	dotR := r * 0.16
	if dotR < 1 {
		dotR = 1
	}

	// Phase is fractional (0..1) over one cycle. Multiplying by 2π
	// rotates the leading dot once per cycle.
	cycle := this.cycle
	if cycle <= 0 {
		cycle = time.Second
	}
	phase := math.Mod(time.Since(this.startTime).Seconds()/cycle.Seconds(), 1.0)
	leadAngle := phase * 2 * math.Pi

	// Draw each dot at its angular position with alpha that fades as
	// you move backwards around the circle. Dot 0 is at the lead
	// position (full alpha); dot 1 trails 1/N of a circle behind, etc.
	// The fade is exponential so the trailing tail decays smoothly
	// rather than popping at a hard edge.
	n := this.dotCount
	for i := 0; i < n; i++ {
		ang := leadAngle - 2*math.Pi*float64(i)/float64(n)
		x := cx + (r-dotR)*math.Cos(ang)
		y := cy + (r-dotR)*math.Sin(ang)
		// Alpha: dot 0 = 1.0, dot 1 = (1-1/n), … dot N-1 ≈ 0.
		alpha := 1.0 - float64(i)/float64(n)
		c := this.color
		c.A = uint8(float64(c.A) * alpha)
		g.Arc(x, y, dotR, 0, 2*math.Pi)
		g.SetBrush1(c)
		g.Fill()
	}
}
