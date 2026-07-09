package gui

// SCADA / 组态 industrial widgets.
//
// A small library of process-visualization widgets for HMI / SCADA screens:
// Tank, Indicator (status lamp), DigitalDisplay (7-segment readout), Valve,
// Pipe, Pump, Thermometer and ValueBar. Each embeds Widget and mirrors the
// idiom of the existing chart widgets (see chart_gauge.go): an init() factory
// registration, a New* constructor calling Init(self), plain value
// setters/getters that call Self().Update(), a SizeHints() and a Draw().
//
// Every widget exposes plain-value setters (SetLevel, SetOn, SetValue, ...) so
// a tag binding can drive it directly, e.g. BindTagFloat(tag, tank.SetLevel).

import (
	"fmt"
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.Tank", core.TypeOf((*Tank)(nil)))
	core.RegisterFactory("gui.Indicator", core.TypeOf((*Indicator)(nil)))
	core.RegisterFactory("gui.DigitalDisplay", core.TypeOf((*DigitalDisplay)(nil)))
	core.RegisterFactory("gui.Valve", core.TypeOf((*Valve)(nil)))
	core.RegisterFactory("gui.Pipe", core.TypeOf((*Pipe)(nil)))
	core.RegisterFactory("gui.Pump", core.TypeOf((*Pump)(nil)))
	core.RegisterFactory("gui.Thermometer", core.TypeOf((*Thermometer)(nil)))
	core.RegisterFactory("gui.ValueBar", core.TypeOf((*ValueBar)(nil)))
}

// clamp01 constrains v to the [0,1] range.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ---------------------------------------------------------------------------
// Tank — vertical vessel with a fillable liquid level.
// ---------------------------------------------------------------------------

// Tank draws a vertical vessel whose liquid fills bottom-up by Level (0..1).
// Min/Max define the engineering range used only for the optional value label.
type Tank struct {
	Widget
	level     float64 // fill fraction, 0..1
	min, max  float64 // engineering range for the label
	color     paint.Color
	showLabel bool

	tagName string // design-time tag name driving this widget
}

// NewTank creates a Tank filled to 0 with a default blue liquid.
func NewTank() *Tank {
	p := new(Tank)
	p.Init(p)
	p.min = 0
	p.max = 100
	p.color = paint.Color{R: 33, G: 150, B: 243, A: 255} // blue
	p.showLabel = true
	return p
}

// SetLevel sets the fill fraction, clamped to [0,1].
func (this *Tank) SetLevel(v float64) {
	this.level = clamp01(v)
	this.Self().Update()
}

// Level returns the current fill fraction.
func (this *Tank) Level() float64 { return this.level }

// SetColor sets the liquid color.
func (this *Tank) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

// Color returns the liquid color.
func (this *Tank) Color() paint.Color { return this.color }

// SetRange sets the engineering min/max used for the value label.
func (this *Tank) SetRange(min, max float64) {
	this.min = min
	this.max = max
	this.Self().Update()
}

// Min returns the engineering minimum.
func (this *Tank) Min() float64 { return this.min }

// Max returns the engineering maximum.
func (this *Tank) Max() float64 { return this.max }

// SetShowLabel toggles the percent/value label.
func (this *Tank) SetShowLabel(b bool) {
	this.showLabel = b
	this.Self().Update()
}

// ShowLabel reports whether the value label is drawn.
func (this *Tank) ShowLabel() bool { return this.showLabel }

// EngValue maps the current level onto the engineering range.
func (this *Tank) EngValue() float64 {
	return this.min + this.level*(this.max-this.min)
}

// SetTagName sets the design-time tag name that drives this widget.
func (this *Tank) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Tank) TagName() string { return this.tagName }

func (this *Tank) EnumProperties(list core.IPropertyList) {
	list.AddProperty("液位", this.Level, this.SetLevel)
	list.AddProperty("显示标签", this.ShowLabel, this.SetShowLabel)
	list.AddProperty("颜色", this.Color, this.SetColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Tank) SizeHints() SizeHints {
	return SizeHints{Width: 80, Height: 140, Policy: GrowHorizontal | GrowVertical}
}

func (this *Tank) Draw(g paint.Painter) {
	w, h := this.Size()
	pad := 6.0
	bx := pad
	by := pad
	bw := w - 2*pad
	bh := h - 2*pad
	if bw < 4 || bh < 4 {
		return
	}

	// Vessel body background.
	g.SetBrush1(paint.Color{R: 236, G: 239, B: 241, A: 255})
	g.Rectangle(bx, by, bw, bh)
	g.Fill()

	// Liquid, filled from the bottom, with a top→bottom gradient.
	fillH := bh * this.level
	if fillH > 0 {
		fy := by + bh - fillH
		top := this.color
		bot := paint.Color{
			R: uint8(float64(this.color.R) * 0.65),
			G: uint8(float64(this.color.G) * 0.65),
			B: uint8(float64(this.color.B) * 0.65),
			A: this.color.A,
		}
		grad := paint.NewLinearGradient(float32(bx), float32(fy), float32(bx), float32(by+bh))
		grad.AddStop(0, top)
		grad.AddStop(1, bot)
		g.SetBrush(grad)
		g.Rectangle(bx, fy, bw, fillH)
		g.Fill()

		// Level line.
		g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 220}, 1.5)
		g.MoveTo(bx, fy)
		g.LineTo(bx+bw, fy)
		g.Stroke()
	}

	// Vessel outline.
	g.SetPen1(paint.Color{R: 96, G: 125, B: 139, A: 255}, 2)
	g.Rectangle(bx, by, bw, bh)
	g.Stroke()

	// Value label.
	if this.showLabel {
		t := Theme()
		text := fmt.Sprintf("%.0f%%", this.level*100)
		g.SetFont(t.Font)
		ext := t.Font.TextExtents(text)
		tx := bx + bw/2 - ext.Width/2 - ext.XBearing
		ty := by + bh/2 - ext.Height/2 - ext.YBearing
		g.SetBrush1(paint.Color{R: 33, G: 33, B: 33, A: 255})
		g.DrawText1(tx, ty, text)
	}
}

// ---------------------------------------------------------------------------
// Indicator — round status lamp (alarms / status).
// ---------------------------------------------------------------------------

// Indicator is a round status lamp that glows in On color when on and shows a
// dim Off color when off. Blink is a stored flag a caller may drive from the
// animation engine to toggle On for an alarm.
type Indicator struct {
	Widget
	on       bool
	color    paint.Color // on color
	offColor paint.Color
	blink    bool

	tagName string // design-time tag name driving this widget
}

// NewIndicator creates an off green lamp.
func NewIndicator() *Indicator {
	p := new(Indicator)
	p.Init(p)
	p.color = paint.Color{R: 76, G: 175, B: 80, A: 255}    // green
	p.offColor = paint.Color{R: 90, G: 96, B: 100, A: 255} // gray
	return p
}

// SetOn turns the lamp on or off.
func (this *Indicator) SetOn(b bool) {
	this.on = b
	this.Self().Update()
}

// IsOn reports the lamp state.
func (this *Indicator) IsOn() bool { return this.on }

// SetColor sets the on (lit) color.
func (this *Indicator) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

// Color returns the on (lit) color.
func (this *Indicator) Color() paint.Color { return this.color }

// SetOffColor sets the unlit color.
func (this *Indicator) SetOffColor(c paint.Color) {
	this.offColor = c
	this.Self().Update()
}

// OffColor returns the unlit color.
func (this *Indicator) OffColor() paint.Color { return this.offColor }

// SetBlink stores the blink flag (driven externally by the animation engine).
func (this *Indicator) SetBlink(b bool) {
	this.blink = b
	this.Self().Update()
}

// IsBlink reports the blink flag.
func (this *Indicator) IsBlink() bool { return this.blink }

// SetTagName sets the design-time tag name that drives this widget.
func (this *Indicator) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Indicator) TagName() string { return this.tagName }

func (this *Indicator) EnumProperties(list core.IPropertyList) {
	list.AddProperty("点亮", this.IsOn, this.SetOn)
	list.AddProperty("闪烁", this.IsBlink, this.SetBlink)
	list.AddProperty("颜色", this.Color, this.SetColor)
	list.AddProperty("熄灭颜色", this.OffColor, this.SetOffColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Indicator) SizeHints() SizeHints {
	return SizeHints{Width: 32, Height: 32, Policy: GrowHorizontal | GrowVertical}
}

func (this *Indicator) Draw(g paint.Painter) {
	w, h := this.Size()
	cx := w / 2
	cy := h / 2
	r := math.Min(w, h)/2 - 3
	if r < 2 {
		return
	}

	if this.on {
		// Radial glow from a bright center to the base color.
		bright := paint.Color{
			R: uint8(math.Min(255, float64(this.color.R)+80)),
			G: uint8(math.Min(255, float64(this.color.G)+80)),
			B: uint8(math.Min(255, float64(this.color.B)+80)),
			A: 255,
		}
		rg := paint.NewRadialGradient(float32(cx-r*0.3), float32(cy-r*0.3), 0, float32(r))
		rg.AddStop(0, bright)
		rg.AddStop(1, this.color)
		g.SetBrush(rg)
	} else {
		g.SetBrush1(this.offColor)
	}
	g.Arc(cx, cy, r, 0, 2*math.Pi)
	g.Fill()

	// Bezel.
	g.SetPen1(paint.Color{R: 55, G: 60, B: 65, A: 255}, 1.5)
	g.Arc(cx, cy, r, 0, 2*math.Pi)
	g.Stroke()

	// Specular highlight when lit.
	if this.on {
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 120})
		g.Arc(cx-r*0.3, cy-r*0.3, r*0.28, 0, 2*math.Pi)
		g.Fill()
	}
}

// ---------------------------------------------------------------------------
// DigitalDisplay — 7-segment numeric readout.
// ---------------------------------------------------------------------------

// segTable maps a rune to its seven segment on/off states, ordered
// [a, b, c, d, e, f, g] (top, top-right, bottom-right, bottom, bottom-left,
// top-left, middle).
var segTable = map[rune][7]bool{
	'0': {true, true, true, true, true, true, false},
	'1': {false, true, true, false, false, false, false},
	'2': {true, true, false, true, true, false, true},
	'3': {true, true, true, true, false, false, true},
	'4': {false, true, true, false, false, true, true},
	'5': {true, false, true, true, false, true, true},
	'6': {true, false, true, true, true, true, true},
	'7': {true, true, true, false, false, false, false},
	'8': {true, true, true, true, true, true, true},
	'9': {true, true, true, true, false, true, true},
	'-': {false, false, false, false, false, false, true},
}

// DigitalDisplay renders a numeric value in a 7-segment LCD style. Value is
// formatted through Format (a Printf verb such as "%.1f"). When limits are
// enabled the segment color changes below Lo or at/above Hi.
type DigitalDisplay struct {
	Widget
	value    float64
	format   string
	unit     string
	onColor  paint.Color // normal segment color
	offColor paint.Color // unlit segment color
	bgColor  paint.Color

	hasLimits bool
	lo, hi    float64
	loColor   paint.Color
	hiColor   paint.Color

	tagName string // design-time tag name driving this widget
}

// NewDigitalDisplay creates a green-on-black readout showing 0.
func NewDigitalDisplay() *DigitalDisplay {
	p := new(DigitalDisplay)
	p.Init(p)
	p.format = "%.1f"
	p.onColor = paint.Color{R: 0, G: 230, B: 118, A: 255} // green
	p.offColor = paint.Color{R: 30, G: 50, B: 40, A: 255}
	p.bgColor = paint.Color{R: 15, G: 20, B: 18, A: 255}
	p.loColor = paint.Color{R: 41, G: 182, B: 246, A: 255} // blue
	p.hiColor = paint.Color{R: 239, G: 83, B: 80, A: 255}  // red
	return p
}

// SetValue sets the displayed value.
func (this *DigitalDisplay) SetValue(v float64) {
	this.value = v
	this.Self().Update()
}

// Value returns the displayed value.
func (this *DigitalDisplay) Value() float64 { return this.value }

// SetFormat sets the Printf format verb used to render the value.
func (this *DigitalDisplay) SetFormat(f string) {
	this.format = f
	this.Self().Update()
}

// Format returns the current format verb.
func (this *DigitalDisplay) Format() string { return this.format }

// SetUnit sets the trailing unit string (e.g. "°C", "bar").
func (this *DigitalDisplay) SetUnit(s string) {
	this.unit = s
	this.Self().Update()
}

// Unit returns the trailing unit string.
func (this *DigitalDisplay) Unit() string { return this.unit }

// SetColor sets the normal (lit) segment color.
func (this *DigitalDisplay) SetColor(c paint.Color) {
	this.onColor = c
	this.Self().Update()
}

// Color returns the normal (lit) segment color.
func (this *DigitalDisplay) Color() paint.Color { return this.onColor }

// SetLimits enables lo/hi color changes at the given thresholds.
func (this *DigitalDisplay) SetLimits(lo, hi float64) {
	this.lo = lo
	this.hi = hi
	this.hasLimits = true
	this.Self().Update()
}

// Lo returns the low threshold.
func (this *DigitalDisplay) Lo() float64 { return this.lo }

// Hi returns the high threshold.
func (this *DigitalDisplay) Hi() float64 { return this.hi }

// displayColor returns the segment color for the current value.
func (this *DigitalDisplay) displayColor() paint.Color {
	if this.hasLimits {
		if this.value >= this.hi {
			return this.hiColor
		}
		if this.value <= this.lo {
			return this.loColor
		}
	}
	return this.onColor
}

// Text returns the formatted value string (without the unit).
func (this *DigitalDisplay) Text() string {
	return fmt.Sprintf(this.format, this.value)
}

// SetTagName sets the design-time tag name that drives this widget.
func (this *DigitalDisplay) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *DigitalDisplay) TagName() string { return this.tagName }

func (this *DigitalDisplay) EnumProperties(list core.IPropertyList) {
	list.AddProperty("数值", this.Value, this.SetValue)
	list.AddProperty("格式", this.Format, this.SetFormat)
	list.AddProperty("单位", this.Unit, this.SetUnit)
	list.AddProperty("颜色", this.Color, this.SetColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *DigitalDisplay) SizeHints() SizeHints {
	return SizeHints{Width: 120, Height: 48, Policy: GrowHorizontal | GrowVertical}
}

func (this *DigitalDisplay) Draw(g paint.Painter) {
	w, h := this.Size()

	// Panel background.
	g.SetBrush1(this.bgColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	pad := 6.0
	text := this.Text()
	on := this.displayColor()

	// Reserve room for the unit label.
	unitW := 0.0
	if this.unit != "" {
		t := Theme()
		unitW = t.Font.TextExtents(this.unit).Width + 6
	}

	digitAreaW := w - 2*pad - unitW
	digitH := h - 2*pad
	if digitAreaW < 6 || digitH < 6 {
		return
	}

	// A digit cell is 0.6*height wide; a '.' takes 0.35 of that.
	dw := digitH * 0.6
	nCells := 0.0
	for _, ch := range text {
		if ch == '.' {
			nCells += 0.35
		} else {
			nCells += 1
		}
	}
	if nCells > 0 && dw*nCells > digitAreaW {
		dw = digitAreaW / nCells
	}
	thick := math.Max(2, digitH*0.11)

	x := pad
	for _, ch := range text {
		if ch == '.' {
			dr := thick * 0.5
			g.SetBrush1(on)
			g.Arc(x+dw*0.35*0.5, pad+digitH-dr, dr, 0, 2*math.Pi)
			g.Fill()
			x += dw * 0.35
			continue
		}
		this.drawDigit(g, x, pad, dw, digitH, ch, on, thick)
		x += dw
	}

	// Unit label.
	if this.unit != "" {
		t := Theme()
		g.SetFont(t.Font)
		ext := t.Font.TextExtents(this.unit)
		ty := pad + digitH/2 - ext.Height/2 - ext.YBearing
		g.SetBrush1(on)
		g.DrawText1(w-pad-ext.Width, ty, this.unit)
	}
}

// drawDigit strokes the seven segments of a single character within the cell.
func (this *DigitalDisplay) drawDigit(g paint.Painter, x, y, w, h float64, ch rune, on paint.Color, thick float64) {
	segs, ok := segTable[ch]
	x0 := x + thick
	x1 := x + w - thick
	yt := y + thick
	ym := y + h/2
	yb := y + h - thick

	// Segment endpoints, ordered a,b,c,d,e,f,g.
	lines := [7][4]float64{
		{x0, yt, x1, yt}, // a top
		{x1, yt, x1, ym}, // b top-right
		{x1, ym, x1, yb}, // c bottom-right
		{x0, yb, x1, yb}, // d bottom
		{x0, ym, x0, yb}, // e bottom-left
		{x0, yt, x0, ym}, // f top-left
		{x0, ym, x1, ym}, // g middle
	}
	for i, ln := range lines {
		lit := ok && segs[i]
		if lit {
			g.SetPen1(on, thick)
		} else {
			g.SetPen1(this.offColor, thick)
		}
		g.MoveTo(ln[0], ln[1])
		g.LineTo(ln[2], ln[3])
		g.Stroke()
	}
}

// ---------------------------------------------------------------------------
// Valve — open/closed valve symbol.
// ---------------------------------------------------------------------------

// Valve draws the classic bow-tie valve symbol, colored by open/closed state.
type Valve struct {
	Widget
	open        bool
	openColor   paint.Color
	closedColor paint.Color

	tagName string // design-time tag name driving this widget
}

// NewValve creates a closed valve.
func NewValve() *Valve {
	p := new(Valve)
	p.Init(p)
	p.openColor = paint.Color{R: 76, G: 175, B: 80, A: 255}     // green
	p.closedColor = paint.Color{R: 158, G: 158, B: 158, A: 255} // gray
	return p
}

// SetState sets the valve open (true) or closed (false).
func (this *Valve) SetState(open bool) {
	this.open = open
	this.Self().Update()
}

// State reports whether the valve is open.
func (this *Valve) State() bool { return this.open }

// Toggle flips the valve state.
func (this *Valve) Toggle() { this.SetState(!this.open) }

// SetOpenColor sets the open-state color.
func (this *Valve) SetOpenColor(c paint.Color) {
	this.openColor = c
	this.Self().Update()
}

// OpenColor returns the open-state color.
func (this *Valve) OpenColor() paint.Color { return this.openColor }

// SetClosedColor sets the closed-state color.
func (this *Valve) SetClosedColor(c paint.Color) {
	this.closedColor = c
	this.Self().Update()
}

// ClosedColor returns the closed-state color.
func (this *Valve) ClosedColor() paint.Color { return this.closedColor }

// SetTagName sets the design-time tag name that drives this widget.
func (this *Valve) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Valve) TagName() string { return this.tagName }

func (this *Valve) EnumProperties(list core.IPropertyList) {
	list.AddProperty("打开", this.State, this.SetState)
	list.AddProperty("打开颜色", this.OpenColor, this.SetOpenColor)
	list.AddProperty("关闭颜色", this.ClosedColor, this.SetClosedColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Valve) SizeHints() SizeHints {
	return SizeHints{Width: 60, Height: 40, Policy: GrowHorizontal | GrowVertical}
}

func (this *Valve) Draw(g paint.Painter) {
	w, h := this.Size()
	cx := w / 2
	cy := h / 2
	half := math.Min(w, h*2) / 2
	tw := half * 0.9
	th := h/2 - 4
	if tw < 3 || th < 3 {
		return
	}

	col := this.closedColor
	if this.open {
		col = this.openColor
	}

	// Left triangle: apex at center, base on the left.
	g.SetBrush1(col)
	g.MoveTo(cx, cy)
	g.LineTo(cx-tw, cy-th)
	g.LineTo(cx-tw, cy+th)
	g.LineTo(cx, cy)
	g.Fill()

	// Right triangle: apex at center, base on the right.
	g.MoveTo(cx, cy)
	g.LineTo(cx+tw, cy-th)
	g.LineTo(cx+tw, cy+th)
	g.LineTo(cx, cy)
	g.Fill()

	// Outline.
	g.SetPen1(paint.Color{R: 66, G: 66, B: 66, A: 255}, 1.5)
	g.MoveTo(cx, cy)
	g.LineTo(cx-tw, cy-th)
	g.LineTo(cx-tw, cy+th)
	g.LineTo(cx, cy)
	g.LineTo(cx+tw, cy-th)
	g.LineTo(cx+tw, cy+th)
	g.LineTo(cx, cy)
	g.Stroke()
}

// ---------------------------------------------------------------------------
// Pipe — horizontal / vertical pipe segment with flow color.
// ---------------------------------------------------------------------------

// Pipe draws a straight pipe segment. When Active it is filled with FlowColor
// and overlaid with flow dashes; when inactive it is drawn in a neutral gray.
type Pipe struct {
	Widget
	vertical  bool
	active    bool
	flowColor paint.Color
	idleColor paint.Color

	tagName string // design-time tag name driving this widget
}

// NewPipe creates an inactive horizontal pipe.
func NewPipe() *Pipe {
	p := new(Pipe)
	p.Init(p)
	p.flowColor = paint.Color{R: 33, G: 150, B: 243, A: 255} // blue
	p.idleColor = paint.Color{R: 176, G: 190, B: 197, A: 255}
	return p
}

// SetActive toggles flow on the pipe.
func (this *Pipe) SetActive(b bool) {
	this.active = b
	this.Self().Update()
}

// IsActive reports whether flow is active.
func (this *Pipe) IsActive() bool { return this.active }

// SetFlowColor sets the color used when flow is active.
func (this *Pipe) SetFlowColor(c paint.Color) {
	this.flowColor = c
	this.Self().Update()
}

// FlowColor returns the active-flow color.
func (this *Pipe) FlowColor() paint.Color { return this.flowColor }

// SetVertical sets the pipe orientation (true = vertical).
func (this *Pipe) SetVertical(b bool) {
	this.vertical = b
	this.Self().Update()
}

// IsVertical reports the pipe orientation.
func (this *Pipe) IsVertical() bool { return this.vertical }

// SetTagName sets the design-time tag name that drives this widget.
func (this *Pipe) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Pipe) TagName() string { return this.tagName }

func (this *Pipe) EnumProperties(list core.IPropertyList) {
	list.AddProperty("有流量", this.IsActive, this.SetActive)
	list.AddProperty("竖直", this.IsVertical, this.SetVertical)
	list.AddProperty("流动颜色", this.FlowColor, this.SetFlowColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Pipe) SizeHints() SizeHints {
	if this.vertical {
		return SizeHints{Width: 24, Height: 120, Policy: GrowHorizontal | GrowVertical}
	}
	return SizeHints{Width: 120, Height: 24, Policy: GrowHorizontal | GrowVertical}
}

func (this *Pipe) Draw(g paint.Painter) {
	w, h := this.Size()

	col := this.idleColor
	if this.active {
		col = this.flowColor
	}

	var px, py, pw, ph float64
	if this.vertical {
		pw = math.Min(w, 24)
		px = (w - pw) / 2
		py = 0
		ph = h
	} else {
		ph = math.Min(h, 24)
		py = (h - ph) / 2
		px = 0
		pw = w
	}

	// Pipe body.
	g.SetBrush1(col)
	g.Rectangle(px, py, pw, ph)
	g.Fill()

	// Walls.
	g.SetPen1(paint.Color{R: 84, G: 110, B: 122, A: 255}, 1.5)
	g.Rectangle(px, py, pw, ph)
	g.Stroke()

	// Flow dashes.
	if this.active {
		g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 200}, 3)
		if this.vertical {
			dashLen := 8.0
			gap := 8.0
			for y := py + 4; y < py+ph-dashLen; y += dashLen + gap {
				g.MoveTo(px+pw/2, y)
				g.LineTo(px+pw/2, y+dashLen)
				g.Stroke()
			}
		} else {
			dashLen := 8.0
			gap := 8.0
			for x := px + 4; x < px+pw-dashLen; x += dashLen + gap {
				g.MoveTo(x, py+ph/2)
				g.LineTo(x+dashLen, py+ph/2)
				g.Stroke()
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Pump — running / stopped / fault status symbol.
// ---------------------------------------------------------------------------

// Pump draws a circular pump body with an impeller mark. It is green while
// running, gray while stopped, and red on fault (fault takes precedence).
type Pump struct {
	Widget
	running bool
	fault   bool

	tagName string // design-time tag name driving this widget
}

// NewPump creates a stopped pump.
func NewPump() *Pump {
	p := new(Pump)
	p.Init(p)
	return p
}

// SetRunning sets the running state.
func (this *Pump) SetRunning(b bool) {
	this.running = b
	this.Self().Update()
}

// IsRunning reports the running state.
func (this *Pump) IsRunning() bool { return this.running }

// SetFault sets the fault state (overrides running when drawn).
func (this *Pump) SetFault(b bool) {
	this.fault = b
	this.Self().Update()
}

// IsFault reports the fault state.
func (this *Pump) IsFault() bool { return this.fault }

// SetTagName sets the design-time tag name that drives this widget.
func (this *Pump) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Pump) TagName() string { return this.tagName }

func (this *Pump) EnumProperties(list core.IPropertyList) {
	list.AddProperty("运行", this.IsRunning, this.SetRunning)
	list.AddProperty("故障", this.IsFault, this.SetFault)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Pump) SizeHints() SizeHints {
	return SizeHints{Width: 60, Height: 60, Policy: GrowHorizontal | GrowVertical}
}

func (this *Pump) Draw(g paint.Painter) {
	w, h := this.Size()
	cx := w / 2
	cy := h / 2
	r := math.Min(w, h)/2 - 3
	if r < 3 {
		return
	}

	col := paint.Color{R: 158, G: 158, B: 158, A: 255} // stopped: gray
	if this.fault {
		col = paint.Color{R: 239, G: 83, B: 80, A: 255} // red
	} else if this.running {
		col = paint.Color{R: 76, G: 175, B: 80, A: 255} // green
	}

	// Body.
	g.SetBrush1(col)
	g.Arc(cx, cy, r, 0, 2*math.Pi)
	g.Fill()
	g.SetPen1(paint.Color{R: 55, G: 60, B: 65, A: 255}, 2)
	g.Arc(cx, cy, r, 0, 2*math.Pi)
	g.Stroke()

	// Impeller: three spokes from the center.
	g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 230}, 2)
	for i := 0; i < 3; i++ {
		a := float64(i) * (2 * math.Pi / 3)
		g.MoveTo(cx, cy)
		g.LineTo(cx+r*0.7*math.Cos(a), cy+r*0.7*math.Sin(a))
		g.Stroke()
	}
	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 230})
	g.Arc(cx, cy, r*0.14, 0, 2*math.Pi)
	g.Fill()
}

// ---------------------------------------------------------------------------
// Thermometer — bulb + column temperature indicator.
// ---------------------------------------------------------------------------

// Thermometer draws a bulb and a column whose fill height maps Value across
// [Min,Max].
type Thermometer struct {
	Widget
	value    float64
	min, max float64
	color    paint.Color

	tagName string // design-time tag name driving this widget
}

// NewThermometer creates a 0..100 thermometer reading Min.
func NewThermometer() *Thermometer {
	p := new(Thermometer)
	p.Init(p)
	p.min = 0
	p.max = 100
	p.value = 0
	p.color = paint.Color{R: 239, G: 83, B: 80, A: 255} // red
	return p
}

// SetValue sets the temperature reading, clamped to [Min,Max].
func (this *Thermometer) SetValue(v float64) {
	if v < this.min {
		v = this.min
	}
	if v > this.max {
		v = this.max
	}
	this.value = v
	this.Self().Update()
}

// Value returns the temperature reading.
func (this *Thermometer) Value() float64 { return this.value }

// SetRange sets the engineering range.
func (this *Thermometer) SetRange(min, max float64) {
	this.min = min
	this.max = max
	this.Self().Update()
}

// Min returns the range minimum.
func (this *Thermometer) Min() float64 { return this.min }

// Max returns the range maximum.
func (this *Thermometer) Max() float64 { return this.max }

// SetColor sets the mercury color.
func (this *Thermometer) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

// Color returns the mercury color.
func (this *Thermometer) Color() paint.Color { return this.color }

// Fraction returns the fill fraction of the current value over the range.
func (this *Thermometer) Fraction() float64 {
	rng := this.max - this.min
	if rng == 0 {
		return 0
	}
	return clamp01((this.value - this.min) / rng)
}

// SetTagName sets the design-time tag name that drives this widget.
func (this *Thermometer) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *Thermometer) TagName() string { return this.tagName }

func (this *Thermometer) EnumProperties(list core.IPropertyList) {
	list.AddProperty("温度", this.Value, this.SetValue)
	list.AddProperty("颜色", this.Color, this.SetColor)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *Thermometer) SizeHints() SizeHints {
	return SizeHints{Width: 40, Height: 140, Policy: GrowHorizontal | GrowVertical}
}

func (this *Thermometer) Draw(g paint.Painter) {
	w, h := this.Size()
	cx := w / 2
	bulbR := math.Min(w/2-2, 12)
	colW := bulbR * 0.9
	top := 6.0
	bulbCy := h - bulbR - 4
	colTop := top
	colBot := bulbCy
	colH := colBot - colTop
	if colH < 6 || bulbR < 3 {
		return
	}

	// Tube + bulb background.
	g.SetBrush1(paint.Color{R: 236, G: 239, B: 241, A: 255})
	g.Rectangle(cx-colW/2, colTop, colW, colH)
	g.Fill()
	g.Arc(cx, bulbCy, bulbR, 0, 2*math.Pi)
	g.Fill()

	// Mercury: bulb + column up to the fill fraction.
	g.SetBrush1(this.color)
	g.Arc(cx, bulbCy, bulbR*0.8, 0, 2*math.Pi)
	g.Fill()
	fillH := colH * this.Fraction()
	if fillH > 0 {
		g.Rectangle(cx-colW*0.35, colBot-fillH, colW*0.7, fillH)
		g.Fill()
	}

	// Outline.
	g.SetPen1(paint.Color{R: 96, G: 125, B: 139, A: 255}, 1.5)
	g.Rectangle(cx-colW/2, colTop, colW, colH)
	g.Stroke()
	g.Arc(cx, bulbCy, bulbR, 0, 2*math.Pi)
	g.Stroke()
}

// ---------------------------------------------------------------------------
// ValueBar — vertical bar over an engineering range with alarm bands.
// ---------------------------------------------------------------------------

// ValueBar draws a vertical bar filled to Value over [Min,Max]. The fill color
// is chosen from the LoLo / Lo / Hi / HiHi alarm bands the value falls into.
type ValueBar struct {
	Widget
	value    float64
	min, max float64

	hasLimits          bool
	loLo, lo, hi, hiHi float64
	normalColor        paint.Color
	warnColor          paint.Color
	alarmColor         paint.Color

	tagName string // design-time tag name driving this widget
}

// NewValueBar creates a 0..100 bar reading 0.
func NewValueBar() *ValueBar {
	p := new(ValueBar)
	p.Init(p)
	p.min = 0
	p.max = 100
	p.normalColor = paint.Color{R: 76, G: 175, B: 80, A: 255} // green
	p.warnColor = paint.Color{R: 255, G: 179, B: 0, A: 255}   // amber
	p.alarmColor = paint.Color{R: 239, G: 83, B: 80, A: 255}  // red
	return p
}

// SetValue sets the bar value, clamped to [Min,Max].
func (this *ValueBar) SetValue(v float64) {
	if v < this.min {
		v = this.min
	}
	if v > this.max {
		v = this.max
	}
	this.value = v
	this.Self().Update()
}

// Value returns the bar value.
func (this *ValueBar) Value() float64 { return this.value }

// SetRange sets the engineering range.
func (this *ValueBar) SetRange(min, max float64) {
	this.min = min
	this.max = max
	this.Self().Update()
}

// Min returns the range minimum.
func (this *ValueBar) Min() float64 { return this.min }

// Max returns the range maximum.
func (this *ValueBar) Max() float64 { return this.max }

// SetLimits enables the LoLo/Lo/Hi/HiHi alarm bands.
func (this *ValueBar) SetLimits(loLo, lo, hi, hiHi float64) {
	this.loLo = loLo
	this.lo = lo
	this.hi = hi
	this.hiHi = hiHi
	this.hasLimits = true
	this.Self().Update()
}

// LoLo returns the low-low limit.
func (this *ValueBar) LoLo() float64 { return this.loLo }

// Lo returns the low limit.
func (this *ValueBar) Lo() float64 { return this.lo }

// Hi returns the high limit.
func (this *ValueBar) Hi() float64 { return this.hi }

// HiHi returns the high-high limit.
func (this *ValueBar) HiHi() float64 { return this.hiHi }

// barColor picks the fill color from the alarm band the value falls into.
func (this *ValueBar) barColor() paint.Color {
	if !this.hasLimits {
		return this.normalColor
	}
	if this.value <= this.loLo || this.value >= this.hiHi {
		return this.alarmColor
	}
	if this.value <= this.lo || this.value >= this.hi {
		return this.warnColor
	}
	return this.normalColor
}

// Fraction returns the fill fraction of the current value over the range.
func (this *ValueBar) Fraction() float64 {
	rng := this.max - this.min
	if rng == 0 {
		return 0
	}
	return clamp01((this.value - this.min) / rng)
}

// SetTagName sets the design-time tag name that drives this widget.
func (this *ValueBar) SetTagName(s string) {
	this.tagName = s
	this.Self().Update()
}

// TagName returns the design-time tag name.
func (this *ValueBar) TagName() string { return this.tagName }

func (this *ValueBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("数值", this.Value, this.SetValue)
	list.AddProperty("tag", this.TagName, this.SetTagName)
}

func (this *ValueBar) SizeHints() SizeHints {
	return SizeHints{Width: 48, Height: 160, Policy: GrowHorizontal | GrowVertical}
}

func (this *ValueBar) Draw(g paint.Painter) {
	w, h := this.Size()
	pad := 6.0
	bx := pad
	by := pad
	bw := w - 2*pad
	bh := h - 2*pad
	if bw < 4 || bh < 4 {
		return
	}

	// Track.
	g.SetBrush1(paint.Color{R: 236, G: 239, B: 241, A: 255})
	g.Rectangle(bx, by, bw, bh)
	g.Fill()

	// Fill from the bottom.
	fillH := bh * this.Fraction()
	if fillH > 0 {
		g.SetBrush1(this.barColor())
		g.Rectangle(bx, by+bh-fillH, bw, fillH)
		g.Fill()
	}

	// Frame.
	g.SetPen1(paint.Color{R: 120, G: 130, B: 138, A: 255}, 1.5)
	g.Rectangle(bx, by, bw, bh)
	g.Stroke()

	// Limit tick lines.
	if this.hasLimits {
		rng := this.max - this.min
		if rng != 0 {
			marks := []struct {
				v float64
				c paint.Color
			}{
				{this.hiHi, this.alarmColor},
				{this.hi, this.warnColor},
				{this.lo, this.warnColor},
				{this.loLo, this.alarmColor},
			}
			for _, m := range marks {
				frac := clamp01((m.v - this.min) / rng)
				y := by + bh - bh*frac
				g.SetPen1(m.c, 1.5)
				g.MoveTo(bx, y)
				g.LineTo(bx+bw, y)
				g.Stroke()
			}
		}
	}
}
