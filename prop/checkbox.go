package prop

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	//"github.com/uk0/silk/paint"
	//"github.com/uk0/silk/persist"
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
	// A tick has no empty state, so a disagreeing selection says so in the
	// label instead — an unticked box would read as "they are all off".
	this.SetText(this.item.MultiValueHint())
	this.SetChecked(this.item.GetValue().(bool))
}

func (this *CheckBox) UpdateConfig() {
	// A read-only property must not look editable: PropertyItem.SetValue drops
	// the write, so an enabled box flips its tick and then shows a value the
	// object never took. gui.CheckBox gates its click and key paths on
	// IsEnabled. Mirrors TextEdit.UpdateConfig's SetReadOnly.
	this.SetEnabled(!this.item.IsReadOnly())
}

func (this *CheckBox) Activate() {
}

func (this *CheckBox) Deactivate() {
}

func (this *CheckBox) OnCheckChanged() {
	this.item.SetValue(this.IsChecked())
	// The whole selection now holds this tick, so the 多值 label is stale.
	this.SetText(this.item.MultiValueHint())
}
