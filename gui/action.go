package gui

import (
	"github.com/uk0/silk/core"
	//	"github.com/uk0/silk/factory"
	"github.com/uk0/silk/paint"
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
	Rev() uint64
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
	// rev counts mutations. It is what caches key on, not mtime: two calls to
	// time.Now() a few microseconds apart return the same instant on Windows,
	// so two changes inside one clock tick were indistinguishable and anything
	// keyed on MTime went stale — a button whose text was set twice in a frame
	// kept the first text's size hints and never repainted for the second.
	rev uint64

	targetAction IAction
	cbAction     func(IAction, interface{})
}

func (this *Action) ObjName() string {
	return this.objname
}

func (this *Action) SetObjName(objname string) {
	this.objname = objname
	this.touch()
}

//func (this *Action) Bind(dst interface{}) {
//	this.Callback.Bind(dst)
//	this.targetAction, _ = dst.(IAction)
//	this.touch()
//}

// Text returns the action's display text, or the empty string when no
// text has been set. Earlier versions returned the literal "<EMPTY>"
// placeholder for empty text — that propagated through Button.Text(),
// which made Button.IsTextVisible report true for icon-only buttons,
// which in turn made ToolBar.layoutHorizontal lay out icon-only buttons
// using the wide text-aware formula (~105 px each instead of 36 px).
// The result was icon buttons spread far apart on the silkide toolbar.
// Returning "" lets the icon-only branches fire correctly throughout
// the widget stack.
func (this *Action) Text() string {
	if this.targetAction != nil {
		return this.targetAction.Text()
	}
	return this.text
}

func (this *Action) SetText(text string) {
	if this.targetAction != nil {
		this.targetAction.SetText(text)
		return
	}
	this.text = text
	this.touch()
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
	this.touch()
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
	this.touch()
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
	this.touch()
}

// touch records a mutation. mtime stays for anything that wants to display a
// change time; rev is the ordering-safe half.
func (this *Action) touch() {
	this.mtime = time.Now()
	this.rev++
}

// Rev is the change counter for cache keys. Bound actions fold both sides in:
// either one advancing changes the sum, which is all a key needs.
func (this *Action) Rev() uint64 {
	if this.targetAction == nil {
		return this.rev
	}
	return this.rev + this.targetAction.Rev()
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
	this.touch()
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
	this.touch()
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
