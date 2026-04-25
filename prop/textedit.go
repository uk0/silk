package prop

import (
	"silk/core"
	"silk/gui"
	//"silk/paint"
	//"silk/persist"
)

func init() {
	core.RegisterFactory("prop.control.TextEdit", core.TypeOf((*TextEdit)(nil)))
}

type TextEdit struct {
	gui.Edit
	item *PropertyItem1
}

func NewTextEdit() *TextEdit {
	p := new(TextEdit)
	p.Init(p)
	return p
}

func (this *TextEdit) Init(self gui.IWidget) {
	this.Edit.Init(self)
	core.Connect2(this.SigTextEdited, this, "OnEdited")
	core.Connect3(this, "Submit", this.OnSubmit)
}

func (this *TextEdit) OnEdited(s string) {
	this.submit()
}

func (this *TextEdit) OnSubmit(s string) {
	this.submit()
}

func (this *TextEdit) BindProperty(item *PropertyItem1) {
	this.item = item
}

func (this *TextEdit) UpdateValue() {
	s := this.item.GetValueStr()
	this.SetText(s)
}

func (this *TextEdit) UpdateConfig() {
	this.SetReadOnly(this.item.IsReadOnly())
}

func (this *TextEdit) Activate() {
}

func (this *TextEdit) Deactivate() {
	this.submit()
}

func (this *TextEdit) submit() {
	this.item.SetValueStr(this.String())
}
