package graph

import (
//	"silk/gui"
)

func NewRectTool() ITool {
	p := new(RectTool)
	p.Init(p)
	p.AddPart(NewDecorPart())
	p.AddPart(NewSelectPart())
	p.AddPart(NewRectPart())
	return p
}

type RectTool struct {
	Tool
}

func (this *RectTool) Init(tool ITool) {
	this.Tool.Init(tool)
	this.SetName("rect-tool")
	this.SetIcon1("rect-tool")
	this.SetText("矩形工具")
}
