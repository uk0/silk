package gui

import (
	"silk/core"
	"silk/geom"
	"silk/paint"
	//	"fmt"
	"math"
)

func init() {
	core.RegisterFactory("gui.ListWidget", core.TypeOf((*ListWidget)(nil))) //((*ListWidget)(nil)))
}

func NewListWidget() *ListWidget {
	p := new(ListWidget)
	p.Init(p)
	return p
}

// 列表控件里的一项
type ListItem struct {
	Text    string
	Icon    paint.Icon
	Checked bool
	Data    interface{}
}

// 列表控件(不使用model-view架构)
type ListWidget struct {
	ScrollArea
	titleProp
	iconProp

	items []ListItem

	iconVisible  bool
	checkVisible bool
	rowHeight    float64
	font         paint.Font
	iconSize     float64
	padding      Padding

	activeIndex int
	hoverIndex  int

	sigSelectionChanged func(o interface{}, idx []int)
	sigCheckChanged     func(o interface{}, idx int)
	sigSubmit           func(o interface{})

	downX      float64
	downY      float64
	downRow    int
	downCol    int
	isLeftDown bool

	showHover  bool
	showSelect bool

	sigDragStart func(idx []int) ([]interface{}, DndAction)
}

func (this *ListWidget) Init(iw IWidget) {
	this.ScrollArea.Init(iw)
	this.padding = Theme().EditPadding
	this.activeIndex = -1
	this.showSelect = true
}

func (this *ListWidget) IsSelectionVisible() bool {
	return this.showSelect
}

func (this *ListWidget) SetSelectionVisible(b bool) {
	this.showSelect = b
	this.Update()
}

func (this *ListWidget) IsHoverVisible() bool {
	return this.showHover
}

func (this *ListWidget) SetHoverVisible(b bool) {
	this.showHover = b
	this.Update()
}

func (this *ListWidget) IsIconVisible() bool {
	return this.iconVisible
}

func (this *ListWidget) SetIconVisible(b bool) {
	this.iconVisible = b
	this.Layout()
}

func (this *ListWidget) IsCheckBoxVisible() bool {
	return this.checkVisible
}

func (this *ListWidget) SetCheckBoxVisible(b bool) {
	this.checkVisible = b
	this.Layout()
}

func (this *ListWidget) RowHeight() float64 {
	if this.rowHeight <= 0 {
		return Theme().ItemHeight
	}
	return this.rowHeight
}

func (this *ListWidget) SetRowHeight(rh float64) {
	this.rowHeight = rh
	this.Layout()
}

func (this *ListWidget) Font() paint.Font {
	if this.font == nil {
		return Theme().Font
	}
	return this.font
}

func (this *ListWidget) SetFont(font paint.Font) {
	this.font = font
	this.Layout()
}

func (this *ListWidget) IconSize() float64 {
	if this.iconSize <= 0 {
		return Theme().IconSize
	}
	return this.iconSize
}

func (this *ListWidget) SetIconSize(sz float64) {
	this.iconSize = sz
	this.Layout()
}

func (this *ListWidget) SetPadding(left, right, top, bottom float64) {
	this.padding.L = math.Ceil(left)
	this.padding.R = math.Ceil(right)
	this.padding.T = math.Ceil(top)
	this.padding.B = math.Ceil(bottom)
	this.Layout()
}

func (this *ListWidget) Padding() (left, right, top, bottom float64) {
	left, right, top, bottom = this.padding.L, this.padding.R, this.padding.T, this.padding.B
	return
}

//func (this *ListWidget) ClientRect() (x, y, width, height float64) {
//	x, y, width, height = this.padding.Apply(0, 0, this.wi
//	}

func (this *ListWidget) Draw(g paint.Painter) {
	g.Save()
	defer func() {
		Theme().DrawViewFrame(g, 0, 0, this.w, this.h)
		g.Restore()
	}()

	g.Rectangle(0, 0, this.w, this.h)
	g.SetBrush1(Theme().ViewBGColor)
	g.Fill()

	//	t := Theme()
	m := this.padding
	g.Translate(m.L, m.T)
	//	core.Debug("m=", m)

	rh := this.RowHeight()
	font := this.Font()
	fe := font.FontExtents()
	sx, sy := this.ScrollPos()
	//	core.Debug("sx  sy=", sx, sy)
	if sx > 0 || sy > 0 {
		g.Translate(-sx, -sy*rh)
	}

	xIcon := 0.0
	xCheck := 0.0

	if this.iconVisible {
		xCheck = xIcon + this.IconSize() + 3
	}
	xText := xCheck

	if this.checkVisible {
		xText = xCheck + Theme().CheckBoxSize + 3
	}

	r0 := int(sy)

	_, _, wc, hc := m.Apply(0, 0, this.w, this.h)

	//	core.Debug("hc=", hc)
	//	core.Debug("rh=", rh)
	n := int(hc / rh)

	//	core.Debug("n=", n)

	r1 := r0 + n + 1 // +1 for partially visible bottom row
	if r1 >= this.Count() {
		r1 = this.Count() - 1
	}
	//	core.Debug("r0  r1=", r0, r1)

	g.SetFont(font)

	//var hlIndex int
	//var sensorIndex int
	//if this.IsAlterHoverStyle() {
	//	hlIndex = this.hoverIndex
	//	sensorIndex = this.activeIndex
	//} else {
	//	hlIndex = this.activeIndex
	//	sensorIndex = this.hoverIndex
	//}

	y := float64(r0) * rh
	for r := r0; r <= r1; r++ {

		if this.showSelect && this.activeIndex == r ||
			!this.showSelect && this.showHover && this.hoverIndex == r {
			g.SetBrush1(Theme().HighLightColor)
			g.Rectangle1(geom.Rect{0, y, wc, rh})
			g.Fill()
		}

		if this.iconVisible {
			icon := this.items[r].Icon
			if icon != nil {
				pd := 0.5 * (rh - this.IconSize())
				g.DrawIcon1(icon, xIcon, y+pd, this.IconSize(), false)
			}
		}

		if this.checkVisible {
			sz := Theme().CheckBoxSize
			pd := 0.5 * (rh - sz)
			checked := this.items[r].Checked
			var icon paint.Icon
			if checked {
				icon = Theme().CheckedIcon
			} else {
				icon = Theme().UncheckedIcon
			}
			g.DrawIcon1(icon, xCheck, y+pd, sz, false)
		}

		yt := y + fe.Ascent + (rh-fe.Height)*0.5
		xt := xText

		if this.showHover && this.showSelect && this.hoverIndex == r {
			g.SetPen1(Theme().HighLightColor, 2)
			g.Line(xt, yt, 20, yt)
			g.Stroke()
		}
		//core.Debug("xt yt = ", xt, yt)
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(xt, yt, this.items[r].Text)

		y += rh
	}

	if sx > 0 || sy > 0 {
		g.Translate(sx, sy*rh)
	}

	g.Translate(-m.L, -m.T)

}

func (this *ListWidget) Append(a ListItem) {
	this.items = append(this.items, a)
	this.Layout()
}

func (this *ListWidget) Insert(idx int, a ListItem) {
	v := append(append(this.items[:idx], a), this.items[idx:]...)
	this.items = v
	this.Layout()
}

func (this *ListWidget) Remove(idx int) ListItem {
	ret := this.items[idx]
	this.items[idx] = ListItem{}
	v := append(this.items[:idx], this.items[idx+1:]...)
	this.items = v
	this.Layout()
	return ret
}

func (this *ListWidget) RemoveLast() ListItem {
	idx := len(this.items) - 1
	ret := this.items[idx]
	this.items[idx] = ListItem{}
	this.items = this.items[:idx-1]
	this.Layout()
	return ret
}

func (this *ListWidget) Clear() {
	this.items = nil
	this.Layout()
}

func (this *ListWidget) ItemList() (ret []ListItem) {
	copy(ret, this.items)
	return
}

func (this *ListWidget) Item(idx int) ListItem {
	return this.items[idx]
}

func (this *ListWidget) SetItem(idx int, item ListItem) {
	this.items[idx] = item
	this.Layout()
}

func (this *ListWidget) Count() int {
	return len(this.items)
}

func (this *ListWidget) HitTest(x, y float64) (row, col int) {
	sx, sy := this.ScrollPos()
	rh := this.RowHeight()
	row = int((y-this.padding.T)/rh + sy)
	if row < 0 || row >= this.Count() {
		row = -1
	}

	xIcon := 0.0
	xCheck := 0.0

	if this.iconVisible {
		xCheck = xIcon + this.IconSize() + 3
	}
	xText := xCheck

	if this.checkVisible {
		xText = xCheck + Theme().CheckBoxSize + 3
	}

	x += sx

	if x >= xText {
		col = 2
	} else if this.checkVisible && x >= xCheck {
		col = 1
	} else if this.iconVisible {
		col = 0
	} else {
		col = -1
	}

	return
}

func (this *ListWidget) emitSelectionChanged(oldIdx int) {
	if this.sigSelectionChanged != nil {
		//this.sigSelectionChanged(this.Self(), oldIdx)
	}
}

func (this *ListWidget) SigSelectionChanged(fn func(o interface{}, idx []int)) {
	this.sigSelectionChanged = fn
}

func (this *ListWidget) emitCheckChanged(row int) {
	if this.sigCheckChanged != nil {
		this.sigCheckChanged(this.Self(), row)
	}
	this.Submit()
}

func (this *ListWidget) SigCheckChanged(fn func(o interface{}, idx int)) {
	this.sigCheckChanged = fn
}

func (this *ListWidget) Submit() {
	if this.sigSubmit != nil {
		this.sigSubmit(this.Self())
	}
}

func (this *ListWidget) SigSubmit(fn func(o interface{})) {
	this.sigSubmit = fn
}

func (this *ListWidget) OnLeftDown(x, y float64) {
	this.isLeftDown = true
	this.downX, this.downY = x, y
	row, col := this.HitTest(x, y)
	this.downRow, this.downCol = row, col
	switch col {
	case 1:
	case 0:
		fallthrough
	case 2:
		this.SetFocus()
		newIdx := row
		oldIdx := this.activeIndex

		if oldIdx != newIdx && newIdx != -1 {
			this.activeIndex = newIdx
			this.emitSelectionChanged(oldIdx)
			this.Layout()
		}
	}
	return
	//	}
	//if this.IsPopup() {
	//	this.Hide()
	//	emulateMouseDown(true)
	//}
}

func (this *ListWidget) OnLeftUp(x, y float64) {
	this.isLeftDown = false
	row, col := this.HitTest(x, y)
	switch col {
	case 1:
		if this.downCol == 1 && this.downRow == row {
			this.items[row].Checked = !this.items[row].Checked
			this.emitCheckChanged(row)
			this.Layout()
		}
	case 0:
		fallthrough
	case 2:
		this.Submit()
	}
	this.downRow, this.downCol = -1, -1
}

func (this *ListWidget) OnMouseMove(x, y float64) {
	if this.isLeftDown {
		if this.downRow != -1 && this.sigDragStart != nil &&
			(math.Abs(x-this.downX) > 4 || math.Abs(y-this.downY) > 4) {
			this.PopCapture()
			data, acts := this.sigDragStart([]int{this.downRow})
			if len(data) != 0 && acts != 0 {
				item := this.items[this.downRow]
				t := Theme()
				pixmap := paint.IconTextToPixmap(item.Icon, t.IconSize, false,
					item.Text, t.Font, t.TextColor, false)
				this.DoDragDrop(pixmap, acts, data...)
				this.isLeftDown = false
			}
			return
		}
	} else {
		row, col := this.HitTest(x, y)
		switch col {
		case 1:
		case 0:
			fallthrough
		case 2:
			newIdx := row
			oldIdx := this.hoverIndex

			if oldIdx != newIdx && newIdx != -1 {
				this.hoverIndex = newIdx
				this.Update()
			}
		}
	}

}

func (this *ListWidget) ActiveIndex() int {
	return this.activeIndex
}

func (this *ListWidget) ActiveItem() ListItem {
	if this.activeIndex >= 0 && this.activeIndex < this.Count() {
		return this.items[this.activeIndex]
	}
	return ListItem{}
}

func (this *ListWidget) SizeHints() SizeHints {
	m := this.padding
	w := 100.0
	h := this.RowHeight() * float64(this.Count())
	w += m.L + m.R
	h += m.T + m.B
	return SizeHints{Width: w, Height: h, Policy: 0}
}

func (this *ListWidget) EnumProperties(list core.IPropertyList) {
	list.AddProperty("显示图标", this.IsIconVisible, this.SetIconVisible)
	list.AddProperty("显示复选框", this.IsCheckBoxVisible, this.SetCheckBoxVisible)
}

func (this *ListWidget) SetDragStartCallback(fn func(idx []int) ([]interface{}, DndAction)) {
	this.sigDragStart = fn
}

// Layout creates/updates the vertical scrollbar when the item list exceeds
// the visible viewport, and hides it when all items fit.
func (this *ListWidget) Layout() {
	m := this.padding
	_, _, _, clientH := m.Apply(0, 0, this.w, this.h)

	rh := this.RowHeight()
	totalRows := float64(this.Count())
	visibleRows := clientH / rh

	vs := this.VertScrollBar()
	if totalRows > visibleRows {
		vs.SetRange(0, totalRows-visibleRows)
		vs.SetDelta(1, visibleRows)
		vs.SetVisible(true)
	} else {
		vs.SetRange(0, 0)
		vs.SetDelta(1, 1)
		vs.SetVisible(false)
		this.SetScrollY(0)
	}

	this.ScrollArea.Layout()
}

// OnMouseWheel scrolls the list by 3 rows per wheel notch.
func (this *ListWidget) OnMouseWheel(x, y, z float64) {
	this.SetScrollY(this.ScrollY() - z*defaultWheelScrollLines)
}

// 键盘导航 (仿Qt QListWidget):
//
//	上/下          : 当前/选中项上移/下移一行, 到头/到尾时夹住
//	Home/End       : 跳到第一/最后一项
//	PageUp/PageDown: 按一个视口页的行数上移/下移
//	回车/空格      : 激活当前项 (走与点击相同的提交回调)
//
// 仅在控件持有焦点时被调用(OnLeftDown里SetFocus后才能收到键盘事件).
func (this *ListWidget) OnKeyDown(key int, repeat bool) {
	n := this.Count()
	if n == 0 {
		return
	}

	switch key {
	case KeyDown:
		this.setActiveIndex(this.activeIndex + 1)
	case KeyUp:
		this.setActiveIndex(this.activeIndex - 1)
	case KeyHome:
		this.setActiveIndex(0)
	case KeyEnd:
		this.setActiveIndex(n - 1)
	case KeyPageDown:
		this.setActiveIndex(pageStepIndex(this.activeIndex, n, this.rowsPerPage(), +1))
	case KeyPageUp:
		this.setActiveIndex(pageStepIndex(this.activeIndex, n, this.rowsPerPage(), -1))
	case KeyEnter, KeySpace:
		this.activateCurrent()
	}
}

// setActiveIndex 把当前/选中行切到idx(夹在有效范围内), 触发选中变化回调,
// 并把该行滚入可见区. 是键盘导航各分支的统一入口.
func (this *ListWidget) setActiveIndex(idx int) {
	idx = clampIndex(idx, this.Count())
	if idx < 0 || idx == this.activeIndex {
		return
	}
	old := this.activeIndex
	this.activeIndex = idx
	this.emitSelectionChanged(old)
	this.scrollRowIntoView(idx)
	this.Layout()
}

// activateCurrent 激活当前项, 与鼠标释放(OnLeftUp)走同一条提交路径.
func (this *ListWidget) activateCurrent() {
	if this.activeIndex < 0 || this.activeIndex >= this.Count() {
		return
	}
	this.Submit()
}

// rowsPerPage 返回视口当前能容纳的整行数(至少1), 作为PageUp/PageDown的步长.
func (this *ListWidget) rowsPerPage() int {
	_, _, _, clientH := this.padding.Apply(0, 0, this.w, this.h)
	rh := this.RowHeight()
	n := int(clientH / rh)
	if n < 1 {
		n = 1
	}
	return n
}

// scrollRowIntoView 调整垂直滚动位置(以行为单位), 让第r行落入可见窗口.
// 复用Layout里"可见行数 = 视口高度 / 行高"的思路.
func (this *ListWidget) scrollRowIntoView(r int) {
	if r < 0 || r >= this.Count() {
		return
	}
	top := int(this.ScrollY())
	if r < top {
		this.SetScrollY(float64(r))
		return
	}
	per := this.rowsPerPage()
	if r >= top+per {
		this.SetScrollY(float64(r - per + 1))
	}
}

// pageStepIndex 计算PageUp/PageDown后落到的行下标: 从cur沿dir方向移动page行,
// 再夹到[0, n)内. cur为-1(无当前行)时, 一次翻页落到首行或末行.
// 拆成纯函数便于在无GL渲染的情况下单测翻页步进逻辑.
func pageStepIndex(cur, n, page, dir int) int {
	if n <= 0 {
		return -1
	}
	if page < 1 {
		page = 1
	}
	if cur < 0 {
		if dir < 0 {
			return n - 1
		}
		return 0
	}
	return clampIndex(cur+dir*page, n)
}
