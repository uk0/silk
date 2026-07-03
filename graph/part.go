package graph

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
)

type IPart interface {
}

type Part struct {
	self IPart
	tool *Tool

	//	alwaysOnMove
}

func (this *Part) Init(self IPart) {
	this.self = self
}

func (this *Part) setOwnerTool(t *Tool) {
	if this.tool != nil && this.tool != t {
		panic("Part already binded to anthor tool")
	}
	this.tool = t
}

func (this *Part) View() IView {
	return this.tool.view
}

func (this *Part) Scene() IScene {
	return this.View().Scene()
}

func (this *Part) Tool() ITool {
	return this.tool.Self()
}

func (this *Part) Self() IPart {
	return this.self
}

func (this *Part) IsActive() bool {
	return this.tool.activePart == this.self
}

func (this *Part) Activate() bool {
	tool := this.tool
	self := this.self
	if tool.activePart != nil && tool.activePart != self {
		core.Warn("Try to activate tool part ", core.ObjInfo(self), ", but a ",
			core.ObjInfo(tool.activePart), " already activated.")
		return false
	}
	tool.activePart = self
	core.Debug("tool part activated: ", core.ObjInfo(self))
	return true
}

/*func (this *Part) deactivate() {
	tool := this.tool
	self := this.self
	if tool.activePart != self {
		//core.Warn("tool.activePart != self")
		return
	}
	tool.activePart = nil
	core.Debug("tool part deactivated: ", core.ObjInfo(self))
}
*/

func (this *Part) IsDraging() bool {
	return this.tool.dragStart
}

func (this *Part) TrackRect() geom.Rect {
	tool := this.tool
	x0, y0, x1, y1 := tool.downX, tool.downY, tool.curX, tool.curY
	//	core.Debug("x0, y0, x1, y1 = ", x0, y0, x1, y1)

	return geom.Rect{x0, y0, x1 - x0, y1 - y0}

}

func (this *Part) DownPos() (x, y float64) {
	return this.tool.DownPos()
}

func (this *Part) CurrentPos() (x, y float64) {
	return this.tool.CurrentPos()
}
