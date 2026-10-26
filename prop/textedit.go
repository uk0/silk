package prop

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	//"github.com/uk0/silk/persist"
)

func init() {
	core.RegisterFactory("prop.control.TextEdit", core.TypeOf((*TextEdit)(nil)))
}

type TextEdit struct {
	gui.Edit
	item        *PropertyItem1
	editSession *propertyEditSession
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
	this.submitText(s)
}

func (this *TextEdit) OnSubmit(s string) {
	this.Deactivate()
}

func (this *TextEdit) BindProperty(item *PropertyItem1) {
	this.finishEdit()
	this.item = item
}

func (this *TextEdit) UpdateValue() {
	this.finishEdit()
	this.updateValueText()
}

func (this *TextEdit) updateValueText() {
	if this.item == nil {
		return
	}
	s := this.item.GetValueStr()
	this.SetText(s)
	this.updateHintTooltip()
}

func (this *TextEdit) UpdateConfig() {
	this.SetReadOnly(this.item.IsReadOnly())
}

// Draw adds the 多值 hint over the empty box a disagreeing selection produces,
// styled like gui.PathEdit's placeholder. Without it an untouched multi-value
// row is indistinguishable from a value that really is the empty string.
func (this *TextEdit) Draw(g paint.Painter) {
	this.Edit.Draw(g)

	if this.item == nil {
		return
	}
	hint := this.item.MultiValueHint()
	if hint == "" || this.String() != "" {
		return
	}
	t := gui.Theme()
	drawPropertyText(g, t.Font, t.TextColor, t.EditPadding.L, 0, this.Width()-t.EditPadding.L-t.EditPadding.R, this.Height(), hint)
}

func (this *TextEdit) Activate() {
}

func (this *TextEdit) Deactivate() {
	this.submit()
	this.finishEdit()
}

func (this *TextEdit) submit() { this.submitText(this.String()) }

func (this *TextEdit) submitText(text string) {
	if !this.canEditProperty() {
		return
	}
	// A 多值 row starts empty on purpose, and Deactivate submits whatever the
	// box holds: committing that emptiness would blank the property on every
	// selected object just for visiting the row. Only typed text is written.
	if text == "" && this.item.IsMultiValue() {
		return
	}
	value, err := parsePropertyValue(this.item.Type(), text)
	if err != nil {
		return
	}
	this.setEditedValue(value)
	this.updateHintTooltip()
}

func (this *TextEdit) updateHintTooltip() {
	if this.item != nil {
		gui.SetToolTip(this.Self(), this.item.Label()+"\n"+this.item.MultiValueHint())
	}
}
