package graph

import (
	//	"silk/geom"
	"silk/core"
	"silk/gui"
)

type IScene interface {
	IItem

	NakedScene() *SceneItem

	View() IView

	UndoStack() gui.IUndoStack

	SigItemAttached(func(s interface{}, item, parent IItem))
	SigItemDetached(func(s interface{}, item, parent IItem))

	SetPropertyConfigName(cfgName string)
	PropertyConfigName() (cfgName string)

	DirtyList() []string
	Save() bool

	PushCommand(cmd gui.ICommand)

	SetTitle(s string)

	IsClean() bool
}

//type SceneItem struct {

//}

//func (this *Doc) Draw(p paint.Painter) {
//	//	root.Draw()
//}

//func (this *Doc) DrawPartial(p paint.Painter, rect geom.Rect) {

//}

type SceneItem struct {
	Item
	view      IView // 目前只支持单视图
	undoStack gui.IUndoStack

	cbItemAttached func(s interface{}, item, parent IItem)
	cbItemDetached func(s interface{}, item, parent IItem)

	propertyCfgName string

	title string

	resizing bool
	moving   bool
}

func (this *SceneItem) Init(i IItem) {
	this.Item.Init(i)
	this.SetSelectable(false)
	this.SetLockPos(true)
	this.SetLockSize(true)
	this.SetLocalCoord(true)
	this.title = "untitled"
}

//func (this *SceneItem) OnSelectItem(a IItem) {

//}

//func (this *SceneItem) OnDeselectItem(a IItem) {

//}

//func (this *SceneItem) OnSelectionChanged(a IItem) {

//}

func (this *SceneItem) Update() {
	if this.view != nil {
		this.view.Update()
	}
}

func (this *SceneItem) View() IView {
	return this.view
}

func (this *SceneItem) setView(a IView) {
	this.view = a
}

func (this *SceneItem) UndoStack() gui.IUndoStack {
	if this.undoStack == nil {
		this.undoStack = gui.NewUndoStack("graph")
	}
	return this.undoStack
}

func (this *SceneItem) PushCommand(cmd gui.ICommand) {
	this.UndoStack().Push(cmd)
}

//func (this *SceneItem) OnItemAttached(parent, item IItem) {

//}

//func (this *SceneItem) OnItemDetached(parent, item IItem) {

//}

func (this *SceneItem) emitItemAttached(item, parent IItem) {
	if this.cbItemAttached != nil {
		this.cbItemAttached(this.Self(), item, parent)
	}
	if this.view == nil {
		return
	}
	this.view.emitItemAttached(parent, item)
}

func (this *SceneItem) emitItemDetached(item, parent IItem) {
	if this.cbItemDetached != nil {
		this.cbItemDetached(this.Self(), item, parent)
	}
	if this.view == nil {
		return
	}
	this.view.emitItemDetached(parent, item)

}

func (this *SceneItem) SigItemAttached(fn func(s interface{}, item, parent IItem)) {
	this.cbItemAttached = fn
}

func (this *SceneItem) SigItemDetached(fn func(s interface{}, item, parent IItem)) {
	this.cbItemDetached = fn
}

func (this *SceneItem) NakedScene() *SceneItem {
	return this
}

func (this *SceneItem) SetPropertyConfigName(cfgName string) {
	this.propertyCfgName = cfgName
}

func (this *SceneItem) PropertyConfigName() (cfgName string) {
	if this.propertyCfgName != "" {
		return this.propertyCfgName
	}
	return "default"
}

func (this *SceneItem) DirtyList() []string {
	if this.IsClean() {
		return nil
	}
	return []string{this.Self().Title()}
}

func (this *SceneItem) IsClean() bool {
	return this.undoStack.IsClean()
}

func (this *SceneItem) Title() string {
	if this.IsClean() {
		return this.title
	}
	return this.title + " *"
}

func (this *SceneItem) SetTitle(t string) {
	this.title = t
}

func (this *SceneItem) Save() bool {
	core.Debug("SceneItem.Save() do nothing")
	return true
}

func (this *SceneItem) OnResize() {
	if this.resizing {
		return
	}

	this.resizing = true
	defer func() { this.resizing = false }()

	// 当场景尺寸改变时, 要重新布局视图, 是页面尺寸和场景尺寸相符
	if this.view != nil {
		this.view.Layout()
	}

	this.Self().Layout()
}

func (this *SceneItem) OnMove() {
	if this.moving {
		return
	}

	this.moving = true
	defer func() { this.moving = false }()

	// 当场景移动时, 要重新布局视图, 把场景移回(0,0)
	if this.view != nil {
		this.view.Layout()
	}

	if !this.localCoord {
		this.Self().Layout()
	}
}
