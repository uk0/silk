package gui

import (
	"github.com/uk0/silk/core"
)

func init() {
	core.RegisterFactory("gui.ButtonBox", core.TypeOf((*ButtonBox)(nil)))
}

var stdBtnMap = map[string]string{
	"@ok":          "OK",
	"@cancel":      "Cancel",
	"@save":        "Save",
	"@discard":     "Discard",
	"@yes":         "Yes",
	"@no":          "No",
	"@apply":       "Apply",
	"@save-all":    "Save All",
	"@discard-all": "Discard All",
	"@close-all":   "Close All",
}

// 按钮框, 即对话框里的"确定/取消"等标准按钮, 也可加入自定义按钮
type ButtonBox struct {
	Menu
	cbSubmit func(string)
	btnMap   map[string]*Button
}

func NewButtonBox() *ButtonBox {
	p := new(ButtonBox)
	p.Init(p)
	p.btnMap = make(map[string]*Button)
	return p
}

func (this *ButtonBox) Init(iw IWidget) {
	this.Menu.Init(iw)
}

func btnDisplayName(s string) string {
	name, ok := stdBtnMap[s]
	if ok {
		return name
	}
	return s
}

func (this *ButtonBox) SetButtons(btns []string) {
	for _, btn := range this.btnMap {
		this.RemoveWidget(btn)
	}
	this.btnMap = make(map[string]*Button)
	for _, name := range btns {
		if _, ok := this.btnMap[name]; ok {
			continue
		}
		p := this.AddButton1(btnDisplayName(name), nil)
		p.SetExtraData(name)
		p.Action().BindFunc(this.onBtnClick)
		this.btnMap[name] = p
	}
}

func (this *ButtonBox) onBtnClick(action IAction, iw interface{}) {
	if this.cbSubmit != nil {
		this.cbSubmit(iw.(IWidget).ExtraData().(string))
	}
}

func (this *ButtonBox) SigSubmit(fn func(string)) {
	this.cbSubmit = fn
}
