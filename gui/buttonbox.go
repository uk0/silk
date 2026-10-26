package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
	"strings"
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
	spec     string
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
		return i18n.T(name)
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

func (this *ButtonBox) ButtonsSpec() string {
	return this.spec
}

// SetButtonsSpec is SetButtons as one editable string: the button names
// separated by commas ("@ok,@cancel"), which is the form a designer types into
// a property sheet and the form a design file stores. Names are trimmed and
// blanks dropped, and the normalized spelling is what ButtonsSpec reports back,
// so " @ok , " and "@ok" are the same box.
func (this *ButtonBox) SetButtonsSpec(s string) {
	var btns []string
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		btns = append(btns, name)
	}
	this.spec = strings.Join(btns, ",")
	this.SetButtons(btns)
}

func (this *ButtonBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("按钮", this.ButtonsSpec, this.SetButtonsSpec)
}

func (this *ButtonBox) onBtnClick(action IAction, iw interface{}) {
	if this.cbSubmit != nil {
		this.cbSubmit(iw.(IWidget).ExtraData().(string))
	}
}

func (this *ButtonBox) SigSubmit(fn func(string)) {
	this.cbSubmit = fn
}
