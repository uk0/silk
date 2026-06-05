package gui

import (
	"silk/core"
	"silk/paint"
	"math"
	//	"time"
)

// 勾选框, 多选框
type CheckBox struct {
	Widget
	pushed   bool
	checked  bool
	text     string
	cbCheck  func(bool)
	readonly bool
}

func init() {
	core.RegisterFactory("gui.CheckBox", core.TypeOf((*CheckBox)(nil)))
}

func NewCheckBox() *CheckBox {
	p := new(CheckBox)
	p.Init(p)
	return p
}

func (this *CheckBox) Draw(g paint.Painter) {
	Theme().DrawCheckBox(g, this)
	if this.HasFocus() {
		this.drawFocusRing(g)
	}
}

// drawFocusRing paints a subtle accent outline around the check icon while the
// box holds keyboard focus, so keyboard users can see where they are. It uses
// the theme highlight color at low alpha (the same accent the edit frame uses
// when focused) and stays within the icon margins.
func (this *CheckBox) drawFocusRing(g paint.Painter) {
	t := Theme()
	_, h := this.Size()
	m := t.ButtonMargin
	// Halo the icon box with a small inset so the ring reads as a focus cue.
	pad := 2.0
	x := m.L - pad
	y := 0.5*(h-t.IconSize) - pad
	w := t.IconSize + pad*2
	rh := t.IconSize + pad*2
	c := t.HighLightColor
	c.A = 90 // low alpha keeps it subtle
	roundedRect(g, x, y, w, rh, 4)
	g.SetPen1(c, 1.5)
	g.Stroke()
}

func (this *CheckBox) OnMouseEnter() {
	this.Self().Update()
}

func (this *CheckBox) OnMouseLeave() {
	this.Self().Update()
}

func (this *CheckBox) OnLeftDown(x, y float64) {
	if this.IsEnabled() {
		this.pushed = true
		this.SetFocus()
		this.Self().Update()
	}
}

func (this *CheckBox) OnLeftUp(x, y float64) {
	pushed := this.pushed
	this.pushed = false
	this.Self().Update()
	this.PopCapture()
	if pushed && this.IsHover() && this.IsEnabled() {
		this.Toggle()
	}
}

// OnKeyDown implements IEventKeyDown, giving the check box Qt QCheckBox style
// keyboard control while it holds focus: Space (and Enter, for convenience)
// toggles the checked state. The widget is not tri-state, so this is a plain
// toggle. It routes through Toggle so the change callback fires exactly as a
// click does. Guarded on IsEnabled so a disabled box ignores keys.
func (this *CheckBox) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	switch key {
	case KeySpace, KeyEnter:
		this.Toggle()
	}
}

func (this *CheckBox) Toggle() {
	this.checked = !this.checked
	if im, ok := this.Self().(interface {
		OnCheckChanged()
	}); ok {
		im.OnCheckChanged()
	}
	if this.cbCheck != nil {
		this.cbCheck(this.checked)
	}
	this.Self().Update()
}

func (this *CheckBox) Text() string {
	return this.text
}

func (this *CheckBox) Icon() paint.Icon {
	if this.checked {
		return Theme().CheckedIcon
	} else {
		return Theme().UncheckedIcon
	}
}

func (this *CheckBox) IsEnabled() bool {
	return !this.readonly
}

func (this *CheckBox) SetEnabled(b bool) {
	this.readonly = !b
	this.Update()
}

func (this *CheckBox) SizeHints() SizeHints {
	t := Theme()

	if this.text != "" {

		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.Text())

		m := t.ButtonMargin
		h := math.Max(t.IconSize, fe.Height)
		w := ext.Width + h
		w += m.L*2 + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}

	} else {
		m := t.ButtonMargin
		w := m.L + t.IconSize + m.R
		h := m.T + t.IconSize + m.B
		//core.Debug(w, h)
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	}
}

func (this *CheckBox) SetText(text string) {
	this.text = text
	this.Update()
}

func (this *CheckBox) IsPushed() bool {
	return this.pushed
}

func (this *CheckBox) IsChecked() bool {
	return this.checked
}

func (this *CheckBox) SetChecked(b bool) {
	if this.checked != b {
		this.Toggle()
	}
}

func (this *CheckBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("选中", this.IsChecked, this.SetChecked)
	list.AddProperty("可用", this.IsEnabled, this.SetEnabled)
}

func (this *CheckBox) SigCheck(fn func(bool)) {
	this.cbCheck = fn
}
