package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
	"strconv"
)

func init() {
	core.RegisterFactory("gui.SpinBox", core.TypeOf((*SpinBox)(nil)))
}

// SpinBox is a numeric input widget with up/down buttons,
// equivalent to QSpinBox in Qt.
type SpinBox struct {
	Widget
	value    int
	min      int
	max      int
	step     int
	pageStep int // PageUp/PageDown step; 0 means auto = 10*step
	suffix   string

	hoverPart int // 0=none, 1=up, 2=down
	pushed    bool

	cbValueChanged func(interface{}, int)
}

func NewSpinBox() *SpinBox {
	p := new(SpinBox)
	p.Init(p)
	return p
}

func (this *SpinBox) Init(self IWidget) {
	this.Widget.Init(self)
	this.min = 0
	this.max = 99
	this.step = 1
	this.value = 0
}

func (this *SpinBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("最小值", this.Min, this.SetMin)
	list.AddProperty("最大值", this.Max, this.SetMax)
	list.AddProperty("步长", this.Step, this.SetStep)
}

func (this *SpinBox) Value() int {
	return this.value
}

func (this *SpinBox) SetValue(v int) {
	if v < this.min {
		v = this.min
	}
	if v > this.max {
		v = this.max
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
		if this.cbValueChanged != nil {
			this.cbValueChanged(this.Self(), this.value)
		}
	}
}

func (this *SpinBox) Min() int {
	return this.min
}

func (this *SpinBox) SetMin(v int) {
	this.min = v
	if this.max < this.min {
		this.max = this.min
	}
	this.SetValue(this.value)
}

func (this *SpinBox) Max() int {
	return this.max
}

func (this *SpinBox) SetMax(v int) {
	this.max = v
	if this.min > this.max {
		this.min = this.max
	}
	this.SetValue(this.value)
}

func (this *SpinBox) SetRange(min, max int) {
	if max < min {
		max = min
	}
	this.min = min
	this.max = max
	this.SetValue(this.value)
}

func (this *SpinBox) Step() int {
	return this.step
}

func (this *SpinBox) SetStep(s int) {
	if s < 1 {
		s = 1
	}
	this.step = s
}

// PageStep returns the larger step used by PageUp/PageDown. When unset it
// defaults to 10*step, mirroring QSpinBox's page jump.
func (this *SpinBox) PageStep() int {
	if this.pageStep > 0 {
		return this.pageStep
	}
	return this.step * 10
}

func (this *SpinBox) SetPageStep(s int) {
	if s < 0 {
		s = 0
	}
	this.pageStep = s
}

// spinStepped returns cur+delta clamped to [min, max]. Pure helper so the
// keyboard stepping logic stays unit-testable.
func spinStepped(cur, delta, min, max int) int {
	v := cur + delta
	if v < min {
		v = min
	} else if v > max {
		v = max
	}
	return v
}

func (this *SpinBox) Suffix() string {
	return this.suffix
}

func (this *SpinBox) SetSuffix(s string) {
	this.suffix = s
	this.Self().Update()
}

func (this *SpinBox) SetValueChangedCallback(cb func(interface{}, int)) {
	this.cbValueChanged = cb
}

func (this *SpinBox) displayText() string {
	s := strconv.Itoa(this.value)
	if this.suffix != "" {
		s += this.suffix
	}
	return s
}

// buttonWidth returns the width of the up/down button area on the right.
func (this *SpinBox) buttonWidth() float64 {
	return math.Max(this.h*0.6, 16)
}

func (this *SpinBox) Draw(g paint.Painter) {
	t := Theme()
	iw := this.Self()
	w, h := iw.Size()
	bw := this.buttonWidth()

	// Background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	// Edit frame
	t.DrawEditFrame(g, 0, 0, w, h, iw.HasFocus(), iw.IsHover(), false)

	// Draw text
	text := this.displayText()
	g.SetFont(t.Font)
	g.SetBrush1(t.TextColor)
	fe := t.Font.FontExtents()
	ext := t.Font.TextExtents(text)
	tx := 4.0 - ext.XBearing
	ty := (h+fe.Height)*0.5 - fe.Height + fe.Ascent
	g.Save()
	g.Rectangle(1, 1, w-bw-2, h-2)
	g.Clip()
	g.Translate(tx, ty)
	g.DrawText(text)
	g.Restore()

	// Separator line between text and buttons
	g.MoveTo(w-bw, 1)
	g.LineTo(w-bw, h-1)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Horizontal divider in button area
	hh := h * 0.5
	g.MoveTo(w-bw, hh)
	g.LineTo(w, hh)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Up button background (hover/push)
	if this.hoverPart == 1 {
		g.Rectangle(w-bw+1, 1, bw-2, hh-1)
		if this.pushed {
			g.SetBrush1(paint.Color{200, 200, 200, 255})
		} else {
			g.SetBrush1(paint.Color{228, 228, 228, 255})
		}
		g.Fill()
	}

	// Down button background (hover/push)
	if this.hoverPart == 2 {
		g.Rectangle(w-bw+1, hh, bw-2, hh-1)
		if this.pushed {
			g.SetBrush1(paint.Color{200, 200, 200, 255})
		} else {
			g.SetBrush1(paint.Color{228, 228, 228, 255})
		}
		g.Fill()
	}

	// Draw up arrow
	cx := w - bw*0.5
	cy := hh * 0.5
	a := 3.0
	g.MoveTo(cx, cy-a)
	g.LineTo(cx-a*1.2, cy+a*0.5)
	g.LineTo(cx+a*1.2, cy+a*0.5)
	g.LineTo(cx, cy-a)
	g.SetBrush1(t.TextColor)
	g.Fill()

	// Draw down arrow
	cy = hh + hh*0.5
	g.MoveTo(cx, cy+a)
	g.LineTo(cx-a*1.2, cy-a*0.5)
	g.LineTo(cx+a*1.2, cy-a*0.5)
	g.LineTo(cx, cy+a)
	g.SetBrush1(t.TextColor)
	g.Fill()

	_ = fmt.Sprint // suppress unused import
}

func (this *SpinBox) hitTest(x, y float64) int {
	bw := this.buttonWidth()
	if x >= this.w-bw {
		if y < this.h*0.5 {
			return 1 // up
		}
		return 2 // down
	}
	return 0 // text area
}

func (this *SpinBox) OnMouseEnter() {
	this.Self().Update()
}

func (this *SpinBox) OnMouseLeave() {
	this.hoverPart = 0
	this.pushed = false
	this.Self().Update()
}

func (this *SpinBox) OnMouseMove(x, y float64) {
	part := this.hitTest(x, y)
	if part != this.hoverPart {
		this.hoverPart = part
		this.Self().Update()
	}
}

func (this *SpinBox) OnLeftDown(x, y float64) {
	this.SetFocus()
	part := this.hitTest(x, y)
	this.hoverPart = part
	this.pushed = true
	switch part {
	case 1:
		this.SetValue(this.value + this.step)
	case 2:
		this.SetValue(this.value - this.step)
	}
	this.Self().Update()
}

func (this *SpinBox) OnLeftUp(x, y float64) {
	this.pushed = false
	this.Self().Update()
}

func (this *SpinBox) OnMouseWheel(x, y, z float64) {
	if z > 0 {
		this.SetValue(this.value + this.step)
	} else if z < 0 {
		this.SetValue(this.value - this.step)
	}
}

// OnKeyDown implements IEventKeyDown, giving the spin box Qt QSpinBox style
// keyboard navigation while it holds focus: Up/Down step by step, PageUp/
// PageDown by the larger page step, and Home/End jump to min/max. The spin box
// always has a bounded [min, max] range, so Home/End are always meaningful.
// All paths route through SetValue, so clamping and the change callback behave
// exactly as the up/down buttons do (callback fires only on a real change).
func (this *SpinBox) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	step := this.step
	page := this.PageStep()
	switch key {
	case KeyUp:
		this.SetValue(spinStepped(this.value, step, this.min, this.max))
	case KeyDown:
		this.SetValue(spinStepped(this.value, -step, this.min, this.max))
	case KeyPageUp:
		this.SetValue(spinStepped(this.value, page, this.min, this.max))
	case KeyPageDown:
		this.SetValue(spinStepped(this.value, -page, this.min, this.max))
	case KeyHome:
		this.SetValue(this.min)
	case KeyEnd:
		this.SetValue(this.max)
	}
}

func (this *SpinBox) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	text := fmt.Sprintf("%d%s", this.max, this.suffix)
	ext := t.Font.TextExtents(text)
	bw := math.Max(fe.Height*1.2, 16)
	w := ext.Width + bw + 12
	if w < 80 {
		w = 80
	}
	h := fe.Height + 8
	if h < 24 {
		h = 24
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}
