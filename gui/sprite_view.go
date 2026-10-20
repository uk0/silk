package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// SpriteView is a tag-driven, read-only HMI symbol. It paints into its parent's
// canvas (no native child window/timer). Icon names can be satin:tank, satin:valve,
// or any existing resource icon. State changes invalidate only when changed.
type SpriteView struct {
	Widget
	iconName, activeIconName, tagName string
	active                            bool
	normal, selected                  paint.Icon
}

func init()                      { core.RegisterFactory("gui.SpriteView", core.TypeOf((*SpriteView)(nil))) }
func NewSpriteView() *SpriteView { w := new(SpriteView); w.Init(w); return w }
func (w *SpriteView) Init(self IWidget) {
	w.Widget.Init(self)
	w.iconName = "satin:valve"
	w.normal = paint.LoadIcon(w.iconName)
}
func (w *SpriteView) IconName() string { return w.iconName }
func (w *SpriteView) SetIconName(s string) {
	if s != w.iconName {
		w.iconName = s
		w.normal = paint.LoadIcon(s)
		w.Self().Update()
	}
}
func (w *SpriteView) ActiveIconName() string { return w.activeIconName }
func (w *SpriteView) SetActiveIconName(s string) {
	if s != w.activeIconName {
		w.activeIconName = s
		w.selected = nil
		if s != "" {
			w.selected = paint.LoadIcon(s)
		}
		w.Self().Update()
	}
}
func (w *SpriteView) Active() bool { return w.active }
func (w *SpriteView) SetActive(v bool) {
	if v != w.active {
		w.active = v
		w.Self().Update()
	}
}
func (w *SpriteView) TagName() string     { return w.tagName }
func (w *SpriteView) SetTagName(s string) { w.tagName = s }
func (w *SpriteView) EnumProperties(p core.IPropertyList) {
	p.AddProperty("图标", w.IconName, w.SetIconName)
	p.AddProperty("激活图标", w.ActiveIconName, w.SetActiveIconName)
	p.AddProperty("激活", w.Active, w.SetActive)
	p.AddProperty("tag", w.TagName, w.SetTagName)
}
func (w *SpriteView) SizeHints() SizeHints {
	return SizeHints{Width: 96, Height: 96, Policy: GrowHorizontal | GrowVertical}
}
func (w *SpriteView) Draw(g paint.Painter) {
	width, height := w.Size()
	size := math.Min(width, height)
	if size <= 0 {
		return
	}
	icon := w.normal
	if w.active && w.selected != nil {
		icon = w.selected
	}
	g.DrawIcon1(icon, (width-size)/2, (height-size)/2, size, !w.active || !w.IsEnabled())
}
