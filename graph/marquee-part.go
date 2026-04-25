package graph

import (
	//	"silk/core"
	"silk/gui"
	"silk/paint"
)

type MarqueePart struct {
	Part
	cond TraversalCond
}

func NewMarqueePart() *MarqueePart {
	p := new(MarqueePart)
	p.Init(p)
	return p
}

func (this *MarqueePart) OnLeftDragStart(x, y float64) {
	this.Activate()
	this.View().Update()
}

func (this *MarqueePart) OnMouseMove(x, y float64) {
	this.View().Update()
}

func (this *MarqueePart) OnLeftUp(x, y float64) {
	if !this.IsActive() {
		return
	}
	rect := this.TrackRect().NormalizeCopy()
	scene := this.Scene()
	items := scene.FindItemsInRect(rect, nil)
	selection := this.View().Selection()
	//core.Debug("rect = ", rect)

	if gui.IsKeyDown(gui.KeyShift) {
		selection.InvertMulti(items)
	} else if gui.IsKeyDown(gui.KeyCtrl) {
		selection.AddMulti(items)
	} else {
		selection.Clear()
		selection.AddMulti(items)
	}

	//	selection.DebugDump()

	//	this.Deactivate()
	this.View().Update()
}

func (this *MarqueePart) OnDraw(g paint.Painter) {

	if this.IsActive() && this.IsDraging() {
		rect := this.TrackRect()
		g.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
		//g.SetLineWidth(1)
		g.SetPen1(paint.Color{0, 160, 255, 255}, 0)
		g.Stroke()

	}
}
