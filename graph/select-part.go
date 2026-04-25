package graph

import (
	"silk/gui"
)

type SelectPart struct {
	Part
	cond TraversalCond
}

func NewSelectPart() *SelectPart {
	p := new(SelectPart)
	p.Init(p)
	return p
}

func (this *SelectPart) OnLeftClick(x, y float64) {
	if this.tool.activePart != nil {
		// 激活了其他部件, 不再执行点选操作
		return
	}
	view := this.View()
	scene := this.Scene()
	selects := view.Selection()
	item := scene.FindItemAt(x, y, this.cond)

	if gui.IsKeyDown(gui.KeyShift) {
		selects.Invert(item)
	} else if gui.IsKeyDown(gui.KeyCtrl) {
		selects.Add(item)
	} else {
		selects.Clear()
		selects.Add(item)
	}
	//selects.DebugDump()
	view.Update()
}
