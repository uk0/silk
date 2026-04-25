package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// RadioGroup manages mutual exclusion among RadioButtons
type RadioGroup struct {
	buttons  []*RadioButton
	selected int
}

func NewRadioGroup() *RadioGroup {
	return &RadioGroup{selected: -1}
}

func (g *RadioGroup) add(rb *RadioButton) {
	g.buttons = append(g.buttons, rb)
}

func (g *RadioGroup) selectButton(rb *RadioButton) {
	for i, b := range g.buttons {
		if b == rb {
			g.selected = i
			if !b.checked {
				b.checked = true
				b.fireChanged(true)
				b.Self().Update()
			}
		} else if b.checked {
			b.checked = false
			b.fireChanged(false)
			b.Self().Update()
		}
	}
}

// SelectedIndex returns the index of the currently selected button, or -1
func (g *RadioGroup) SelectedIndex() int {
	return g.selected
}

// RadioButton is a mutually-exclusive toggle within a RadioGroup
type RadioButton struct {
	Widget
	text      string
	checked   bool
	group     *RadioGroup
	cbChanged func(interface{}, bool)
}

func init() {
	core.RegisterFactory("gui.RadioButton", core.TypeOf((*RadioButton)(nil)))
}

func NewRadioButton(text string, group *RadioGroup) *RadioButton {
	p := new(RadioButton)
	p.Init(p)
	p.text = text
	p.group = group
	if group != nil {
		group.add(p)
	}
	return p
}

func (this *RadioButton) IsChecked() bool {
	return this.checked
}

func (this *RadioButton) SetChecked(b bool) {
	if b == this.checked {
		return
	}
	if b && this.group != nil {
		this.group.selectButton(this)
	} else if !b {
		this.checked = false
		this.fireChanged(false)
		this.Self().Update()
	}
}

func (this *RadioButton) Text() string {
	return this.text
}

func (this *RadioButton) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *RadioButton) SetChangedCallback(cb func(interface{}, bool)) {
	this.cbChanged = cb
}

func (this *RadioButton) fireChanged(checked bool) {
	if this.cbChanged != nil {
		this.cbChanged(this.Self(), checked)
	}
}

func (this *RadioButton) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("选中", this.IsChecked, this.SetChecked)
}

// --- Events ---

func (this *RadioButton) OnMouseEnter() {
	this.Self().Update()
}

func (this *RadioButton) OnMouseLeave() {
	this.Self().Update()
}

func (this *RadioButton) OnLeftDown(x, y float64) {
	if !this.IsEnabled() {
		return
	}
	this.SetFocus()
	if !this.checked {
		if this.group != nil {
			this.group.selectButton(this)
		} else {
			this.checked = true
			this.fireChanged(true)
			this.Self().Update()
		}
	}
}

// --- Drawing ---

func (this *RadioButton) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Size()
	m := t.ButtonMargin

	radius := t.CheckBoxSize * 0.5
	cx := m.L + radius
	cy := h * 0.5

	// outer circle
	g.Arc(cx, cy, radius, 0, 2*math.Pi)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// filled inner circle when checked
	if this.checked {
		innerRadius := radius * 0.55
		g.Arc(cx, cy, innerRadius, 0, 2*math.Pi)
		g.SetBrush1(t.HighLightColor)
		g.Fill()
	}

	// text label
	if this.text != "" {
		if this.IsEnabled() {
			g.SetBrush1(t.TextColor)
		} else {
			g.SetBrush1(t.MenuGrayTextColor)
		}
		g.SetFont(t.Font)
		ext := g.Font().TextExtents(this.text)
		xt := m.L + t.CheckBoxSize + m.L - ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(this.text)
		g.Translate(-xt, -yt)
	}
}

// --- SizeHints ---

func (this *RadioButton) SizeHints() SizeHints {
	t := Theme()
	m := t.ButtonMargin

	if this.text != "" {
		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.text)
		h := math.Max(t.CheckBoxSize, fe.Height)
		w := ext.Width + t.CheckBoxSize + m.L
		w += m.L + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	}

	w := m.L + t.CheckBoxSize + m.R
	h := m.T + t.CheckBoxSize + m.B
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}
