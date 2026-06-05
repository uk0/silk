package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// Slider provides a draggable control for selecting a value within a range
type Slider struct {
	Widget
	min, max, value float64
	step            float64 // single-step size (arrow keys); 0 means auto = (max-min)/100
	pageStep        float64 // page-step size (PageUp/PageDown); 0 means auto = (max-min)/10
	tickInterval    float64 // spacing of tick marks along the track; 0 means none
	vertical        bool
	dragging        bool
	cbValueChanged  func(interface{}, float64)
}

func init() {
	core.RegisterFactory("gui.Slider", core.TypeOf((*Slider)(nil)))
}

func NewSlider(min, max float64) *Slider {
	p := new(Slider)
	p.Init(p)
	p.min = min
	p.max = max
	if max < min {
		p.max = min
	}
	p.value = p.min
	return p
}

func (this *Slider) Value() float64 {
	return this.value
}

func (this *Slider) SetValue(v float64) {
	if v < this.min {
		v = this.min
	} else if v > this.max {
		v = this.max
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
		this.fireValueChanged()
	}
}

func (this *Slider) Min() float64 {
	return this.min
}

func (this *Slider) SetMin(v float64) {
	this.SetRange(v, this.max)
}

func (this *Slider) Max() float64 {
	return this.max
}

func (this *Slider) SetMax(v float64) {
	this.SetRange(this.min, v)
}

func (this *Slider) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("最小值", this.Min, this.SetMin)
	list.AddProperty("最大值", this.Max, this.SetMax)
	list.AddProperty("垂直", this.IsVertical, this.SetVertical)
}

func (this *Slider) SetRange(min, max float64) {
	if max < min {
		max = min
	}
	this.min = min
	this.max = max
	this.SetValue(this.value)
}

func (this *Slider) Range() (min, max float64) {
	return this.min, this.max
}

// Step returns the single-step size used by the arrow keys. When unset it
// defaults to 1/100 of the range, mirroring Qt's QAbstractSlider.
func (this *Slider) Step() float64 {
	if this.step > 0 {
		return this.step
	}
	if s := (this.max - this.min) / 100; s > 0 {
		return s
	}
	return 1
}

func (this *Slider) SetStep(s float64) {
	if s < 0 {
		s = 0
	}
	this.step = s
}

// PageStep returns the larger step used by PageUp/PageDown. When unset it
// defaults to 1/10 of the range.
func (this *Slider) PageStep() float64 {
	if this.pageStep > 0 {
		return this.pageStep
	}
	if s := (this.max - this.min) / 10; s > 0 {
		return s
	}
	return 1
}

func (this *Slider) SetPageStep(s float64) {
	if s < 0 {
		s = 0
	}
	this.pageStep = s
}

// TickInterval is the spacing between tick marks drawn along the track.
// A value of 0 disables tick marks.
func (this *Slider) TickInterval() float64 {
	return this.tickInterval
}

func (this *Slider) SetTickInterval(v float64) {
	if v < 0 {
		v = 0
	}
	this.tickInterval = v
	this.Self().Update()
}

// steppedValue returns cur+delta clamped to [min, max]. Pure helper so the
// keyboard stepping logic stays unit-testable.
func steppedValue(cur, delta, min, max float64) float64 {
	v := cur + delta
	if v < min {
		v = min
	} else if v > max {
		v = max
	}
	return v
}

func (this *Slider) SetVertical(b bool) {
	this.vertical = b
	this.Self().Update()
}

func (this *Slider) IsVertical() bool {
	return this.vertical
}

func (this *Slider) SetValueChangedCallback(cb func(interface{}, float64)) {
	this.cbValueChanged = cb
}

func (this *Slider) fireValueChanged() {
	if this.cbValueChanged != nil {
		this.cbValueChanged(this.Self(), this.value)
	}
}

// --- Mouse position to value conversion ---

func (this *Slider) posToValue(x, y float64) float64 {
	if this.max <= this.min {
		return this.min
	}
	thumbSize := 12.0
	half := thumbSize * 0.5
	var ratio float64
	if this.vertical {
		_, h := this.Size()
		track := h - thumbSize
		if track <= 0 {
			return this.min
		}
		ratio = (y - half) / track
	} else {
		w, _ := this.Size()
		track := w - thumbSize
		if track <= 0 {
			return this.min
		}
		ratio = (x - half) / track
	}
	if ratio < 0 {
		ratio = 0
	} else if ratio > 1 {
		ratio = 1
	}
	return this.min + ratio*(this.max-this.min)
}

// --- Events ---

func (this *Slider) OnMouseEnter() {
	this.Self().Update()
}

func (this *Slider) OnMouseLeave() {
	this.Self().Update()
}

func (this *Slider) OnLeftDown(x, y float64) {
	if !this.IsEnabled() {
		return
	}
	this.dragging = true
	this.SetFocus()
	this.PushCapture()
	v := this.posToValue(x, y)
	this.SetValue(v)
}

func (this *Slider) OnLeftUp(x, y float64) {
	this.dragging = false
	this.PopCapture()
	this.Self().Update()
}

func (this *Slider) OnMouseMove(x, y float64) {
	if this.dragging {
		v := this.posToValue(x, y)
		this.SetValue(v)
	}
}

// OnKeyDown implements IEventKeyDown, giving the slider Qt QSlider style
// keyboard navigation while it holds focus. The same key mapping is used for
// both orientations: Up/Right increase, Down/Left decrease.
func (this *Slider) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	step := this.Step()
	page := this.PageStep()
	switch key {
	case KeyLeft, KeyDown:
		this.SetValue(steppedValue(this.value, -step, this.min, this.max))
	case KeyRight, KeyUp:
		this.SetValue(steppedValue(this.value, step, this.min, this.max))
	case KeyPageDown:
		this.SetValue(steppedValue(this.value, -page, this.min, this.max))
	case KeyPageUp:
		this.SetValue(steppedValue(this.value, page, this.min, this.max))
	case KeyHome:
		this.SetValue(this.min)
	case KeyEnd:
		this.SetValue(this.max)
	}
}

// --- Drawing ---

func (this *Slider) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	thumbSize := 12.0
	half := thumbSize * 0.5
	trackThickness := 4.0

	if this.vertical {
		// vertical track
		tx := (w - trackThickness) * 0.5
		g.Rectangle(tx, half, trackThickness, h-thumbSize)
		g.SetBrush1(t.BorderColor)
		g.Fill()

		// tick marks
		this.drawTicks(g, w, h, thumbSize, half)

		// thumb position
		track := h - thumbSize
		var ratio float64
		if this.max > this.min {
			ratio = (this.value - this.min) / (this.max - this.min)
		}
		ty := half + ratio*track

		// thumb circle
		g.Arc(w*0.5, ty, half, 0, 2*math.Pi)
		if this.dragging {
			g.SetBrush1(t.HighLightColor)
		} else {
			g.SetBrush1(paint.Color{100, 100, 100, 255})
		}
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
	} else {
		// horizontal track
		ty := (h - trackThickness) * 0.5
		g.Rectangle(half, ty, w-thumbSize, trackThickness)
		g.SetBrush1(t.BorderColor)
		g.Fill()

		// tick marks
		this.drawTicks(g, w, h, thumbSize, half)

		// thumb position
		track := w - thumbSize
		var ratio float64
		if this.max > this.min {
			ratio = (this.value - this.min) / (this.max - this.min)
		}
		tx := half + ratio*track

		// thumb circle
		g.Arc(tx, h*0.5, half, 0, 2*math.Pi)
		if this.dragging {
			g.SetBrush1(t.HighLightColor)
		} else {
			g.SetBrush1(paint.Color{100, 100, 100, 255})
		}
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
	}
}

// drawTicks paints evenly-spaced tick marks along the track when a
// tickInterval is set. Each tick is a short line perpendicular to the track,
// drawn under the thumb so it never obscures the handle.
func (this *Slider) drawTicks(g paint.Painter, w, h, thumbSize, half float64) {
	if this.tickInterval <= 0 || this.max <= this.min {
		return
	}
	t := Theme()
	tickLen := 4.0
	if this.vertical {
		track := h - thumbSize
		for v := this.min; v <= this.max+this.tickInterval*0.001; v += this.tickInterval {
			ratio := (v - this.min) / (this.max - this.min)
			ty := half + ratio*track
			g.MoveTo(w*0.5+half, ty)
			g.LineTo(w*0.5+half+tickLen, ty)
		}
	} else {
		track := w - thumbSize
		for v := this.min; v <= this.max+this.tickInterval*0.001; v += this.tickInterval {
			ratio := (v - this.min) / (this.max - this.min)
			tx := half + ratio*track
			g.MoveTo(tx, h*0.5+half)
			g.LineTo(tx, h*0.5+half+tickLen)
		}
	}
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
}

// --- SizeHints ---

func (this *Slider) SizeHints() SizeHints {
	if this.vertical {
		return SizeHints{Width: 20, Height: 100, Policy: GrowVertical | GrowHorizontal}
	}
	return SizeHints{Width: 100, Height: 20, Policy: GrowHorizontal | GrowVertical}
}
