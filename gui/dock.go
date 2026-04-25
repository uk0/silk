package gui

import (
	"silk/core"
	"silk/geom"
	"silk/gv"
	"silk/paint"
	//	"math"
)

func init() {
	core.RegisterFactory("gui.Dock", core.TypeOf((*Dock)(nil)))
}

func NewDock() *Dock {
	p := new(Dock)
	p.Init(p)
	return p
}

// Dock是Frame里的"子框架", 用作视图的容器
type IDock interface {
	IWidget
	IsMainDock() bool
	RemoveIndex(int) IWidget
	DetachIfEmpty() bool
	ViewCount() int
	AddView(iw IWidget)
	InsertView(idx int, iw IWidget)
	Frame() *Frame
	AllViews() []IWidget
	Close()

	CloseDocViews()
	CloseAllViews()

	PromptSaveCloseIndex(idx int) bool
	PromptSaveCloseView(iw IWidget) bool
	CloseIndex(idx int) bool
	CloseView(iw IWidget) bool

	ActiveIndex() int
	ActiveView() IWidget

	Layout()
}

// Dock是Frame里的"子框架", 用作视图的容器
type Dock struct {
	Widget
	Brick

	tabH    float64
	headH   float64 // tab + toolbar
	menuW   float64
	menuH   float64
	tabbar  *TabBar
	minBtn  *Button
	maxBtn  *Button
	hoverHT dockHitTest
	downHT  dockHitTest

	isMainDock bool
	isDown     bool
	isDraging  bool
	isMax      bool
	isMin      bool
	isTempMin  bool // ignore when !isMinimized
	menuInRow0 bool
	dropRect   *geom.Rect

	menu         *Menu
	menuCache    map[IWidget]*Menu
	cbTabChanged func(int)
}

type dockHitTest struct {
	part  string
	index int
}

type dockDndHandle struct {
	dock  IDock
	view  IWidget
	index int
}

func (this *Dock) Init(self IWidget) {
	this.Widget.Init(self)
	this.Brick.ot = self.(IBrick)
	this.tabbar = NewTabBar()
	this.tabbar.SetParent(this)
	this.tabbar.SetDndCallback(
		func(tb *TabBar, idx int) interface{} {
			return &dockDndHandle{this, tb.Data(idx).(IWidget), idx}
		},
		func(tb *TabBar, dnd IDndContext) {

		},
		func(tb *TabBar, idx int, dnd IDndContext) {

		})
	this.tabbar.SetActivateCallback(func(tb *TabBar, idx int) {
		this.Layout()
		if this.cbTabChanged != nil {
			this.cbTabChanged(idx)
		}
	})
	this.tabbar.SetCloseCallback(func(tb *TabBar, idx int) bool {
		return this.PromptSaveCloseIndex(idx)
	})

	this.minBtn = NewButton1("", LoadIcon("minimize"))
	this.minBtn.SetTextVisible(false)
	this.minBtn.SetParent(this)
	this.minBtn.Action().BindFunc0(func() {
		this.toggleMinimize()
	})

	this.maxBtn = NewButton1("", LoadIcon("maximize"))
	this.maxBtn.SetTextVisible(false)
	this.maxBtn.SetParent(this)
	this.maxBtn.Action().BindFunc0(func() {
		this.toggleMaximize()
	})
}

func (this *Dock) Detach() {
	this.Brick.Detach()
	this.Widget.Detach()
	if this.Frame() != nil {
		this.Frame().Layout()
	}
}

func (this *Dock) AddView(iw IWidget) {
	iw.SetParent(this)
	this.tabbar.AddTab(iw, true)

	frame := this.Frame()
	if p, ok := frame.toolViews[core.FactoryNameOf(iw)]; ok {
		if p.Widget == nil {
			p.Widget = iw
		}
	}

	this.switchMenu()
	this.Layout()
}

func (this *Dock) InsertView(idx int, iw IWidget) {
	iw.SetParent(this)
	this.tabbar.InsertTab(idx, iw, true)
	this.Layout()
}

// 从Dock上移除视图
// 注: 所有关闭/移除视图的操作, 最终都调用此接口来执行
func (this *Dock) RemoveIndex(idx int) IWidget {
	i := this.tabbar.RemoveTab(idx)
	iw := i.(IWidget)
	iw.SetParent(nil)

	frame := this.Frame()
	for _, v := range frame.toolViews {
		if v.Widget == iw {
			v.Widget = nil
			v.DockPath = DockPath(this)
			core.Debug("DockPath = ", v.DockPath)
			v.ViewIdx = idx
			break
		}
	}

	delete(this.menuCache, iw)
	this.switchMenu()
	this.Layout()
	this.DetachIfEmpty()
	return iw
}

func (this *Dock) RemoveView(iw IWidget) {
	idx := this.IndexOfView(iw)
	if idx == -1 {
		return
	}
	this.RemoveIndex(idx)
}

// 关闭视图
// 除了移除以外, 还调用视图的Close接口
func (this *Dock) CloseIndex(idx int) bool {
	iw := this.RemoveIndex(idx)
	if iw == nil {
		return false
	}

	if iclose, ok := iw.(interface {
		Close()
	}); ok {
		iclose.Close()
	}
	return true
}

func (this *Dock) CloseView(iw IWidget) bool {
	idx := this.IndexOfView(iw)
	if idx == -1 {
		return false
	}
	return this.CloseIndex(idx)
}

func (this *Dock) IndexOfView(iw IWidget) int {
	for i := this.ViewCount() - 1; i >= 0; i-- {
		x := this.ViewAtIndex(i)
		if x == iw {
			return i
		}
	}
	return -1
}

func (this *Dock) ViewAtIndex(idx int) IWidget {
	i := this.tabbar.Data(idx)
	return i.(IWidget)
}

func (this *Dock) PromptSaveCloseIndex(idx int) bool {
	iw := this.ViewAtIndex(idx)
	if PromptSaveClose(this, iw) {
		this.RemoveIndex(idx)
		return true
	}
	return false
}

func (this *Dock) PromptSaveCloseView(iw IWidget) bool {
	idx := this.IndexOfView(iw)
	if idx == -1 {
		return false
	}
	return this.PromptSaveCloseIndex(idx)
}

//func (this *Dock) PromptSaveCloseDock() bool {
//	return PromptSaveClose(this, this)
//}

func (this *Dock) IsMainDock() bool {
	return this.isMainDock
}

func (this *Dock) ContainMainDock() bool {
	return this.isMainDock
}

func (this *Dock) IsVisible() bool {
	return this.Widget.IsVisible()
}

func (this *Dock) SetBounds(x, y, w, h float64) {
	this.Widget.SetBounds(x, y, w, h)
	this.Brick.SetBounds(x, y, w, h)
}

func (this *Dock) Bounds() (x, y, w, h float64) {
	return this.Widget.Bounds()
}

func (this *Dock) SetBounds1(rc geom.Rect) {
	this.Widget.SetBounds1(rc)
	this.Brick.SetBounds1(rc)
}

func (this *Dock) Bounds1() geom.Rect {
	return this.Widget.Bounds1()
}

func (this *Dock) Draw(g paint.Painter) {
	//if this.tabbar.IsEmpty() {
	t := Theme()
	g.Rectangle(0, this.headH, this.Widget.w, this.Widget.h-this.headH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	g.Rectangle(0, 0, this.Widget.w, this.headH)
	g.SetBrush1(t.FormDarkColor)
	g.Fill()
	//}
}

func (this *Dock) DrawOverlay(g paint.Painter) {
	if this.dropRect != nil {
		g.Rectangle(this.dropRect.X+4, this.dropRect.Y+4, this.dropRect.Width-8, this.dropRect.Height-8)
		g.SetBrush1(paint.Color{0, 160, 255, 64})
		g.Fill()
	}

}

func (this *Dock) SelfBrick() IBrick {
	return this.Self().(IBrick)
}

//func (this *Dock) doDrop(t IBrick, xp, yp float64) {
//	split, left, vert, merge := this.SplitHint(xp, yp)
//	if merge {
//		core.Unimplemented()
//		return
//	}
//	this.Split(t, split, left, vert)
//}

//func (this *Dock) setParentBrick(a IBrick) {
//	this.parentBrick = a
//	//if a != nil {
//	//	this.SetParent(a.Frame())
//	//	//this.setFrame(a.Frame())
//	//} else {
//	//	this.SetParent(nil)
//	//	//this.setFrame(nil)
//	//}
//}

func (this *Dock) setFrame(a *Frame) {
	if this.Brick.frame == a {
		return
	}
	this.Brick.setFrame(a)
	this.SetParent(a)
	//if a != nil {
	//	this.SetParent(a)
	//	//this.setFrame(a.Frame())
	//} else {
	//	this.SetParent(nil)
	//	//this.setFrame(nil)
	//}

}

func (this *Dock) SizeHints() SizeHints {
	return SizeHints{}
}

func (this *Dock) Layout() {
	//core.Debug("(this *Dock) Layout()")
	this.switchMenu()
	hints := this.tabbar.SizeHints()
	this.tabbar.SetBounds(0, 0, hints.Width, hints.Height)
	this.tabH = hints.Height
	this.headH = hints.Height
	if this.menu != nil {
		menuHints := this.menu.SizeHints()
		this.headH += menuHints.Height
		this.menu.SetBounds(0, this.tabH, this.Width(), menuHints.Height)
	}
	//	this.tile.SetBounds(0, hints.Height, this.Width(), this.Height()-hints.Height)
	count := this.tabbar.Count()
	active := this.tabbar.ActiveTab()
	for i := 0; i < count; i++ {
		iw := this.tabbar.Data(i).(IWidget)
		iw.SetVisible(active == i)
		iw.SetBounds(0, this.headH, this.Width(), this.Height()-this.headH)
	}

	this.maxBtn.SetBounds(this.Width()-hints.Height, 0, hints.Height, hints.Height)
	this.minBtn.SetBounds(this.Width()-hints.Height*2, 0, hints.Height, hints.Height)

	this.Self().Update()

}

func (this *Dock) AllViews() (ret []IWidget) {
	count := this.tabbar.Count()
	for i := 0; i < count; i++ {
		iw := this.tabbar.Data(i).(IWidget)
		ret = append(ret, iw)
	}
	return
}

func (this *Dock) ActiveIndex() int {
	return this.tabbar.ActiveTab()
}

func (this *Dock) SetActiveIndex(idx int) {
	if idx >= 0 && idx < this.tabbar.Count() {
		this.tabbar.SetActiveTab(idx)
		this.Layout()
	}
}

// SetTabChangedCallback registers a callback invoked when the active tab changes.
// The callback receives the new active tab index.
func (this *Dock) SetTabChangedCallback(cb func(int)) {
	this.cbTabChanged = cb
}

func (this *Dock) ActiveView() IWidget {
	active := this.tabbar.ActiveTab()
	data := this.tabbar.Data(active)
	if data == nil {
		return nil
	}
	return data.(IWidget)
}

func (this *Dock) extractDndHandle(dnd IDndContext) *dockDndHandle {
	if dnd.HasFormat("[]interface{}") {
		data := dnd.Data("[]interface{}").([]interface{})
		for _, v := range data {
			if p, ok := v.(*dockDndHandle); ok {
				return p
			}
		}
	}
	return nil
}

func (this *Dock) isNearFrameBorder(x, y float64) bool {
	frame := this.Frame()
	root := frame.RootBrick()
	rx, ry, rw, rh := root.Bounds()
	//core.Debug(rx, ry, rw, rh)
	const M = 20.0
	x += this.x
	y += this.y
	//core.Debug(x, y)
	return x < rx+M || y < ry+M ||
		x > rx+rw-M || y > ry+rh-M
}

func (this *Dock) OnDragEnter(x, y float64, dnd IDndContext) {
	this.dropRect = nil
	if this.isNearFrameBorder(x, y) {
		dnd.SetAction(0)
		return
	}
	handle := this.extractDndHandle(dnd)
	if handle == nil {
		dnd.SetAction(DndIgnore)
		return
	}
	dnd.SetAction(DndMove)
}

func (this *Dock) OnDragLeave() {
	if this.dropRect != nil {
		this.dropRect = nil
		this.Self().Update()
	}

}

func (this *Dock) setDropRect(rc *geom.Rect) {
	if rc == nil {
		if this.dropRect != nil {
			this.dropRect = nil
			this.Self().Update()
		}
		return
	}
	if this.dropRect == nil || *rc != *this.dropRect {
		this.dropRect = rc
		this.Self().Update()
	}
}

func (this *Dock) OnDragMove(x, y float64, dnd IDndContext) {
	if this.isNearFrameBorder(x, y) {
		dnd.SetAction(0)
		this.setDropRect(nil)
		return
	}
	handle := this.extractDndHandle(dnd)
	if handle == nil {
		dnd.SetAction(DndIgnore)
		this.setDropRect(nil)
		return
	}

	// drop to self
	if handle.dock == this && !this.IsMainDock() && this.ViewCount() == 1 {
		dnd.SetAction(DndMove)
		this.setDropRect(&geom.Rect{0, 0, this.w, this.h})
		return
	}

	srcFrame := handle.dock.Frame()
	if srcFrame.IsToolView(handle.view) && srcFrame != this.Frame() {
		// 工具视图不允许跨Frame拖放
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		return
	}

	dnd.SetAction(DndMove)
	x1 := this.Widget.x + x
	y1 := this.Widget.y + y
	//	core.Debug(x1, y1)
	split, vert, left, merge := this.Brick.DropSplitHint(x1, y1)
	rc := new(geom.Rect)
	rc.X, rc.Y, rc.Width, rc.Height = this.Brick.DropRect(split, vert, left, merge)
	rc.X -= this.Widget.x
	rc.Y -= this.Widget.y
	this.setDropRect(rc)

}

func (this *Dock) OnDrop(x, y float64, dnd IDndContext) {
	this.Self().Update()
	if this.dropRect != nil {
		this.dropRect = nil
	}
	if this.isNearFrameBorder(x, y) {
		dnd.SetAction(DndIgnore)
		core.Debug("near border")
		return
	}

	handle := this.extractDndHandle(dnd)
	if handle == nil {
		dnd.SetAction(DndIgnore)
		core.Debug("Dock: drop handle is nil")
		return
	}

	if handle.dock == this && !this.IsMainDock() && this.ViewCount() == 1 {
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		this.Self().Update()
		return
	}

	srcFrame := handle.dock.Frame()
	if srcFrame.IsToolView(handle.view) && srcFrame != this.Frame() {
		// 工具视图不允许跨Frame拖放
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		return
	}

	dnd.SetAction(DndMove)
	x1 := this.Widget.x + x
	y1 := this.Widget.y + y
	//this.doDrop(handle, x1, y1)
	frame := this.Frame()
	split, vert, left, merge := this.Brick.DropSplitHint(x1, y1)
	if merge {
		if this != handle.dock {
			iw := handle.dock.RemoveIndex(handle.index)
			this.AddView(iw)
		}
	} else {
		iw := handle.dock.RemoveIndex(handle.index)
		dock := NewDock()
		dock.SetParent(frame)
		dock.AddView(iw)
		this.Split(dock, split, vert, left)
	}
	//handle.dock.DetachIfEmpty()
	frame.Layout()
}

func (this *Dock) ViewCount() int {
	return this.tabbar.Count()
}

func (this *Dock) DetachIfEmpty() bool {
	if this.isMainDock ||
		this.ParentBrick() == nil ||
		!this.tabbar.IsEmpty() {
		this.Layout()
		return false
	}

	this.Detach()
	return true
}

func (this *Dock) loadViewMenu(view IWidget) (menu *Menu) {
	if i, ok := view.(interface {
		Actions() []IAction
	}); ok {
		menu = BuildMenu(i.Actions(), "")
	}
	if menu == nil {
		core.Debug("(*Dock) loadViewMenu(...): failed to load menu.")
	} else {
		menu.SetParent(this)
	}
	return
}

func (this *Dock) switchMenu() { //core.Debug("(this *Dock) switchMenu(view IWidget)")
	view := this.ActiveView()
	if this.menuCache == nil {
		this.menuCache = make(map[IWidget]*Menu)
	}

	menu, ok := this.menuCache[view]
	if !ok {
		menu = this.loadViewMenu(view)
		this.menuCache[view] = menu
	}

	if this.menu != nil && this.menu != menu {
		this.menu.Hide()
	}

	this.menu = menu

	if this.menu != nil {
		this.menu.Show()
	}
}

func (this *Dock) OnIdle() {
	for _, v := range this.Children() {
		if v.IsVisible() {
			if im, ok := v.(interface {
				OnIdle()
			}); ok {
				im.OnIdle()
			}
		}
	}
}

func (this *Dock) Close() {
	this.CloseAllViews()
}

// savedBounds stores the dock bounds before minimize/maximize for restore
type dockSavedBounds struct {
	x, y, w, h float64
}

var dockSavedState = make(map[*Dock]*dockSavedBounds)

// toggleMinimize collapses the dock panel to just its tab bar height, or restores it.
func (this *Dock) toggleMinimize() {
	if this.isMin {
		// Restore from minimized state
		this.isMin = false
		this.minBtn.SetIcon(LoadIcon("minimize"))
		if saved, ok := dockSavedState[this]; ok {
			this.SetBounds(saved.x, saved.y, saved.w, saved.h)
			delete(dockSavedState, this)
		}
		// Show all views
		count := this.tabbar.Count()
		active := this.tabbar.ActiveTab()
		for i := 0; i < count; i++ {
			iw := this.tabbar.Data(i).(IWidget)
			iw.SetVisible(active == i)
		}
	} else {
		// Minimize: collapse to tab bar height
		this.isMin = true
		this.isMax = false
		this.minBtn.SetIcon(LoadIcon("maximize"))
		x, y, w, h := this.Bounds()
		dockSavedState[this] = &dockSavedBounds{x, y, w, h}
		// Hide all views - only tab bar remains visible
		count := this.tabbar.Count()
		for i := 0; i < count; i++ {
			iw := this.tabbar.Data(i).(IWidget)
			iw.SetVisible(false)
		}
	}
	frame := this.Frame()
	if frame != nil {
		frame.Layout()
	}
	this.Self().Update()
}

// toggleMaximize expands the dock panel to fill the frame area, or restores it.
func (this *Dock) toggleMaximize() {
	if this.isMax {
		// Restore from maximized state
		this.isMax = false
		this.maxBtn.SetIcon(LoadIcon("maximize"))
		if saved, ok := dockSavedState[this]; ok {
			this.SetBounds(saved.x, saved.y, saved.w, saved.h)
			delete(dockSavedState, this)
		}
	} else {
		// Maximize: expand to fill parent frame
		this.isMax = true
		this.isMin = false
		this.maxBtn.SetIcon(LoadIcon("minimize"))
		this.minBtn.SetIcon(LoadIcon("minimize"))
		x, y, w, h := this.Bounds()
		dockSavedState[this] = &dockSavedBounds{x, y, w, h}

		frame := this.Frame()
		if frame != nil {
			root := frame.RootBrick()
			if root != nil {
				rx, ry, rw, rh := root.Bounds()
				this.SetBounds(rx, ry, rw, rh)
			}
		}
	}
	this.Layout()
	this.Self().Update()
}

func (this *Dock) CloseAllViews() {
	for i := this.ViewCount() - 1; i >= 0; i-- {
		this.CloseIndex(i)
	}
	// this.Detach()
}

func (this *Dock) CloseDocViews() {
	frame := this.Frame()
	for i := this.ViewCount() - 1; i >= 0; i-- {
		if frame.IsToolView(this.ViewAtIndex(i)) {
			continue
		}
		this.CloseIndex(i)
	}
	//this.DetachIfEmpty()
}

type ISaveLoadSession interface {
	SaveSession() (doc *core.TDoc, err error)
	LoadSession(doc *core.TDoc) error
}

func (this *Dock) SaveTDoc() *core.TDoc {
	doc := core.NewTDoc()
	doc.SetValue("dock")

	doc.WriteAttr("rect", this.Brick.rc)
	doc.WriteAttr("main", this.isMainDock)

	vn := core.NewTDoc()
	for _, view := range this.AllViews() {
		factoryName := core.FactoryNameOf(view)
		if factoryName == "" {
			// 不支持动态创建, 无法保存
			continue
		}

		is, ok := view.(ISaveLoadSession)
		if ok {
			// 有保存会话的接口
			p, _ := is.SaveSession()
			if p != nil {
				p.SetValue(factoryName)
				vn.AddChild(p)
			}
		} else {
			// 无保存会话的接口
			p := core.NewTDoc()
			p.SetValue(factoryName)
			vn.AddChild(p)
		}
	}

	if vn.HasChildren() {
		vn.SetKey("views")
		doc.AddChild(vn)
	}

	return doc
}

func (this *Dock) LoadTDoc(doc *core.TDoc) {

	var rect geom.Rect
	doc.ReadAttr("rect", &rect)
	this.SetBounds1(rect)

	doc.ReadAttr("main", &this.isMainDock)

	vn := doc.ChildByKey("views", false)
	if vn == nil {
		return
	}

	frame := this.Frame()

	for _, p := range vn.Childdren() {
		var viewType string
		p.Value(&viewType)

		var iw IWidget
		_, ok := GetToolViewDef(viewType)
		if ok {
			// 工具视图, 每个框架只有一个同类视图
			// 如果已经存在, 则保留在原处
			info, jc := frame.requireToolView(viewType)
			if !jc {
				continue
			}
			iw = info.Widget
		} else {
			// 普通视图
			iw, ok = core.New(viewType).(IWidget)
			if !ok {
				continue
			}
		}

		this.AddView(iw)
		ii, ok := iw.(ISaveLoadSession)
		if ok {
			ii.LoadSession(p)
		}
	}

}

func (this *Dock) Frame() *Frame {
	if p, ok := this.Parent().(*Frame); ok {
		return p
	}
	return this.Brick.frame
}

func (this *Dock) ExportGv(g *gv.Graph) {
	this.Widget.ExportGv(g)
	//if g.IsAdded(this.ParentBrick()) {
	edge := g.Edge(this.Self(), this.ParentBrick())
	edge.Color = "lightblue"
	edge.TextColor = edge.Color
	edge.Text = "P"
	edge.Weight = 0

	//}
}
