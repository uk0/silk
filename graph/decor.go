package graph

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
)

const HANDLE_SIZE = 3.2

var hoverHandle int
var hoverDecor IDecor

var normalHandleIcons = [3]paint.Icon{
	LoadIcon("handle-0-normal"),
	LoadIcon("handle-1-normal"),
	LoadIcon("handle-2-normal"),
}

var activeHandleIcons = [3]paint.Icon{
	LoadIcon("handle-0-active"),
	LoadIcon("handle-1-active"),
	LoadIcon("handle-2-active"),
}

func HandleIcon(shape int, active bool) paint.Icon {
	if active {
		return activeHandleIcons[shape%len(activeHandleIcons)]
	} else {
		return normalHandleIcons[shape%len(normalHandleIcons)]
	}
}

type IDecor interface {
	setItem(item IItem)
	setView(view IView)

	HandleAt(x, y float64) int
	HandleCursor(handle int) *gui.Cursor
	OnDraw(g paint.Painter)

	NakedDecor() *Decor

	Item() IItem
	View() IView
}

type Decor struct {
	item IItem
	view IView
	self IDecor

	activeHandle int
	isMoving     bool
}

func (this *Decor) Init(self IDecor) {
	this.self = this
}

func (this *Decor) HandleSize() float64 {
	return HANDLE_SIZE / this.view.ZoomFactor()
}

func (this *Decor) setItem(item IItem) {
	this.item = item
}

func (this *Decor) setView(view IView) {
	this.view = view
}

func (this *Decor) HandleAt(x, y float64) int {
	return 0
}

func (this *Decor) DrawDefaultHandle(g paint.Painter, x, y float64, shape int, active bool) {
	w := this.HandleSize()
	hw := w * 0.5
	x0, y0 := x-hw, y-hw
	g.DrawIcon1(HandleIcon(shape, active), x0, y0, w, false)

}

//func (this *Decor) OnPressHandle(handle int, x, y float64) {

//}

func (this *Decor) OnBeginMoveHandle(handle int, x, y float64) {

}

func (this *Decor) OnMoveHandle(handle int, x, y float64) {

}

func (this *Decor) OnEndMoveHandle(handle int, x, y float64) {

}

//func (this *Decor) OnReleaseHandle(handle int, x, y float64) {

//}

func (this *Decor) OnClickHandle(handle int, x, y float64) {

}

func (this *Decor) HandleCursor(handle int) *gui.Cursor {
	return gui.DefaultCursor()
}

func (this *Decor) OnDraw(g paint.Painter) {
	this.item.DrawOutline(g)
}

func (this *Decor) IsHitCircleHandle(xc, yc, x, y float64) bool {
	return this.IsHitCircleHandle1(xc, yc, this.HandleSize()*0.5, x, y)
}

func (this *Decor) IsHitRectHandle(xc, yc, x, y float64) bool {
	return this.IsHitRectHandle1(xc, yc, this.HandleSize()*0.5, x, y)
}

func (this *Decor) IsHitDiamonHandle(xc, yc, x, y float64) bool {
	return this.IsHitDiamonHandle1(xc, yc, this.HandleSize()*0.5, x, y)
}

func (this *Decor) IsHitCircleHandle1(xc, yc, r, x, y float64) bool {
	dx, dy := x-xc, y-yc
	return dx*dx+dy*dy <= r*r
}

func (this *Decor) IsHitRectHandle1(xc, yc, r, x, y float64) bool {
	dx, dy := math.Abs(x-xc), math.Abs(y-yc)
	return dx <= r && dy <= r
}

func (this *Decor) IsHitDiamonHandle1(xc, yc, r, x, y float64) bool {
	dx, dy := math.Abs(x-xc), math.Abs(y-yc)
	return dx+dy <= r
}

func (this *Decor) HoverHandle() int {
	if hoverDecor != this.self {
		return 0
	}
	return hoverHandle
}

func (this *Decor) ActiveHandle() int {
	return this.activeHandle
}

func (this *Decor) SetActiveHandle(n int) {
	this.activeHandle = n
}

func (this *Decor) NakedDecor() *Decor {
	return this
}

func (this *Decor) Item() IItem {
	return this.item
}

func (this *Decor) View() IView {
	return this.view
}

func (this *Decor) IsMovingHandle() bool {
	return this.isMoving
}

func (this *Decor) SetMovingHandle(b bool) {
	this.isMoving = b
}

func (this *Decor) Update() {
	this.item.Update()
}

func (this *Decor) Scene() IScene {
	return this.item.Scene()
}
