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

// Draw adds the 多值 hint over the empty box a disagreeing selection produces,
// styled like gui.PathEdit's placeholder. Without it an untouched multi-value
// row is indistinguishable from a value that really is the empty string.
func (this *TextEdit) Draw(g paint.Painter) {
	this.Edit.Draw(g)

	hint := this.item.MultiValueHint()
	if hint == "" || this.String() != "" {
		return
	}
	t := gui.Theme()
	fe := t.Font.FontExtents()
	g.SetFont(t.Font)
	g.SetBrush1(t.FormDarkColor)
	g.DrawText1(t.EditPadding.L, (this.Height()+fe.Ascent-fe.Descent)/2, hint)
}

func (this *TextEdit) Activate() {
}

func (this *TextEdit) Deactivate() {
	this.submit()
}

func (this *TextEdit) submit() {
	// A 多值 row starts empty on purpose, and Deactivate submits whatever the
	// box holds: committing that emptiness would blank the property on every
	// selected object just for visiting the row. Only typed text is written.
	if this.String() == "" && this.item.IsMultiValue() {
		return
	}
	this.item.SetValueStr(this.String())
}
