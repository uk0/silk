package gui

import (
	"silk/core"
	"silk/geom"
	"silk/paint"
	//	"math"
	"math"
	"sort"
)

func init() {
	core.RegisterFactory("gui.TreeView", core.TypeOf((*TreeView)(nil))) //((*TreeView)(nil)))
}

const (
	TWC_CONTENT  = 0
	TWC_EXPANDER = 1
	TWC_ICON     = 2
)

func NewTreeView() *TreeView {
	p := new(TreeView)
	p.Init(p)
	return p
}

type TreeViewRow struct {

	// 对应的索引
	// 因为存在多视图, 所以单索引可以对应多个TreeViewRow
	mi ModelIndex

	// y偏移量
	// 因为不同行高度可以不同, 所以通常情况下ypos要通过累加的出
	// 只有显式设置为"统一行高"时才能用行高乘以行数
	ypos float64

	// 是否展开, 由用户点击"+"或"-"号来操作
	// 有时因为各种原因, 展开后子节点个数可能为0,
	// 但这不影响程序的逻辑, 无需作特殊处理, 用户仍可自由点击"+"或"-"号
	expanded bool

	// 当前行是否被选中
	selected bool

	// (总)行号, 随着上方结点的展开/收缩而变动
	ri int

	// 缩进等级
	indent int
}

// 支持模型-视图机制的树形视图
type TreeView struct {
	GuiView

	titleProp
	iconProp

	// 简单起见, 目前先用slice来存放, 等确实发现"展开/收缩"操作慢了再考虑优化
	rows []*TreeViewRow

	// ModelIndex和TreeViewRow在当前视图里的对应关系
	// 注1: 原本多个ModelIndex可以对应到同一行, 但我们只用第0列来索引
	// 注2: 也不能用Internal来作索引 (例: 井号做internal时, 一口井可有多个井号)
	rmap map[ModelIndex]*TreeViewRow

	// 根节点
	// 注: 不一定是模型的根结点, 可能是子树
	rootSet map[*TreeViewRow]int

	// 缩进根节点, 把第一层子节点顶格
	indentRoot bool

	// 统一行高
	uniformRowHeight bool

	// 行高
	defRowHeight float64

	header *HeaderView

	hh float64

	padding Padding
}

func (this *TreeView) Init(iw IWidget) {
	this.GuiView.Init(iw)
	this.defRowHeight = Theme().ItemHeight
	this.uniformRowHeight = false
	this.header = NewHeaderView()
	this.header.SetParent(iw)
	this.header.SetVisible(true)
	this.hh = Theme().ItemHeight
	this.padding = Theme().EditPadding
}

func (this *TreeView) initRoots() {
	this.rows = nil
	this.rootSet = make(map[*TreeViewRow]int)
	rootCount := this.model.RowCount(ModelIndex{})

	var rootIndent int
	if this.indentRoot {
		rootIndent = -1
	} else {
		rootIndent = 0
	}
	y := 0.0 // y坐标独立于client, 这样在隐藏header时不用重算y坐标
	for ri := 0; ri < rootCount; ri++ {
		mi := this.model.Index(ri, 0, ModelIndex{})
		if mi.IsNil() {
			panic("")
		}
		if ri != mi.Row {
			panic("")
		}
		row := this.getRow(mi, true)
		row.ri = ri
		row.indent = rootIndent
		row.ypos = y
		this.rows = append(this.rows, row)
		this.rootSet[row] = 1

		y += this.calcRowHeight(row)
	}

}

func (this *TreeView) firstRow() *TreeViewRow {
	if len(this.rows) == 0 {
		return nil
	}
	return this.rows[0]
}

func (this *TreeView) OnBeginReset() {
	this.rmap = make(map[ModelIndex]*TreeViewRow)
	this.rootSet = nil
	this.rows = nil
}

func (this *TreeView) OnEndReset() {
	//this.root = this.getRow(this.RootIndex())
	this.initRoots()
	//for p, _ := range this.rootSet {
	//	this.expand(p)
	//}
	//this.expand(this.firstRow())
	this.ExpandAll(0)
}

//func (this *TreeView) setRootIndex(m IGuiModel, mi ModelIndex) {
//	mi = mi.SameRow(0)
//	this.model = m
//}

func (this *TreeView) SetModel(m IGuiModel) {
	this.OnBeginReset()
	this.bindItemModel(m)
	this.OnEndReset()

	this.header.SetModel(m)
}

func (this *TreeView) Model() IGuiModel {
	return this.model
}

//func (this *TreeView) SetRootIndex(mi ModelIndex) {
//	this.setRootIndex(mi.Model, mi)
//}

//func (this *TreeView) RootIndex() ModelIndex {
//	if this.model == nil {
//		return ModelIndex{}
//	}
//	return this.model.Index(0, 0, ModelIndex{})
//}

func (this *TreeView) getRow(col0mi ModelIndex, create bool) *TreeViewRow {
	p, ok := this.rmap[col0mi]
	if !ok {
		if !create {
			return nil
		}
		if !col0mi.IsNil() {
			p = new(TreeViewRow)
			p.mi = col0mi
		}
		this.rmap[col0mi] = p
	}
	return p
}

func (this *TreeView) deleteRow(mi ModelIndex) {
	delete(this.rmap, mi)
}

func (this *TreeView) deleteRow1(p *TreeViewRow) {
	delete(this.rmap, p.mi)
}

func (this *TreeView) SetRootIndent(indent bool) {
	if this.indentRoot == indent {
		return
	}
	if this.indentRoot {
		for _, p := range this.rows {
			p.indent++
		}
	} else {
		for _, p := range this.rows {
			p.indent--
		}
	}
	this.indentRoot = indent
	this.Update()
}

func (this *TreeView) RootIndent() (indent bool) {
	return this.indentRoot
}

//func (this *TreeView) firstRow() *TreeViewRow {
//	if len(this.rows) == 0 {
//		return nil
//	}
//	return this.rows[0]
//}

//func (this *TreeView) hasChildren(mi ModelIndex) bool {
//	if i, ok := this.model.(interface {
//		HasChildren(ModelIndex) bool
//	}); ok {
//		return i.HasChildren(mi)
//	}
//	return this.model.RowCount(mi) > 0
//}

func (this *TreeView) Expand(mi ModelIndex) {
	this.expand(this.getRow(mi, false))
}

func (this *TreeView) ExpandAll(depth int) {
	for i := 0; i < len(this.rows); i++ {
		if this.rows[i].indent <= depth {
			this.expand(this.rows[i])
		}
	}
}

func (this *TreeView) Collapse(mi ModelIndex) {
	this.collapse(this.getRow(mi, false))
}

func (this *TreeView) expand(row *TreeViewRow) {
	//core.Debug("expand(row *TreeViewRow)")
	if row == nil || row.expanded {
		return
	}

	rc := this.model.RowCount(row.mi)
	//core.Debug("rc = ", rc)
	if rc <= 0 {
		return
	}

	var rs []*TreeViewRow
	y0 := row.ypos + this.rowHeight(row.ri)
	y := y0
	for i := 0; i < rc; i++ {
		mi := this.model.Index(i, 0, row.mi)
		p := this.getRow(mi, true)
		p.ri = row.ri + i + 1
		p.ypos = y
		p.indent = row.indent + 1
		rs = append(rs, p)
		y += this.calcRowHeight(p)
	}

	h := y - y0
	//core.Debug("h = ", h)

	for i := row.ri + 1; i < len(this.rows); i++ {
		p := this.rows[i]
		p.ri += rc
		p.ypos += h
	}

	newRows := make([]*TreeViewRow, len(this.rows)+rc)
	copy(newRows[:row.ri+1], this.rows)
	copy(newRows[row.ri+1:row.ri+1+rc], rs)
	copy(newRows[row.ri+1+rc:], this.rows[row.ri+1:])
	this.rows = newRows

	row.expanded = true

	for _, v := range rs {
		if v.expanded {
			v.expanded = false
			this.expand(v)
		}
	}

	this.Layout()
}

func (this *TreeView) collapse(row *TreeViewRow) {
	if row == nil || !row.expanded {
		return
	}

	y0 := row.ypos + this.rowHeight(row.ri)
	y := y0
	rc := 0
	for i := row.ri + 1; i < len(this.rows); i++ {
		p := this.rows[i]
		if p.indent <= row.indent {
			break
		}
		rc++
		y += this.rowHeight(i)
	}
	h := y - y0
	if rc > 0 {
		for i := row.ri + 1; i < len(this.rows)-rc; i++ {
			this.rows[i] = this.rows[i+rc]
			this.rows[i].ypos -= h
			this.rows[i].ri -= rc
		}
		this.rows = this.rows[:len(this.rows)-rc]
	}
	row.expanded = false
	this.Layout()
}

func (this *TreeView) RowCount() int {
	return len(this.rows)
}

func (this *TreeView) updateVertScrollBar() {
	ch := this.h - this.headerHeight()
	ch -= Theme().ItemHeight + Theme().ScrollWidth

	lr := len(this.rows)
	lh := 0.0
	for lr > 0 {
		a := this.rowHeight(lr)
		if lh+a >= ch {
			break
		}
		lh += a
		lr--
	}
	//core.Debug(lr)
	vs := this.VertScrollBar()
	if lr == 0 {
		vs.SetRange(0, 0)
		vs.SetVisible(false)
		vs.SetValue(0)
	} else {
		vs.SetRange(0, float64(lr))
		vs.SetVisible(true)
		vs.SetDelta(1, 5)
	}
	this.Update()
}

func (this *TreeView) updateHorzScrollBar() {
	cw := this.Width()
	vs := this.VertScrollBar()
	if vs.IsVisible() {
		cw -= vs.Width()
	}
	tw := this.header.TotalSectionSize()
	hs := this.HorzScrollBar()
	if cw > tw {
		hs.SetRange(0, 0)
		hs.SetVisible(false)
		hs.SetValue(0)
	} else {
		hs.SetRange(0, tw-cw)
		hs.SetVisible(true)
		hs.SetDelta(16, cw-32)
	}

	this.Update()
}

func (this *TreeView) Layout() {

	if this.header.IsVisible() {
		rect := geom.Rect{0, 0, this.Width(), this.Height()}
		rect = this.padding.Apply1(rect)
		rect.Height = this.hh
		this.header.SetBounds1(rect)
		this.header.SetScrollX(this.ScrollX())
	}

	this.updateVertScrollBar()
	this.updateHorzScrollBar()

	this.GuiView.Layout()
}

func (this *TreeView) headerHeight() float64 {
	if this.header.IsVisible() {
		return this.hh
	}
	return 0
}

func (this *TreeView) getRow1(r int) *TreeViewRow {
	if r < 0 {
		return nil
	}

	n := len(this.rows)

	if n == 0 {
		return nil
	}

	if r >= n {
		return nil
	}

	return this.rows[r]
}

func (this *TreeView) clientRect() geom.Rect {
	rect := geom.Rect{0, 0, this.Width(), this.Height()}
	rect = this.padding.Apply1(rect)
	rect.Height -= this.headerHeight()
	rect.Y += this.headerHeight()
	//rect.Y -= this.ScrollYPx()
	return rect
}

func (this *TreeView) logicRect() geom.Rect {
	rect := geom.Rect{0, 0, this.Width(), this.Height()}
	rect.Width -= this.padding.L + this.padding.R
	rect.Height -= this.padding.T + this.padding.B
	rect.X += this.ScrollXPx()
	rect.Y += this.ScrollYPx()
	return rect
}

func (this *TreeView) ScrollPosPx() (x, y float64) {
	x = this.ScrollXPx()
	y = this.ScrollYPx()
	return x, y
}

func (this *TreeView) ScrollXPx() float64 {
	return this.ScrollX()
}

func (this *TreeView) ScrollYPx() float64 {
	if len(this.rows) > 0 {
		r := int(this.ScrollY())
		if r < len(this.rows) {
			return this.rows[r].ypos
		} else {
			return this.rows[len(this.rows)-1].ypos
		}
	}
	return 0
}

func (this *TreeView) OnMouseWheel(x, y, z float64) {
	this.SetScrollY(this.ScrollY() - z*defaultWheelScrollLines)
}

func (this *TreeView) rowYPos(r int) float64 {
	if r < 0 {
		return 0
	}

	n := len(this.rows)

	if n == 0 {
		return 0
	}

	if r >= n {
		return this.rows[n-1].ypos
	}

	return this.rows[r].ypos
}

// [row0, row1)
func (this *TreeView) VisibleColRange() (col0, col1 int) {
	return 0, 0
}

// [row0, row1)
func (this *TreeView) VisibleRowRange() (row0, row1 int) {
	rect := this.logicRect()
	return this.rowsIndexRange(rect.Y, rect.Bottom())
}

func (this *TreeView) calcRowHeight(row *TreeViewRow) float64 {
	if this.uniformRowHeight || row == nil || row.mi.IsNil() {
		return this.defRowHeight
	}

	i := this.model.Data(row.mi, SizeHintRole)
	vec, ok := i.(geom.Vec2)
	if !ok {
		return this.defRowHeight
	}

	if vec.Y <= 0 {
		return this.defRowHeight
	}
	return vec.Y
}

func (this *TreeView) Close() {
	this.header.Close()
	this.GuiView.Close()
}

func (this *TreeView) rowAt(ypos float64) *TreeViewRow {
	idx := this.rowIndexAtScrolled(ypos)
	if idx < 0 || idx >= len(this.rows) {
		return nil
	}
	return this.rows[idx]
}

// 显示的行号[-1, size]
func (this *TreeView) rowIndexAtScrolled(ypos float64) int {
	return sort.Search(len(this.rows), func(i int) bool {
		return this.rowBottom(i) >= ypos
	})
}

// 显示的列号[-1, size]
func (this *TreeView) colIndexAtScrolled(xpos float64) int {
	return this.header.logicIndexAtScrolled(xpos)
}

func (this *TreeView) rowIndentPx(row *TreeViewRow) float64 {
	return float64(row.indent) * this.expanderSize()
}

// return [i0, i1)
func (this *TreeView) rowsIndexRange(y0, y1 float64) (i0, i1 int) {
	//core.Debug("y0 y1=", y0, y1)
	i0 = this.rowIndexAtScrolled(y0)
	i1 = this.rowIndexAtScrolled(y1)
	if i0 > i1 {
		i0, i1 = i1, i0
	}
	return
}

func (this *TreeView) MapToScrolled(x, y float64) (x1, y1 float64) {
	client := this.clientRect()
	x1 = x - client.X + this.ScrollXPx()
	y1 = y - client.Y + this.ScrollYPx()
	return x1, y1

}

func (this *TreeView) MapFromScrolled(x, y float64) (x1, y1 float64) {
	client := this.clientRect()
	x1 = x + client.X - this.ScrollXPx()
	y1 = y + client.Y - this.ScrollYPx()
	return x1, y1
}

func (this *TreeView) OnLeftDown(x, y float64) {
	x1, y1 := this.MapToScrolled(x, y)

	row, detail := this.hitTestScrolled(x1, y1)

	if row != nil && detail == TWC_EXPANDER {
		if row.expanded {
			this.collapse(row)
		} else {
			this.expand(row)
		}
	}
}

func (this *TreeView) rowHeight(i int) float64 {
	if i > len(this.rows) {
		return 0
	}
	if i == len(this.rows) {
		return this.defRowHeight
	}
	return this.rowBottom(i) - this.rows[i].ypos
}

func (this *TreeView) rowBottom(i int) float64 {
	if i < 0 {
		return 0
	}
	sz := len(this.rows)
	if i >= sz {
		return this.rowBottom(sz - 1)
	}
	if i >= sz-1 {
		if sz > 0 {
			return this.rows[sz-1].ypos + this.calcRowHeight(this.rows[sz-1])
		}
		return 0
	}
	return this.rows[i+1].ypos
}

func (this *TreeView) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	g.SetFont(Theme().Font)

	client := this.clientRect()
	g.Rectangle1(client)
	g.SetBrush1(Theme().ViewBGColor)
	g.Fill()

	Theme().DrawEditFrame(g, 0, 0, this.w, this.h,
		this.HasFocus(), this.IsHover(), !this.IsEnabled())

	g.Translate(client.X, client.Y)
	lrect := this.logicRect()
	g.Translate(-lrect.X, -lrect.Y)

	r0, r1 := this.VisibleRowRange()
	//core.Debug("r0  r1=", r0, r1)

	for r := r0; r < r1; r++ {
		row := this.getRow1(r)
		if row == nil {
			continue
		}
		y := row.ypos
		h := this.calcRowHeight(row)
		for vc := 0; vc < this.header.SectionCount(); vc++ {
			colInfo := this.header.VisualSection(vc)
			if colInfo.Hidden {
				continue
			}
			c := colInfo.LogicIndex
			x := colInfo.Offset
			w := colInfo.Size
			//g.SetPen1(paint.Color{255, 128, 255, 255}, 1)
			//g.Rectangle(x, y, w, h)
			//g.Stroke()
			this.drawCell(g, row, c, x, y, w, h)
		}
	}
}

func (this *TreeView) expanderIconSize() float64 {
	return 11 //Theme().IconSize
}

func (this *TreeView) expanderSize() float64 {
	return this.expanderIconSize() + 4
}

func (this *TreeView) drawCell(g paint.Painter, row *TreeViewRow, ci int, x, y, w, h float64) {
	if ci == 0 {
		this.drawCol0Cell(g, row, x, y, w, h)
		return
	}
	this.drawCellContent(g, row, ci, x, y, w, h)
}

func (this *TreeView) hitTestScrolled(x, y float64) (row *TreeViewRow, detail int) {
	ri := this.rowIndexAtScrolled(y)
	ci := this.colIndexAtScrolled(x)
	if ri < 0 || ri >= this.RowCount() ||
		ci < 0 || ci >= this.ColCount() {
		return
	}

	sec := this.header.VisualSection(ci)
	if sec.Hidden {
		return
	}

	row = this.rows[ri]
	rect := geom.Rect{sec.Offset, row.ypos, sec.Size, this.rowHeight(ri)}

	if sec.LogicIndex == 0 && row.indent >= 0 && this.rowHasChildren(row) {
		indent := this.rowIndentPx(row)
		expandSize := this.expanderSize()
		if x >= rect.X+indent && x < rect.X+indent+expandSize {
			detail = TWC_EXPANDER
			return
		}
	}
	return
}

/*
func (this *TreeView) HitTest(x, y float64) (mi ModelIndex, detail int) {
	x1, y1 := this.MapToScrolled(x, y)
	ri := this.rowIndexAtScrolled(y1)
	ci := this.colIndexAtScrolled(x1)
	if ri < 0 || ri >= this.RowCount() ||
		ci < 0 || ci >= this.ColCount() {
		return
	}
	row := this.rows[ri]
	mi = p.mi.SameRow(ci)

	rect, ok := this.cellRectScrolled(mi)
	if !ok {
		return
	}
	if ci == 0 && this.rowHasChildren(row) {
		indent := this.rowIndentPx(row)
		expandSize := this.expanderSize()
		if x1 >= rect.X+indent && x < rect.X+indent+expandSize {
			detail = TWC_EXPANDER
			return
		}
	}
	return
}
*/

func (this *TreeView) cellRectScrolled(mi ModelIndex) (rect geom.Rect, ok bool) {
	if mi.IsNil() {
		return
	}

	row := this.getRow(mi.SameRow(0), false)
	if row == nil {
		return
	}

	sec := this.header.LogicSection(mi.Col)
	if sec.Hidden {
		return
	}
	y := row.ypos
	h := this.rowHeight(row.ri)
	x := sec.Offset
	w := sec.Size
	return geom.Rect{x, y, w, h}, true
}

func (this *TreeView) drawCol0Cell(g paint.Painter, row *TreeViewRow, x, y, w, h float64) {
	//g.SetPen1(paint.Color{0, 128, 255, 255}, 1)
	//g.Rectangle(x, y, w, h)
	//g.Stroke()

	indent := this.rowIndentPx(row)
	x += indent
	w -= indent
	expandSize := this.expanderSize()

	if row.indent >= 0 && this.rowHasChildren(row) {
		var icon paint.Icon
		if row.expanded {
			icon = Theme().ExpandedIcon
		} else {
			icon = Theme().CollapsedIcon
		}

		sz := this.expanderIconSize()
		sz1 := this.expanderSize()
		xi := x + math.Floor(0.5*(sz1-sz))
		yi := y + math.Floor(0.5*(h-sz))
		g.DrawIcon1(icon, xi, yi, sz, false)
	}

	x1 := x + expandSize
	w1 := w - expandSize
	if w1 > 0 {
		this.drawCellContent(g, row, 0, x1, y, w1, h)
	}
}

func (this *TreeView) drawCellContent(g paint.Painter, row *TreeViewRow, ci int, x, y, w, h float64) {
	//g.SetPen1(paint.Color{255, 128, 255, 255}, 1)
	//g.Rectangle(x, y, w, h)
	//g.Stroke()
	g.Save()
	defer g.Restore()

	g.Rectangle(x, y, w, h)
	g.Clip()

	g.SetBrush1(Theme().TextColor)
	txt := this.getCellText(row, ci)
	fe := g.Font().FontExtents()
	g.DrawText1(x+2, y+fe.Ascent+0.5*(h-fe.Height), txt)
	//g.DrawText1(x+2, y+fe.Ascent+0.5*(this.rowHeight(row.ri)-fe.Height), txt)
}

func (this *TreeView) getCellText(row *TreeViewRow, ci int) string {
	d := this.model.Data(row.mi.SameRow(ci), DisplayRole)
	return core.VisualString(d)
}

func (this *TreeView) OnHorzScroll(sender IWidget) {
	this.header.SetScrollX(this.ScrollX())
	this.Self().Update()
}

func (this *TreeView) rowHasChildren(row *TreeViewRow) bool {
	return this.model.HasChildren(row.mi)
}

func (this *TreeView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("根缩进", this.RootIndent, this.SetRootIndent)
}

func (this *TreeView) ColCount() int {
	return this.model.ColCount()
}
