package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.Form", core.TypeOf((*Form)(nil))) //((*Form)(nil)))
}

// 表单
// 用界面编辑器编辑界面时的容器, 控件要放在表单上
// 程序里加载上来后, 可作为对话框显示, 也可嵌到其他控件中
type Form struct {
	Widget

	icon  paint.Icon
	title string
}

func NewForm() *Form {
	p := new(Form)
	p.Init(p)
	return p
}

func (this *Form) SetTitle(s string) {
	this.title = s
}

func (this *Form) Title() string {
	return this.title
}

func (this *Form) SetIcon(icon paint.Icon) {
	this.icon = icon
}

func (this *Form) Icon() paint.Icon {
	return this.icon
}

func (this *Form) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
}

func (this *Form) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, this.Width(), this.Height())
	g.Fill()
}

func (this *Form) LoadGui(doc *core.TDoc) error {
	return nil
}
