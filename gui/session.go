package gui

import (
	"errors"
	"fmt"
	"github.com/uk0/silk/core"
)

// 工具视图定义
type ToolViewDef struct {
	// 视图的标识
	// 应和对象工厂名相同, 底层根据此字段调用对象工厂创建对象
	Id string
	// 视图的名称
	Name string
	// 视图的图标
	Icon string
	// 视图的描述
	Desc string
}

var toolViewReg = make(map[string]ToolViewDef)

// 注册工具视图
func RegisterToolView(info ToolViewDef) error {
	if info.Id == "" {
		err := core.StrErr(fmt.Sprint("invalid view info: ", info))
		core.Warn(err)
		return err
	}
	if oi, ok := toolViewReg[info.Id]; ok && oi != info {
		core.Debug("view info already registered, will be overwritten: ", oi)
	}
	toolViewReg[info.Id] = info
	return nil
}

func GetToolViewDef(typ string) (ToolViewDef, bool) {
	a, ok := toolViewReg[typ]
	return a, ok
}

// 加载框架布局
// 如果对应框架已经存在, 则在原有框架上加载
// 如果对应框架不存在, 则创建一个新的框架, 再加载
func LoadFrameSession(doc *core.TDoc) (*Frame, error) {
	var uuid core.Uuid
	if err := doc.ReadAttr("uuid", &uuid); err != nil {
		return nil, err
	}
	if uuid.IsZero() {
		return nil, errors.New("uuid is zero")
	}
	frame := FindFrameByUuid(uuid)
	if frame == nil {
		frame = NewFrameWindow()
		frame.SetUuid(uuid)
	}
	err := frame.LoadSession(doc)
	frame.SetVisible(true)
	return frame, err
}

func SaveSession() (*core.TDoc, error) {
	doc := core.NewTDoc()
	frames := AllFrames()
	for _, frame := range frames {
		if frame.Uuid().IsZero() {
			core.Debug(`frame's uuid is zero, skip. title="` + frame.Title() + `"`)
			continue
		}
		p, err := frame.SaveSession()
		if err != nil {
			core.Warn(err)
			continue
		}
		p.SetValue("frame")
		doc.AddChild(p)
	}
	return doc, nil
}

func LoadSession(doc *core.TDoc) error {
	for _, p := range doc.Childdren() {
		var tpy string
		p.Value(&tpy)
		if tpy == "frame" {
			LoadFrameSession(p)
		}
	}
	return nil
}

func SaveSessionFile(path string) error {
	core.Debug(`save session file: "` + path + `"`)
	doc, err := SaveSession()
	if err != nil {
		return err
	}
	return doc.SaveFile(path)
}

func LoadSessionFile(path string) error {
	core.Debug(`load session file: "` + path + `"`)
	doc, err := core.LoadTDocFile(path)
	if err != nil {
		return err
	}
	return LoadSession(doc)
}

func updateToolViewsMenu(btn IButton) {
	//core.Debug("updateViewsMenu")
	frame := FindOwnerFrame(btn.(IWidget))
	if frame == nil {
		core.Warn("frame not found")
		return
	}
	sub := btn.SubPopup().(*Menu)
	sub.Clear()
	for _, p := range frame.ToolViewActions() {
		sub.AddActionButton(p)
	}
}

func AddToolViewSubMenu(parentMenu *Menu) (*Menu, *Button) {
	menu, btn := parentMenu.AddSubMenu("视图", nil, nil)
	btn.SetSubPopupCallback(updateToolViewsMenu)
	return menu, btn
}
