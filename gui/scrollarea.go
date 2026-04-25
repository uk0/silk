package gui

import (
	//	"silk/diag"
	"silk/core"
)

func init() {
	core.RegisterFactory("gui.ScrollArea", core.TypeOf((*ScrollArea)(nil)))
}

func NewScrollArea() *ScrollArea {
	p := new(ScrollArea)
	p.Init(p)
	return p
}

// 滚动区域
// 注: 滚动区域提供滚动条和相应接口, 不自动滚动客户区坐标
//    程序代码中, 要根据滚动位置, 另行偏移客户区坐标和平移绘图对象
//    由于偏移操作可定制, 所以滚动单位可以不是像素
type ScrollArea struct {
	Widget
	vs, hs        *ScrollBar
	sx, sy        float64
	layoutReEnter int
}

func (this *ScrollArea) SetScrollX(sx float64) {
	if this.hs != nil {
		this.hs.SetValue(sx)
	} else {
		this.sx = sx
		this.Self().Update()
	}
}

func (this *ScrollArea) SetScrollY(sy float64) {
	if this.vs != nil {
		this.vs.SetValue(sy)
	} else {
		this.sy = sy
		this.Self().Update()
	}
}

func (this *ScrollArea) ScrollX() float64 {
	if this.hs != nil {
		this.sx = this.hs.Value()
		//		core.Debug(this.sx)
	}
	return this.sx
}

func (this *ScrollArea) ScrollY() float64 {
	if this.vs != nil {
		this.sy = this.vs.Value()
		//		core.Debug(this.sy)
	}
	return this.sy
}

func (this *ScrollArea) ScrollPos() (x, y float64) {
	return this.ScrollX(), this.ScrollY()
}

func (this *ScrollArea) VertScrollBar() *ScrollBar {
	iw := this.Self()
	if this.vs == nil {
		this.vs = core.New("gui.ScrollBar").(*ScrollBar)
		this.vs.SetParent(iw)
		this.vs.SetVertical(true)
		this.vs.SetChangedCallback(this.onVertScroll)
	}
	return this.vs
}

func (this *ScrollArea) HorzScrollBar() *ScrollBar {
	iw := this.Self()
	if this.hs == nil {
		this.hs = core.New("gui.ScrollBar").(*ScrollBar)
		this.hs.SetParent(iw)
		this.hs.SetChangedCallback(this.onHorzScroll)
	}
	return this.hs
}

func (this *ScrollArea) onHorzScroll(sender IWidget) {
	if i, ok := this.Self().(IOnHorzScroll); ok {
		i.OnHorzScroll(sender)
		return
	}
	this.Self().Update()
}

func (this *ScrollArea) onVertScroll(sender IWidget) {
	if i, ok := this.Self().(IOnVertScroll); ok {
		i.OnVertScroll(sender)
		return
	}
	this.Self().Update()
}

func (this *ScrollArea) OnHorzScroll(sender IWidget) {
	this.Self().Update()
}

func (this *ScrollArea) OnVertScroll(sender IWidget) {
	this.Self().Update()
}

func (this *ScrollArea) ViewportSizePx() (width, height float64) {
	width = this.w
	height = this.h
	sw := Theme().ScrollWidth
	if this.vs != nil && this.vs.IsVisible() {
		width -= sw
	}
	if this.hs != nil && this.hs.IsVisible() {
		height -= sw
	}
	return
}

func (this *ScrollArea) EnumProperties(list core.IPropertyList) {
	list.AddProperty("可见", this.IsVisible, this.SetVisible)
}

func (this *ScrollArea) Layout() {
	sw := Theme().ScrollWidth
	width, height := this.Self().Size()

	hVisible := this.hs != nil && this.hs.IsVisible()
	vVisible := this.vs != nil && this.vs.IsVisible()

	if this.hs != nil {
		if vVisible {
			this.hs.SetBounds(0, height-sw, width-sw, sw)
		} else {
			this.hs.SetBounds(0, height-sw, width, sw)
		}
	}

	if this.vs != nil {
		if hVisible {
			this.vs.SetBounds(width-sw, 0, sw, height-sw)
		} else {
			this.vs.SetBounds(width-sw, 0, sw, height)
		}
	}

	this.Self().Update()
}
