package graph

import (
//	"silk/gui"
)

type _IDecorEx interface {
	IDecor
	//OnPressHandle(handle int, x, y float64)
	OnBeginMoveHandle(handle int, x, y float64)
	OnMoveHandle(handle int, x, y float64)
	OnEndMoveHandle(handle int, x, y float64)
	//OnReleaseHandle(handle int, x, y float64)
	OnClickHandle(handle int, x, y float64)

	IsMovingHandle() bool
	SetMovingHandle(b bool)
	ActiveHandle() int
	SetActiveHandle(n int)

	Update()
}

type DecorPart struct {
	Part
	decor  _IDecorEx
	handle int
}

func NewDecorPart() *DecorPart {
	p := new(DecorPart)
	p.Init(p)
	return p
}

//func (this *DecorPart) ActiveHandle() int {
//	if this.decor == nil {
//		return 0
//	}
//	return this.decor.ActiveHandle()
//}

//func (this *DecorPart) IsMovingHandle() bool {
//	if this.decor == nil {
//		return false
//	}
//	return this.decor.IsMovingHandle()
//}

func (this *DecorPart) OnLeftDown(x, y float64) {
	view := this.View()
	decor, handle := view.FindHandleAt(x, y)
	// core.Debug(core.TypeInfo(decor.Item()), handle)
	if decor == nil {
		this.decor = nil
		this.handle = 0
		return
	}
	this.decor = decor.(_IDecorEx)
	this.handle = handle
	this.decor.SetActiveHandle(handle)
	this.decor.SetMovingHandle(false)
	//	x1, y1 := this.MapToDecor(x, y)
	//	this.decor.OnPressHandle(handle, x1, y1)
	this.decor.Update()
	this.Activate()
}

func (this *DecorPart) MapToDecor(x, y float64) (x1, y1 float64) {
	if this.decor == nil {
		x1, y1 = x, y
		return
	}
	x1, y1 = this.decor.Item().MapFromScene(x, y)
	return
}

func (this *DecorPart) OnLeftDragStart(x, y float64) {
	if this.decor == nil {
		return
	}
	x1, y1 := this.MapToDecor(x, y)
	this.decor.SetMovingHandle(true)
	this.decor.OnBeginMoveHandle(this.handle, x1, y1)
	this.decor.Update()
}

func (this *DecorPart) OnMouseMove(x, y float64) {
	if this.decor == nil || !this.decor.IsMovingHandle() {
		return
	}
	x1, y1 := this.MapToDecor(x, y)
	this.decor.Update()
	this.decor.OnMoveHandle(this.handle, x1, y1)
	this.decor.Update()
}

func (this *DecorPart) OnLeftUp(x, y float64) {
	defer func() {
		this.decor = nil
		this.handle = 0
	}()

	if this.decor == nil || !this.IsActive() {
		return
	}

	x1, y1 := this.MapToDecor(x, y)
	if this.decor.IsMovingHandle() {
		this.decor.OnEndMoveHandle(this.handle, x1, y1)
		this.decor.SetMovingHandle(false)
	}
	//	this.decor.OnReleaseHandle(this.handle, x1, y1)
	this.decor.SetActiveHandle(0)
	this.decor.Update()
	//rect := this.TrackRect().NormalizeCopy()
	//scene := this.Scene()
	//items := scene.FindItemsInRect(rect, nil)
	//selection := this.View().Selection()
	////core.Debug("rect = ", rect)

	//if gui.IsKeyDown(gui.KeyShift) {
	//	selection.InvertMulti(items)
	//} else if gui.IsKeyDown(gui.KeyCtrl) {
	//	selection.AddMulti(items)
	//} else {
	//	selection.Clear()
	//	selection.AddMulti(items)
	//}

	//selection.DebugDump()

	//	this.Deactivate()
	this.View().Update()
}

func (this *DecorPart) HandleSize() float64 {
	return HANDLE_SIZE / this.View().ZoomFactor()
}
