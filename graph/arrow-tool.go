package graph

import (
//	"github.com/uk0/silk/gui"
)

func NewArrowTool() ITool {
	p := new(ArrowTool)
	p.Init(p)
	p.AddPart(NewDecorPart())
	p.AddPart(NewSelectPart())
	p.AddPart(NewMovePart())
	p.AddPart(NewMarqueePart())
	return p
}

type ArrowTool struct {
	Tool
}

func (this *ArrowTool) Init(tool ITool) {
	this.Tool.Init(tool)
	this.SetName("arrow-tool")
	this.SetIcon1("arrow-tool")
	this.SetText("箭头工具")
}
