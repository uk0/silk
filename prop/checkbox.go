package prop

import (
	"silk/core"
	"silk/gui"
	//"silk/paint"
	//"silk/persist"
)

func init() {
	core.RegisterFactory("prop.control.CheckBox", core.TypeOf((*CheckBox)(nil)))
}

type CheckBox struct {
	gui.CheckBox
	item *PropertyItem1
}

func NewCheckBox() *CheckBox {
	p := new(CheckBox)
	p.Init(p)
	return p
}

func (this *CheckBox) BindProperty(item *PropertyItem1) {
	this.item = item
}

func (this *CheckBox) UpdateValue() {
	this.SetChecked(this.item.GetValue().(bool))
}

func (this *CheckBox) UpdateConfig() {
}

func (this *CheckBox) Activate() {
}

func (this *CheckBox) Deactivate() {
}

func (this *CheckBox) OnCheckChanged() {
	this.item.SetValue(this.IsChecked())
}
