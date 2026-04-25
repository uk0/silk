package graph

import (
	//	"silk/core"
	//	"silk/gui"
	"silk/paint"
)

type RectPart struct {
	Part
	cond TraversalCond
}

func NewRectPart() *RectPart {
	p := new(RectPart)
	p.Init(p)
	return p
}

func (this *RectPart) OnLeftDragStart(x, y float64) {
	//view := this.CurrentView()
	//scene := this.Scene()
	//selects := scene.Selection()
	//item := scene.FindItemAt(x, y, this.cond)
	//core.Debug("(this *RectPart) OnDragStart()", x, y)
	this.Activate()
	this.View().Update()
}

func (this *RectPart) OnMouseMove(x, y float64) {
	this.View().Update()
}

func (this *RectPart) OnLeftUp(x, y float64) {
	if !this.IsActive() {
		return
	}
	selection := this.View().Selection()
	rect := this.TrackRect().NormalizeCopy()
	if !rect.IsEmpty() {
		p := NewRectItem()
		p.SetBounds1(rect)
		//p.SetParent(this.Scene())
		cmd := NewAddCommand()
		cmd.AddItem(p, this.Scene())
		this.Scene().UndoStack().Push(cmd)
		selection.Clear()
		selection.Add(p)
	}
	/*
		scene := this.Scene()
		items := scene.FindItemsInRect(rect, nil)
		//core.Debug("rect = ", rect)

		if gui.IsKeyDown(gui.KeyShift) {
			selection.InvertMulti(items)
		} else if gui.IsKeyDown(gui.KeyCtrl) {
			selection.AddMulti(items)
		} else {
			selection.Clear()
			selection.AddMulti(items)
		}

		selection.DebugDump()
	*/

	//	this.Deactivate()
	this.View().Update()
}

func (this *RectPart) OnDraw(g paint.Painter) {

	if this.IsActive() && this.IsDraging() {
		rect := this.TrackRect()
		g.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
		//g.SetLineWidth(1)
		g.SetPen1(paint.Color{0, 160, 255, 255}, 0)
		g.Stroke()

	}
}
