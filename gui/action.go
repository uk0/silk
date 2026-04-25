package gui

import (
	"silk/core"
	//	"silk/factory"
	"silk/paint"
	"sort"
	"time"
)

func init() {
	core.RegisterFactory("gui.Action", core.TypeOf((*Action)(nil)))
}

func NewAction() *Action {
	p := new(Action)
	return p
}

func NewAction1(txt string, icon paint.Icon) *Action {
	p := NewAction()
	p.SetText(txt)
	p.SetIcon(icon)
	return p
}

// Action 是按钮/菜单等命令的抽象
type IAction interface {
	//core.ICallback
	Text() string
	SetText(text string)
	Icon() paint.Icon
	SetIcon(icon paint.Icon)
	ObjName() string
	SetObjName(objname string)
	IsEnabled() bool
	SetEnabled(b bool)
	IsChecked() bool
	SetChecked(b bool)
	MTime() time.Time
	Trigger(sender interface{})

	BindFunc(fn func(IAction, interface{}))
	BindFunc0(fn func())
	BindFunc1(fn func(IAction))

	BindAction(a IAction)

	SetExtra(a interface{})
	Extra() interface{}
}

// Action 是按钮/菜单等命令的抽象
type Action struct {
	//core.Callback
	icon    paint.Icon
	objname string
	text    string
	extra   interface{}

	disabled bool
	checked  bool
	mtime    time.Time

	targetAction IAction
	cbAction     func(IAction, interface{})
}

func (this *Action) ObjName() string {
	return this.objname
}

func (this *Action) SetObjName(objname string) {
	this.objname = objname
	this.mtime = time.Now()
}

//func (this *Action) Bind(dst interface{}) {
//	this.Callback.Bind(dst)
//	this.targetAction, _ = dst.(IAction)
//	this.mtime = time.Now()
//}

func (this *Action) Text() string {
	if this.targetAction != nil {
		return this.targetAction.Text()
	}
	if this.text == "" {
		return "<EMPTY>"
	}
	return this.text
}

func (this *Action) SetText(text string) {
	if this.targetAction != nil {
		this.targetAction.SetText(text)
		return
	}
	this.text = text
	this.mtime = time.Now()
}

func (this *Action) Icon() paint.Icon {
	if this.targetAction != nil {
		return this.targetAction.Icon()
	}
	//if this.text == "" && this.icon == nil {
	//	return LoadIcon("error")
	//}
	return this.icon
}

func (this *Action) SetIcon(icon paint.Icon) {
	if this.targetAction != nil {
		this.targetAction.SetIcon(icon)
		return
	}
	this.icon = icon
	this.mtime = time.Now()
}

func (this *Action) IsEnabled() bool {
	if this.targetAction != nil {
		return this.targetAction.IsEnabled()
	}
	return !this.disabled
}

func (this *Action) SetEnabled(b bool) {
	if this.targetAction != nil {
		this.targetAction.SetEnabled(b)
		return
	}
	this.disabled = !b
	this.mtime = time.Now()
}

func (this *Action) IsChecked() bool {
	if this.targetAction != nil {
		return this.targetAction.IsChecked()
	}
	return this.checked
}

func (this *Action) SetChecked(b bool) {
	if this.targetAction != nil {
		this.targetAction.SetChecked(b)
		return
	}
	this.checked = b
	this.mtime = time.Now()
}

func (this *Action) MTime() time.Time {
	if this.targetAction == nil {
		return this.mtime
	}
	t1 := this.targetAction.MTime()
	t2 := this.mtime
	if t1.Before(t2) {
		return t2
	}
	return t1
}

func (this *Action) BindFunc(fn func(IAction, interface{})) {
	this.cbAction = fn
	this.mtime = time.Now()
}

func (this *Action) BindFunc0(fn func()) {
	this.BindFunc(func(IAction, interface{}) {
		fn()
	})
}

func (this *Action) BindFunc1(fn func(IAction)) {
	this.BindFunc(func(a IAction, b interface{}) {
		fn(a)
	})
}

func (this *Action) BindAction(a IAction) {
	this.targetAction = a
	this.mtime = time.Now()
}

func (this *Action) Trigger(sender interface{}) {
	if this.targetAction != nil {
		this.targetAction.Trigger(sender)
	}
	if this.cbAction != nil {
		this.cbAction(this, sender)
	}
}

func (this *Action) SetExtra(a interface{}) {
	this.extra = a
}

func (this *Action) Extra() interface{} {
	return this.extra
}

type sortActions []IAction

func (v sortActions) Len() int {
	return len(v)
}

func (v sortActions) Less(i, j int) bool {
	return v[i].Text() < v[j].Text()
}

func (v sortActions) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func SortActions(a []IAction) {
	sort.Sort(sortActions(a))
}
