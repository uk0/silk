package graph

import (
	//"silk/core"
	//	"silk/gui"
	"silk/paint"
)

type MovePart struct {
	Part
	cond TraversalCond
	item IItem
}

func NewMovePart() *MovePart {
	p := new(MovePart)
	p.cond = TraversalCond_SelectableAndMoveable
	p.Init(p)
	return p
}

func (this *MovePart) OnLeftDragStart(x, y float64) {
	scene := this.Scene()
	downX, downY := this.DownPos()

	this.item = scene.FindItemAt(downX, downY, this.cond)

	if this.item == nil {
		return
	}

	selection := this.View().Selection()
	if !selection.Contains(this.item) {
		selection.Clear()
		selection.Add(this.item)
	}
	this.Activate()
	this.View().Update()
}

func (this *MovePart) OnMouseMove(x, y float64) {
	this.View().Update()
}

func (this *MovePart) OnLeftUp(x, y float64) {
	dx := this.TrackRect().Width
	dy := this.TrackRect().Height
	if dx != 0 || dy != 0 {
		cmd := this.View().Selection().GenerateMoveCommand(dx, dy)
		if cmd != nil {
			this.Scene().UndoStack().Push(cmd)
		}
	}
	this.View().Update()
	this.item = nil
}

func (this *MovePart) OnDraw(g paint.Painter) {

}
