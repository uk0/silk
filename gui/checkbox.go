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
