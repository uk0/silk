package data

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func init() {
	core.RegisterFactory("data.PrjView", gui.TypeOf(PrjView{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "data.PrjView",
		Name: "项目",
		Icon: "project",
		Desc: "显示项目列表",
	})
}

// ////////////////////////////////////////////////////////////
type PrjView struct {
	gui.TreeView
	actNewProject gui.IAction
}

func NewPrjView() *PrjView {
	p := new(PrjView)
	p.Init(p)
	return p
}

func (this *PrjView) Init(self gui.IWidget) {
	this.TreeView.Init(self)
	this.actNewProject = gui.NewAction1("创建建新项目", gui.LoadIcon("document"))
	this.actNewProject.BindFunc0(this.PromptCreateProject)
	this.SetContextMenuCallback(func(w gui.IWidget, x, y float64) {
		w.(*PrjView).onContextMenu(x, y)
	})
	m := new(PrjModel)
	m.Init(m)
	m.Refresh()
	this.SetModel(m)
}

func (this *PrjView) PrjModel() *PrjModel {
	return this.Model().(*PrjModel)
}

func (this *PrjView) onContextMenu(x, y float64) {
	xg, yg := this.MapToGlobal(x, y)
	menu := gui.NewPopupMenu()
	menu.AddActionButton(this.actNewProject)
	menu.ShowAsPopup(xg, yg, true)
}

func (this *PrjView) PromptCreateProject() {
	prjname, ok := gui.ShowInputBox(this, nil, "新建项目", "项目名称:", core.NewUuid().String())
	if !ok {
		return
	}
	err := CreateProject(prjname)
	if err != nil {
	}

	//for i := 0; i < 10; i++ {
	//	CreateProject(core.GenUuid().String())
	//}
}
