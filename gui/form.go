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

	icon       paint.Icon
	title      string
	layoutRoot IWidget
}

func NewForm() *Form {
	p := new(Form)
	p.Init(p)
	return p
}

// SetLayoutRoot designates an already attached, active layout as the form's
// client-area layout. Other direct children are left alone. Passing nil clears
// the policy. It never reparents a widget or creates another native window.
func (this *Form) SetLayoutRoot(widget IWidget) bool {
	if widget != nil && !this.isLayoutRoot(widget) {
		return false
	}
	this.layoutRoot = widget
	if widget != nil {
		before := widget.Bounds1()
		this.Layout()
		// A just-initialized generated form may already have the right
		// size, but its latest content/properties still need final layout.
		if widget.Bounds1() == before {
			widget.NakedWidget().layoutNow()
		}
	}
	return true
}

func (this *Form) isLayoutRoot(widget IWidget) bool {
	if widget == nil || widget.Parent() != this.Self() {
		return false
	}
	manager, ok := widget.(interface{ LayoutEnabled() bool })
	return ok && manager.LayoutEnabled()
}

func (this *Form) releaseLayoutRoot(widget IWidget) {
	if this.layoutRoot == widget {
		this.layoutRoot = nil
	}
}

// LayoutRoot returns the active layout, or nil if none is attached.
func (this *Form) LayoutRoot() IWidget {
	if !this.isLayoutRoot(this.layoutRoot) {
		return nil
	}
	return this.layoutRoot
}

// Layout keeps the designated layout within the logical client area. The
// normal Widget resize path calls it for both embedded and native forms.
func (this *Form) Layout() {
	if child := this.LayoutRoot(); child != nil {
		child.SetBounds(0, 0, this.Width(), this.Height())
	}
}

func (this *Form) SetTitle(s string) {
	this.title = s
}

func (this *Form) Title() string {
	return this.title
}

// SetIcon selects this form's icon. Nil/air icons inherit from its owner or
// the application when attached. Updating an attached form is UI-thread-only.
func (this *Form) SetIcon(icon paint.Icon) {
	this.icon = icon
	if win := this.Window(); win != nil && win.IsValid() {
		if selected := initialWindowIcon(this.Self(), win.ParentWindow()); selected != nil {
			win.SetIcon(selected)
		} else {
			win.clearIcon()
		}
	}
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
