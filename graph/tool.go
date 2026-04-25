package graph

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"time"
)

var (
//	currentToolView IView
)

type ITool interface {
	Self() ITool
	Action() gui.IAction

	Text() string
	SetText(text string)
	Icon() paint.Icon
	SetIcon(icon paint.Icon)
	Name() string
	SetName(name string)
	IsEnabled() bool
	SetEnabled(b bool)
	IsActive() bool

	Activate()

	OnDraw(g paint.Painter)

	OnLeftDown(x, y float64)
	OnLeftUp(x, y float64)
	OnMouseMove(x, y float64)
	OnLeftClick(x, y float64)
	OnLeftDragStart(x, y float64)

	updateMTime()
}

type Tool struct {
	view   *GraphView
	self   ITool
	icon   paint.Icon
	text   string
	tip    string
	name   string
	action *actionForTool

	disabled bool
	checked  bool

	parts      []IPart
	activePart IPart

	downBtn   int     // 0 = nul, 1 = left, 2 = right, 3 = middle
	downX     float64 // mm
	downY     float64 // mm
	curX      float64 // mm
	curY      float64 // mm
	dragStart bool
	mtime     time.Time
}

func (this *Tool) Init(self ITool) {
	this.self = self
}

func (this *Tool) setOwnerView(view *GraphView) {
	if this.view != nil && this.view != view {
		panic("Tool already binded to anthor view")
	}
	this.view = view
	this.updateMTime()
}

func (this *Tool) GraphView() IView {
	return this.view
}

func (this *Tool) Scene() IScene {
	return this.view.Scene()
}

func (this *Tool) Self() ITool {
	return this.self
}

func (this *Tool) Action() gui.IAction {
	if this.action == nil {
		this.action = &actionForTool{this}
	}
	return this.action
}

func (this *Tool) Text() string {
	return this.text
}

func (this *Tool) SetText(text string) {
	this.text = text
	this.updateMTime()
}

func (this *Tool) Icon() paint.Icon {
	if this.icon == nil && this.text == "" {
		return gui.LoadIcon("error")
	}
	return this.icon
}

func (this *Tool) SetIcon(icon paint.Icon) {
	this.icon = icon
	this.updateMTime()
}

func (this *Tool) Name() string {
	return this.name
}

func (this *Tool) SetName(name string) {
	this.name = name
	this.updateMTime()
}

func (this *Tool) IsEnabled() bool {
	return !this.disabled
}

func (this *Tool) SetEnabled(b bool) {
	this.disabled = !b
	this.updateMTime()
}

func (this *Tool) IsActive() bool {
	return this.view != nil && this.view.ActiveTool() == this.Self()
}

func (this *Tool) Activate() {
	if this.view == nil {
		core.Warn("Tool is not in view")
		return
	}
	this.view.SetActiveTool(this.Self())
}

func (this *Tool) SetIcon1(s string) {
	this.SetIcon(LoadIcon(s))
}

func (this *Tool) addPart(part IPart) {
	//this.parts = append(this.parts, part...)
	im := part.(interface {
		setOwnerTool(*Tool)
	})
	im.setOwnerTool(this)
	this.parts = append(this.parts, part)
	this.updateMTime()
}

func (this *Tool) AddPart(part ...IPart) {
	for _, p := range part {
		this.addPart(p)
	}
}

func (this *Tool) OnLeftDown(x, y float64) {
	this.deactivatePart()
	this.curX, this.curY = x, y
	this.downBtn = 1
	this.dragStart = false
	this.downX = x
	this.downY = y

	if this.parts == nil {
		core.Warn("Tool is empty, nothing to do")
		return
	}

	gp := this.activePart
	if gp != nil {
		if i, ok := gp.(interface {
			OnLeftDown(x, y float64)
		}); ok {
			i.OnLeftDown(x, y)
		}
		if this.activePart != nil {
			return
		}
	}

	for _, p := range this.parts {
		if p == gp {
			continue
		}

		if i, ok := p.(interface {
			OnLeftDown(x, y float64)
		}); ok {
			i.OnLeftDown(x, y)
		}
		if this.activePart != nil {
			return
		}
	}
}

func (this *Tool) OnMouseMove(x, y float64) {
	this.curX, this.curY = x, y
	//core.Debug("x, y = ", x, y)
	if this.parts == nil {
		core.Warn("Tool is empty, nothing to do")
		return
	}

	gp := this.activePart
	if gp != nil {
		if i, ok := gp.(interface {
			OnMouseMove(x, y float64)
		}); ok {
			i.OnMouseMove(x, y)
		}
		if this.activePart != nil {
			return
		}
	}

	for _, p := range this.parts {
		if p == gp {
			continue
		}

		if i, ok := p.(interface {
			OnMouseMove(x, y float64)
		}); ok {
			i.OnMouseMove(x, y)
		}
		if this.activePart != nil {
			return
		}
	}
}

func (this *Tool) OnLeftClick(x, y float64) {
	this.curX, this.curY = x, y
	if this.parts == nil {
		core.Warn("Tool is empty, nothing to do")
		return
	}
	//if i, ok := this.activePart.(interface {
	//	OnLeftClick(x, y float64)
	//}); ok {
	//	i.OnLeftClick(x, y)
	//}
	gp := this.activePart
	if gp != nil {
		if i, ok := gp.(interface {
			OnLeftClick(x, y float64)
		}); ok {
			i.OnLeftClick(x, y)
		}
		if this.activePart != nil {
			return
		}
	}

	for _, p := range this.parts {
		if p == gp {
			continue
		}

		if i, ok := p.(interface {
			OnLeftClick(x, y float64)
		}); ok {
			i.OnLeftClick(x, y)
		}
		if this.activePart != nil {
			return
		}
	}
}

func (this *Tool) OnLeftDragStart(x, y float64) {
	this.curX, this.curY = x, y
	this.dragStart = true
	if this.parts == nil {
		core.Warn("Tool is empty, nothing to do")
		return
	}

	//if i, ok := this.activePart.(interface {
	//	OnLeftDragStart(x, y float64)
	//}); ok {
	//	i.OnLeftDragStart(x, y)
	//}

	gp := this.activePart
	if gp != nil {
		if i, ok := gp.(interface {
			OnLeftDragStart(x, y float64)
		}); ok {
			i.OnLeftDragStart(x, y)
		}
		if this.activePart != nil {
			return
		}
	}

	for _, p := range this.parts {
		if p == gp {
			continue
		}

		if i, ok := p.(interface {
			OnLeftDragStart(x, y float64)
		}); ok {
			i.OnLeftDragStart(x, y)
		}
		if this.activePart != nil {
			return
		}
	}
}

func (this *Tool) OnLeftUp(x, y float64) {
	this.curX, this.curY = x, y
	defer func(this *Tool) {
		this.downBtn = 0
		this.dragStart = false
		this.deactivatePart()
	}(this)

	if this.parts == nil {
		core.Warn("Tool is empty, nothing to do")
		return
	}

	//if this.activePart == nil {
	//	return
	//}

	if i, ok := this.activePart.(interface {
		OnLeftUp(x, y float64)
	}); ok {
		i.OnLeftUp(x, y)
	}
	//if this.activePart != nil {
	//	return
	//}

	//for _, p := range this.parts {
	//	if p == gp {
	//		continue
	//	}

	//	if i, ok := p.(interface {
	//		OnLeftUp(x, y float64)
	//	}); ok {
	//		i.OnLeftUp(x, y)
	//	}
	//	if this.activePart != nil {
	//		return
	//	}
	//}
}

func (this *Tool) OnDraw(g paint.Painter) {
	if im, ok := this.activePart.(interface {
		OnDraw(paint.Painter)
	}); ok {
		im.OnDraw(g)
	}
}

func (this *Tool) DownPos() (x, y float64) {
	return this.downX, this.downY
}

func (this *Tool) CurrentPos() (x, y float64) {
	return this.curX, this.curY
}

func (this *Tool) deactivatePart() {
	if this.activePart != nil {
		core.Debug("tool part deactivated: ", core.ObjInfo(this.activePart))
		this.activePart = nil
	}
}

func (this *Tool) ActivePart() IPart {
	return this.activePart
}

func (this *Tool) updateMTime() {
	this.mtime = time.Now()
}

//////////////////////////////////////////////////////////
type actionForTool struct {
	tool *Tool
}

func (this *actionForTool) Text() string {
	return this.tool.Self().Text()
}

func (this *actionForTool) SetText(text string) {
	this.tool.Self().SetText(text)
}

func (this *actionForTool) Icon() paint.Icon {
	return this.tool.Self().Icon()
}

func (this *actionForTool) SetIcon(icon paint.Icon) {
	this.tool.Self().SetIcon(icon)
}

func (this *actionForTool) ObjName() string {
	return this.tool.Self().Name()
}

func (this *actionForTool) SetObjName(name string) {
	this.tool.Self().SetName(name)
}

func (this *actionForTool) IsEnabled() bool {
	return this.tool.Self().IsEnabled()
}

func (this *actionForTool) SetEnabled(b bool) {
	this.tool.Self().SetEnabled(b)
}

func (this *actionForTool) IsChecked() bool {
	return this.tool.Self().IsActive()
}

func (this *actionForTool) SetChecked(b bool) {
	if b {
		this.tool.Self().Activate()
	} else if this.tool.view != nil {
		this.tool.view.SetActiveTool(this.tool.Self())
	} else {
		core.Warn("Tool is not in view")
	}
}

//func (this *actionForTool) Target() interface{} {
//	return this.tool.Self()
//}

func (this *actionForTool) BindFunc(fn func(gui.IAction, interface{})) {

}

func (this *actionForTool) BindFunc0(fn func()) {

}

func (this *actionForTool) BindFunc1(fn func(gui.IAction)) {

}

func (this *actionForTool) BindAction(a gui.IAction) {

}

func (this *actionForTool) MTime() (ret time.Time) {
	return this.tool.mtime
}

func (this *actionForTool) Trigger(sender interface{}) {
	this.tool.Self().Activate()
}

func (this *actionForTool) SetExtra(a interface{}) {
}

func (this *actionForTool) Extra() interface{} {

	return nil
}
