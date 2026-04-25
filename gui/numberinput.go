package gui

import (
	"silk/core"
	"silk/paint"
	"math"
	"strconv"
)

// NumberInput 数值输入控件，支持浮点数，带上下调节按钮
type NumberInput struct {
	Widget
	value    float64
	min      float64
	max      float64
	step     float64
	decimals int
	editing  bool
	editText string
	focused  bool
	hoverUp  bool
	hoverDn  bool

	cbValueChanged func(float64)
}

func init() {
	core.RegisterFactory("gui.NumberInput", core.TypeOf((*NumberInput)(nil)))
}

func NewNumberInput() *NumberInput {
	p := new(NumberInput)
	p.Init(p)
	p.min = 0
	p.max = 100
	p.step = 1
	p.decimals = 2
	return p
}

func (this *NumberInput) Value() float64 {
	return this.value
}

func (this *NumberInput) SetValue(v float64) {
	v = math.Max(this.min, math.Min(this.max, v))
	if v != this.value {
		this.value = v
		if this.cbValueChanged != nil {
			this.cbValueChanged(v)
		}
		this.Self().Update()
	}
}

func (this *NumberInput) Min() float64     { return this.min }
func (this *NumberInput) Max() float64     { return this.max }
func (this *NumberInput) Step() float64    { return this.step }
func (this *NumberInput) Decimals() int    { return this.decimals }

func (this *NumberInput) SetMin(v float64)     { this.min = v; this.SetValue(this.value) }
func (this *NumberInput) SetMax(v float64)     { this.max = v; this.SetValue(this.value) }
func (this *NumberInput) SetStep(v float64)    { this.step = v }
func (this *NumberInput) SetDecimals(n int)    { this.decimals = n; this.Self().Update() }

func (this *NumberInput) SetRange(min, max float64) {
	this.min = min
	this.max = max
	this.SetValue(this.value)
}

func (this *NumberInput) SigValueChanged(fn func(float64)) {
	this.cbValueChanged = fn
}

func (this *NumberInput) StepUp() {
	this.SetValue(this.value + this.step)
}

func (this *NumberInput) StepDown() {
	this.SetValue(this.value - this.step)
}

func (this *NumberInput) formatValue() string {
	if this.editing {
		return this.editText
	}
	return strconv.FormatFloat(this.value, 'f', this.decimals, 64)
}

// --- Events ---

func (this *NumberInput) OnMouseEnter() {
	this.Self().Update()
}

func (this *NumberInput) OnMouseLeave() {
	this.hoverUp = false
	this.hoverDn = false
	this.Self().Update()
}

func (this *NumberInput) OnMouseMove(x, y float64) {
	w, h := this.Size()
	btnW := 20.0
	inBtn := x >= w-btnW
	wasUp, wasDn := this.hoverUp, this.hoverDn
	this.hoverUp = inBtn && y < h/2
	this.hoverDn = inBtn && y >= h/2
	if wasUp != this.hoverUp || wasDn != this.hoverDn {
		this.Self().Update()
	}
}

func (this *NumberInput) OnLeftDown(x, y float64) {
	w, h := this.Size()
	btnW := 20.0

	if x >= w-btnW {
		if y < h/2 {
			this.StepUp()
		} else {
			this.StepDown()
		}
		return
	}

	this.SetFocus()
	this.editing = true
	this.editText = this.formatValue()
	this.Self().Update()
}

func (this *NumberInput) OnFocusIn() {
	this.focused = true
	this.Self().Update()
}

func (this *NumberInput) OnFocusOut() {
	this.focused = false
	if this.editing {
		this.commitEdit()
	}
	this.Self().Update()
}

func (this *NumberInput) commitEdit() {
	this.editing = false
	v, err := strconv.ParseFloat(this.editText, 64)
	if err == nil {
		this.SetValue(v)
	}
}

func (this *NumberInput) OnKeyDown(key int, mods int) {
	if !this.editing {
		switch key {
		case KeyUp:
			this.StepUp()
		case KeyDown:
			this.StepDown()
		}
		return
	}

	switch key {
	case KeyBackSpace:
		if len(this.editText) > 0 {
			runes := []rune(this.editText)
			this.editText = string(runes[:len(runes)-1])
			this.Self().Update()
		}
	case KeyEnter:
		this.commitEdit()
		this.Self().Update()
	case KeyEsc:
		this.editing = false
		this.Self().Update()
	case KeyUp:
		this.StepUp()
	case KeyDown:
		this.StepDown()
	}
}

func (this *NumberInput) OnChar(ch rune) {
	if !this.editing {
		return
	}
	// allow digits, dot, minus
	if (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' {
		this.editText += string(ch)
		this.Self().Update()
	}
}

// --- Drawing ---

func (this *NumberInput) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	btnW := 20.0
	textW := w - btnW

	// text area background
	g.Rectangle(0, 0, textW, h)
	if this.focused {
		g.SetBrush1(t.ViewBGColor)
	} else {
		g.SetBrush1(t.FormColor)
	}
	g.FillPreserve()
	if this.focused {
		g.SetPen1(t.HighLightColor, 1.5)
	} else {
		g.SetPen1(t.BorderColor, 1)
	}
	g.Stroke()

	// value text
	displayText := this.formatValue()
	g.SetFont(t.Font)
	fe := t.Font.FontExtents()
	ext := t.Font.TextExtents(displayText)
	tx := 6.0
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.SetBrush1(t.TextColor)
	g.Translate(tx, ty)
	g.DrawText(displayText)
	g.Translate(-tx, -ty)

	// cursor when editing
	if this.editing && this.focused {
		cx := tx + ext.Width + ext.XBearing + 1
		g.MoveTo(cx, (h-fe.Height)/2)
		g.LineTo(cx, (h+fe.Height)/2)
		g.SetPen1(t.TextColor, 1)
		g.Stroke()
	}

	// button area
	// up button
	g.Rectangle(textW, 0, btnW, h/2)
	if this.hoverUp {
		g.SetBrush1(t.FormLightColor)
	} else {
		g.SetBrush1(t.FormColor)
	}
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// up arrow
	cx := textW + btnW/2
	cy := h / 4
	g.MoveTo(cx-4, cy+2)
	g.LineTo(cx, cy-2)
	g.LineTo(cx+4, cy+2)
	g.SetPen1(t.FormDarkColor, 1.5)
	g.Stroke()

	// down button
	g.Rectangle(textW, h/2, btnW, h/2)
	if this.hoverDn {
		g.SetBrush1(t.FormLightColor)
	} else {
		g.SetBrush1(t.FormColor)
	}
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// down arrow
	cy = h * 3 / 4
	g.MoveTo(cx-4, cy-2)
	g.LineTo(cx, cy+2)
	g.LineTo(cx+4, cy-2)
	g.SetPen1(t.FormDarkColor, 1.5)
	g.Stroke()

}

func (this *NumberInput) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	return SizeHints{Width: 100, Height: fe.Height + 10, Policy: GrowHorizontal | GrowVertical}
}

func (this *NumberInput) EnumProperties(list core.IPropertyList) {
	list.AddProperty("值", this.Value, this.SetValue)
	list.AddProperty("最小值", this.Min, this.SetMin)
	list.AddProperty("最大值", this.Max, this.SetMax)
	list.AddProperty("步长", this.Step, this.SetStep)
	list.AddProperty("小数位", this.Decimals, this.SetDecimals)
}
