package gui

import (
	"silk/core"
	//	"silk/factory"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.TabBar", core.TypeOf((*TabBar)(nil))) //((*TabBar)(nil)))
}

func NewTabBar() *TabBar {
	p := new(TabBar)
	p.Init(p)
	//	p.Init()
	return p
}

//type ITabBar interface {
//}

// 标签页
// (注: 目前只在Dock中使用, 未准备好用在别处)
type TabBar struct {
	Widget
	tabs          []*_Tab
	activeTab     int
	hoverTab      int
	closeBtn      bool
	hoverCloseBtn bool
	cbActivate    func(tb *TabBar, idx int)
	cbDeactivate  func(tb *TabBar, idx int)
	cbClose       func(tb *TabBar, idx int) bool
	downTab       int
	downX         float64
	downY         float64
	downCloseBtn  bool
	nextActiveId  int
	dropPos       int
	cbDragStart   func(tb *TabBar, idx int) interface{}
	cbDragMove    func(tb *TabBar, dnd IDndContext)
	cbDrop        func(tb *TabBar, idx int, dnd IDndContext)
}

type _Tab struct {
	// 实际的数据
	data interface{}

	// 宽度
	width float64

	// 当前标签页激活的序号, 用来在关闭标签页后激活前一个标签页
	activeId int

	// 当前显示的图标
	icon paint.Icon

	// 当前应显示的完整文本
	text string

	// 当前实际显示的文本, 可能完整也可能是缩略
	label string
}

// 同步数据, 有变化返回true, 否则返回false
// 此函数由TabBar在空闲时调用, 以确定是否要刷新标签
func (this *_Tab) sync() (changed bool) {
	t := this.realText()
	if t != this.text {
		this.text = t
		changed = true
		this.label = EllipsisText(t, 16)
	}
	i := this.realIcon()
	if i != this.icon {
		this.icon = i
		changed = true
	}
	return
}

func (this *_Tab) Text() string {
	this.sync()
	return this.label
}

func (this *_Tab) realText() (s string) {
	if iTitle, ok := this.data.(ITitle); ok {
		s = iTitle.Title()
	} else if iText, ok := this.data.(IText); ok {
		s = iText.Text()
	} else if iString, ok := this.data.(IString); ok {
		s = iString.String()
	}
	//s = EllipsisText(s, 16)
	return
}

func (this *_Tab) Icon() paint.Icon {
	this.sync()
	return this.icon
}

func (this *_Tab) realIcon() paint.Icon {
	if i, ok := this.data.(IIcon); ok {
		return i.Icon()
	}
	return nil
}

func (this *_Tab) Data() interface{} {
	return this.data
}

func (this *_Tab) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	icon := this.icon
	text := this.label
	ext := t.Font.TextExtents(text)
	if icon != nil {
		m := t.TabMargin
		h := math.Max(t.IconSize, fe.Height)
		w := ext.Width + h
		w += m.L*2 + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	} else {
		m := t.TabMargin
		h := fe.Height
		w := ext.Width
		w += m.L + m.R
		h += m.T + m.B
		if w < h {
			w = h * 2
		}
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	}
}

func (this *TabBar) Init(self IWidget) {
	this.Widget.Init(self)
	//this.AddTab(IconText{LoadIcon("globe"), "Test1"}, false)
	//this.AddTab(IconText{LoadIcon("clipboard"), "Test2"}, false)
	//this.AddTab(IconText{nil, "Test3"}, false)
	//this.AddTab(IconText{nil, "Test4"}, false)
	this.closeBtn = true
	this.hoverTab = -1
	this.downTab = -1
	this.dropPos = -1
}

func (this *TabBar) drawTab(tb *_Tab, g paint.Painter, w, h float64, active, hover bool) {
	t := Theme()
	t.DrawTab(g, tb.icon, tb.label, w, h,
		active, hover, active && this.closeBtn && (hover || this.downCloseBtn),
		active && this.hoverCloseBtn, this.downCloseBtn)
}

func (this *TabBar) Draw(g paint.Painter) {
	t := Theme()

	g.Save()
	g.Rectangle(0, this.h-t.TabMargin.B, this.w, t.TabMargin.B)
	g.SetBrush1(t.ViewBGColor)
	//g.SetLineWidth(0)
	g.Fill()

	g.MoveTo(0, this.h-t.TabMargin.B-1)
	g.LineTo(this.w, this.h-t.TabMargin.B-1)
	g.SetPen1(t.BorderColor, 1)
	//g.SetLineWidth(1)
	g.Stroke()

	x, y := 0.0, 0.0
	h := this.h
	g.Translate(x, y)
	for i, tab := range this.tabs {
		this.drawTab(tab, g, tab.width, h, this.activeTab == i, this.hoverTab == i)
		if this.dropPos == i {
			g.Rectangle(-2, t.TabMargin.T, 4, h-t.TabMargin.B-t.TabMargin.T)
			g.SetBrush1(t.HighLightColor)
			g.Fill()
		}
		g.Translate(tab.width, 0)
	}
	if this.dropPos == len(this.tabs) {
		g.Rectangle(-2, t.TabMargin.T, 4, h-t.TabMargin.B-t.TabMargin.T)
		g.SetBrush1(t.HighLightColor)
		g.Fill()
	}
	g.Restore()
}

func (this *TabBar) IsEmpty() bool {
	return len(this.tabs) == 0
}

func (this *TabBar) Count() int {
	return len(this.tabs)
}

//func (this *TabBar) OnMouseEnter() {
//	core.Debug("(this *TabBar) OnMouseEnter()")
//	this.Ow().Update()
//}

func (this *TabBar) OnMouseLeave() {
	//core.Debug("(this *TabBar) OnMouseLeave()")
	if this.hoverTab != -1 {
		this.hoverTab = -1
		this.Self().Update()
	}
}

func (this *TabBar) OnLeftDown(x, y float64) {
	this.downX = x
	this.downY = y
	this.downTab, this.downCloseBtn = this.HitTest(x, y)
	this.SetActiveTab(this.downTab)
	this.Self().Update()
}

func (this *TabBar) OnLeftUp(x, y float64) {
	if this.downCloseBtn {
		_, downBtn := this.HitTest(x, y)
		if downBtn {
			this.CloseTab(this.activeTab)
		}
		this.downCloseBtn = false
		this.Self().Update()
	}
	this.downTab = -1
}

func (this *TabBar) OnMouseMove(x, y float64) {
	//core.Debug("(this *TabBar) OnMouseMove()")
	if this.downTab != -1 && (math.Abs(x-this.downX) > 4 || math.Abs(y-this.downY) > 4) {
		this.PopCapture()
		//	core.Debug(this.downTab)
		tb := this.tabs[this.downTab]
		tb.sync()
		t := Theme()
		pixmap := paint.IconTextToPixmap(tb.icon, t.IconSize, false,
			tb.label, t.Font, t.TextColor, false)
		var data interface{}
		if this.cbDragStart != nil {
			data = this.cbDragStart(this, this.downTab)
		} else {
			data = tb
		}
		this.DoDragDrop(pixmap, DndMove, data)
		this.downTab = -1
		return
	}

	if this.downTab == -1 {
		hover, hoverClose := this.HitTest(x, y)
		if hover != this.hoverTab || this.hoverCloseBtn != hoverClose {
			this.hoverTab = hover
			this.hoverCloseBtn = hoverClose
			this.Self().Update()
		}
	}
}

func (this *TabBar) HitTest(x, y float64) (index int, hoverCloseBtn bool) {
	if x < 0 || y < 0 || x >= this.w || y >= this.h {
		index = -1
		return
	}
	t := Theme()
	m := t.TabMargin
	x0 := 0.0
	//h := this.h - t.TabBarMargin.T - t.TabBarMargin.B
	for i, tab := range this.tabs {
		if x >= x0 && x < x0+tab.width {
			index = i
			if index == this.activeTab {
				xc := x0 + tab.width - t.TabCloseSize - m.R
				yc := m.T + (this.h-m.T-m.B-t.TabCloseSize)*0.5
				hoverCloseBtn = x >= xc && x < xc+t.TabCloseSize &&
					y >= yc && y < yc+t.TabCloseSize
			}
			return
		}
		x0 += tab.width
	}
	return -1, false
}

func (this *TabBar) DropIndex(x, y float64) (index int) {
	//t := Theme()
	//m := t.TabMargin
	x0 := 0.0
	x1 := 0.0
	//h := this.h - t.TabBarMargin.T - t.TabBarMargin.B
	for i, tab := range this.tabs {
		if x >= x1 && x < x0+tab.width*0.5 {
			index = i
			return
		}
		x1 = x0 + tab.width*0.5
		x0 += tab.width
	}
	return len(this.tabs)
}

func (this *TabBar) SetActivateCallback(callback func(tb *TabBar, idx int)) {
	this.cbActivate = callback
}

func (this *TabBar) SetDeactivateCallback(callback func(tb *TabBar, idx int)) {
	this.cbDeactivate = callback
}

func (this *TabBar) SetCloseCallback(callback func(tb *TabBar, idx int) bool) {
	this.cbClose = callback
}

func (this *TabBar) SetActiveTab(idx int) {
	old := this.activeTab
	if old == idx {
		return
	}
	this.activeTab = idx
	if old != -1 && this.cbDeactivate != nil {
		this.cbDeactivate(this, old)
	}
	if idx != -1 {
		this.nextActiveId++
		this.tabs[idx].activeId = this.nextActiveId
		if this.cbActivate != nil {
			this.cbActivate(this, idx)

		}
	}
	this.Self().Update()
}

func (this *TabBar) activateLasTab() {
	idx := -1
	id := -1
	for i, t := range this.tabs {
		if t.activeId > id {
			id = t.activeId
			idx = i
		}
	}
	if idx != -1 {
		this.SetActiveTab(idx)
	}
}

func (this *TabBar) ActiveTab() int {
	return this.activeTab
}

func (this *TabBar) AddTab(data interface{}, activate bool) {
	t := new(_Tab)
	t.data = data
	this.tabs = append(this.tabs, t)
	t.sync()
	this.Layout()
	if activate {
		this.SetActiveTab(len(this.tabs) - 1)
	}
}

func (this *TabBar) Data(idx int) interface{} {
	if idx < 0 || idx >= len(this.tabs) {
		return nil
	}
	return this.tabs[idx].data
}

func (this *TabBar) InsertTab(idx int, data interface{}, activate bool) {
	if idx < 0 || idx >= len(this.tabs) {
		this.AddTab(data, activate)
		return
	}
	t := new(_Tab)
	t.data = data
	t.sync()
	this.tabs = append(this.tabs, nil)
	copy(this.tabs[idx+1:], this.tabs[idx:])
	this.tabs[idx] = t

	if this.activeTab >= idx {
		this.activeTab++
	}
	this.hoverTab = -1
	this.Layout()
	if activate {
		this.SetActiveTab(idx)
	}
}

func (this *TabBar) RemoveTab(idx int) interface{} {
	if idx < 0 || idx >= len(this.tabs) {
		return nil
	}
	if this.activeTab == idx {
		this.activeTab = -1
		if this.cbDeactivate != nil {
			this.cbDeactivate(this, idx)
		}
	}
	ret := this.tabs[idx].data
	for i := idx; i < len(this.tabs)-1; i++ {
		this.tabs[i] = this.tabs[i+1]
	}
	this.tabs[len(this.tabs)-1] = nil
	this.tabs = this.tabs[:len(this.tabs)-1]
	this.activateLasTab()
	this.Self().Update()
	return ret
}

func (this *TabBar) CloseTab(idx int) bool {
	if idx < 0 || idx >= len(this.tabs) {
		return false
	}

	if this.cbClose == nil {
		a := this.Data(idx)
		return PromptSaveClose(this, a)
	}

	return this.cbClose(this, idx)
}

func (this *TabBar) SetDndCallback(cbDragStart func(tb *TabBar, idx int) interface{},
	cbDragMove func(tb *TabBar, dnd IDndContext),
	cbDrop func(tb *TabBar, idx int, dnd IDndContext)) {
	this.cbDragStart = cbDragStart
	this.cbDragMove = cbDragMove
	this.cbDrop = cbDrop
}

func (this *TabBar) OnDragEnter(x, y float64, dnd IDndContext) {
	//core.Debug(dnd.From(), this)
	if dnd.From() == this {
		dnd.SetAction(DndMove)
		return
	}
	core.Debug("(this *TabBar) OnDragEnter")
	if dnd.HasFormat("[]interface{}") {
		//core.Debug("(this *Edit) OnDragEnter: has []interface{}")
		//dnd.SetAction(DndMove)
		//data := dnd.Data("[]interface{}").([]interface{})

	}
}

func (this *TabBar) OnDragLeave() {
	if this.dropPos != -1 {
		this.dropPos = -1
		this.Self().Update()
	}
}

func (this *TabBar) OnDragMove(x, y float64, dnd IDndContext) {
	//if dnd.HasFormat("text/plain") {
	//	dnd.SetAction(DndMove)
	//}
	var move bool
	if dnd.From() == this {
		move = true
	} else if this.cbDragMove != nil {
		this.cbDragMove(this, dnd)
		move = dnd.Action() == DndMove
	}
	if move {
		this.dropPos = this.DropIndex(x, y)
		//if this.dropPos == this.downTab || this.dropPos == this.downTab+1 {
		//	this.dropPos = -1
		//	dnd.SetAction(DndIgnore)
		//} else {
		dnd.SetAction(DndMove)
		//}
		this.Self().Update()
	}
}

func (this *TabBar) OnDrop(x, y float64, dnd IDndContext) {
	if dnd.From() == this {
		this.dropPos = this.DropIndex(x, y)
		//if this.dropPos == this.downTab || this.dropPos == this.downTab+1 {
		//	this.dropPos = -1
		//	dnd.SetAction(DndIgnore)
		//	core.Debug("aaaaa")
		//} else {
		if this.dropPos > this.downTab {
			this.dropPos--
		}
		data := this.RemoveTab(this.downTab)
		this.InsertTab(this.dropPos, data, true)
		dnd.SetAction(DndMove)
		//}
	}

	this.dropPos = -1
	this.Self().Update()
}

//func (this *TabBar) OnMouseStop(x, y float64) {
//	core.Debug("TabBar.OnMouseStop():", x, y)
//}

func (this *TabBar) SizeHints() SizeHints {
	t := Theme()
	m := t.TabMargin
	var w, h float64

	for _, c := range this.tabs {
		hints := c.SizeHints()
		w += hints.Width
		h = math.Max(h, hints.Height)
	}

	w += m.L + m.R
	h += m.T + m.B
	if h < t.TabBarHeight {
		h = t.TabBarHeight
	}
	return SizeHints{Width: w, Height: h, Policy: 0}
}

func (this *TabBar) Layout() {
	count := this.Count()
	if count == 0 {
		return
	}
	iw := this.Self()
	//	t := Theme()
	//	margin := float64(0)
	spacing := float64(0)
	w, h := iw.Size()
	//var margin *Margin
	for _, v := range this.tabs {
		v.sync()
	}
	//margin = &t.TabMargin

	//x, _, w, h := margin.Apply(0, 0, w, h)
	x := 0.0
	n := count
	hints := make([]SizeHints, n, n)
	var total float64
	var expandBase, shrinkBase float64
	//core.Debug(w, h)
	for i, c := range this.tabs {
		hi := c.SizeHints()
		hints[i] = hi
		total += hi.Width

		if (hi.Policy & ExpandHorizontal) != 0 {
			expandBase += hi.Width
		}
		if (hi.Policy & ShrinkHorizontal) != 0 {
			shrinkBase += hi.Width
		}

		if (hi.Policy & (ExpandVertical | GrowVertical)) != 0 {
			hints[i].Height = h
		}
	}
	extra := w - total - float64(sepCount(n))*spacing
	if extra > 0 && expandBase > 0 {
		dw := extra / expandBase
		x1 := x
		for i, c := range this.tabs {
			hi := hints[i]
			var w1 float64
			if (hi.Policy & ExpandHorizontal) != 0 {
				w1 = hi.Width + hi.Width*dw
			} else {
				w1 = hi.Width
			}
			c.width = w1
			x1 += w1 + spacing
		}
	} else if extra < 0 && shrinkBase > 0 {
		dw := extra / shrinkBase
		x1 := x
		for i, c := range this.tabs {
			hi := hints[i]
			var w1 float64
			if (hi.Policy & ShrinkHorizontal) != 0 {
				w1 = hi.Width + hi.Width*dw
			} else {
				w1 = hi.Width
			}
			c.width = w1
			x1 += w1 + spacing
		}
	} else {
		x1 := x
		for i, c := range this.tabs {
			hi := hints[i]
			var w1 float64 = hi.Width
			c.width = w1
			x1 += w1 + spacing
		}
	}
	iw.Update()
}

func (this *TabBar) OnIdle() {
	var needLayout bool
	for _, tb := range this.tabs {
		if tb.sync() {
			needLayout = true
		}
	}

	if needLayout {
		dock, ok := this.Parent().(IDock)
		if ok {
			dock.Layout()
		} else {
			this.Layout()
		}
	}
}
