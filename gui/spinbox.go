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

// SpinBox edits an integer with bounded steps and an owned single-line editor.
// Drafts commit on Enter or focus loss; Value reports the last committed value.
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

	editor         *spinBoxEdit
	layoutTheme    uint64
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
	this.editor = newSpinBoxEdit(this)
	this.editor.SetParent(self)
	this.syncEditor()
}

func (this *SpinBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("最小值", this.Min, this.SetMin)
	list.AddProperty("最大值", this.Max, this.SetMax)
	list.AddProperty("步长", this.Step, this.SetStep)
	list.AddProperty("分页步长", this.PageStepOverride, this.SetPageStep)
	list.AddProperty("后缀", this.Suffix, this.SetSuffix)
	list.AddProperty("只读", this.IsReadOnly, this.SetReadOnly)
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
	changed := v != this.value
	this.value = v
	draftChanged := this.syncEditor() // an explicit model update supersedes any local draft
	if changed || draftChanged {
		this.Self().Update()
	}
	if changed && this.cbValueChanged != nil {
		this.cbValueChanged(this.Self(), v)
	}
}

func (this *SpinBox) Min() int {
	return this.min
}

func (this *SpinBox) SetMin(v int) {
	if this.min == v {
		return
	}
	this.min = v
	if this.max < this.min {
		this.max = this.min
	}
	this.SetValue(this.value)
	this.Self().Update()
}

func (this *SpinBox) Max() int {
	return this.max
}

func (this *SpinBox) SetMax(v int) {
	if this.max == v {
		return
	}
	this.max = v
	if this.min > this.max {
		this.min = this.max
	}
	this.SetValue(this.value)
	this.Self().Update()
}

func (this *SpinBox) SetRange(min, max int) {
	if max < min {
		max = min
	}
	if this.min == min && this.max == max {
		return
	}
	this.min = min
	this.max = max
	this.SetValue(this.value)
	this.Self().Update()
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
	if this.step > math.MaxInt/10 {
		return math.MaxInt
	}
	return this.step * 10
}

// PageStepOverride returns the configured page step; zero keeps it automatic.
func (this *SpinBox) PageStepOverride() int { return this.pageStep }

func (this *SpinBox) SetPageStep(s int) {
	if s < 0 {
		s = 0
	}
	this.pageStep = s
}

// spinStepped returns cur+delta clamped to [min, max]. Pure helper so the
// keyboard stepping logic stays unit-testable.
func spinStepped(cur, delta, min, max int) int {
	// Check machine limits before addition; clamping a wrapped result is too late.
	if delta > 0 && cur > math.MaxInt-delta {
		return max
	}
	if delta < 0 && cur < math.MinInt-delta {
		return min
	}
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
	if this.suffix == s {
		return
	}
	this.suffix = s
	this.Layout()
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
	return math.Min(math.Max(0, this.w), math.Max(this.h*0.6, 16))
}

func (this *SpinBox) Draw(g paint.Painter) {
	if this.layoutTheme != ThemeRev() {
		this.Layout()
	}
	t := Theme()
	w, h := this.Size()
	if w <= 0 || h <= 0 {
		return
	}
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()
	t.DrawEditFrame(g, 0, 0, w, h, this.HasFocus(), this.IsHover(), this.IsReadOnly())
	bw, hh := this.buttonWidth(), h/2
	g.MoveTo(w-bw, 1)
	g.LineTo(w-bw, h-1)
	g.MoveTo(w-bw, hh)
	g.LineTo(w, hh)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
	for part := 1; part <= 2; part++ {
		canStep := this.IsEnabled() && !this.IsReadOnly() && (part == 1 && this.value < this.max || part == 2 && this.value > this.min)
		ink := t.TextColor
		if !canStep {
			ink = t.MenuGrayTextColor
		}
		if canStep && this.hoverPart == part {
			fill := t.ButtonHoverBGColor
			if this.pushed {
				fill = t.MenuActiveBGColor
				ink = t.MenuActiveTextColor
			}
			g.Rectangle(w-bw+1, float64(part-1)*hh+1, math.Max(0, bw-2), math.Max(0, hh-2))
			g.SetBrush1(fill)
			g.Fill()
		}
		a := math.Min(3, math.Min(bw/4, hh/3))
		cx, cy := w-bw/2, (float64(part)-.5)*hh
		direction := 1.0
		if part == 2 {
			direction = -1
		}
		g.MoveTo(cx, cy-direction*a)
		g.LineTo(cx-a*1.2, cy+direction*a*.5)
		g.LineTo(cx+a*1.2, cy+direction*a*.5)
		g.LineTo(cx, cy-direction*a)
		g.SetBrush1(ink)
		g.Fill()
	}
	if this.suffix != "" {
		r := this.suffixRect()
		text := ElideText(t.Font, this.suffix, r.Width)
		g.Save()
		g.Rectangle(r.X, r.Y, r.Width, r.Height)
		g.Clip()
		g.SetFont(t.Font)
		g.SetBrush1(t.TextColor)
		fe := t.Font.FontExtents()
		g.Translate(r.X-t.Font.TextExtents(text).XBearing, (h-fe.Height)/2+fe.Ascent)
		g.DrawText(text)
		g.Restore()
	}
}

func (this *SpinBox) hitTest(x, y float64) int {
	if x < 0 || y < 0 || x >= this.w || y >= this.h {
		return 0
	}
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
	if !this.IsEnabled() || this.IsReadOnly() {
		return
	}
	if x < 0 || y < 0 || x >= this.w || y >= this.h {
		return
	}
	this.SetFocus()
	part := this.hitTest(x, y)
	this.hoverPart = part
	this.pushed = true
	switch part {
	case 1:
		this.stepBy(this.step)
	case 2:
		this.stepBy(-this.step)
	}
	this.Self().Update()
}

func (this *SpinBox) OnLeftUp(x, y float64) {
	this.pushed = false
	this.Self().Update()
}

func (this *SpinBox) OnMouseWheel(x, y, z float64) {
	if !this.IsEnabled() || this.IsReadOnly() {
		return
	}
	if z > 0 {
		this.stepBy(this.step)
	} else if z < 0 {
		this.stepBy(-this.step)
	}
}

// OnKeyDown implements IEventKeyDown, giving the spin box Qt QSpinBox style
// keyboard navigation while it holds focus: Up/Down step by step, PageUp/
// PageDown by the larger page step, and Home/End jump to min/max. The spin box
// always has a bounded [min, max] range, so Home/End are always meaningful.
// All paths route through SetValue, so clamping and the change callback behave
// exactly as the up/down buttons do (callback fires only on a real change).
func (this *SpinBox) OnKeyDown(key int, repeat bool) {
	if this.editor != nil {
		this.editor.OnKeyDown(key, repeat)
	}
}

func (this *SpinBox) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	// Both bounds must be measured: Draw clips the text to the strip left of
	// the buttons, and a negative minimum ("-1000000") is far wider than the
	// maximum it is paired with.
	textW := t.Font.TextExtents(fmt.Sprintf("%d%s", this.max, this.suffix)).Width
	textW = math.Max(textW, t.Font.TextExtents(fmt.Sprintf("%d%s", this.min, this.suffix)).Width)
	bw := math.Max(fe.Height*1.2, 16)
	w := textW + bw + 12
	if w < 80 {
		w = 80
	}
	h := fe.Height + 8
	if h < 24 {
		h = 24
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}
