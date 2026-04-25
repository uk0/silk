package gui

import (
	"errors"
	"silk/core"
	"silk/geom"
	"silk/gv"
	"silk/paint"
	"sync"
)

func init() {
	core.RegisterFactory("gui.Frame", core.TypeOf((*Frame)(nil)))
}

// 默认框架
var defaultFrame *Frame

// frameMu protects frameUuidIndex, frameSet, and defaultFrame from concurrent access.
var frameMu sync.Mutex

// 框架的uuid索引
var frameUuidIndex = make(map[core.Uuid]*Frame)

// 存放全部框架
var frameSet = make(map[*Frame]int)

type _ToolViewInfo struct {
	ToolViewDef
	Widget IWidget
	Action IAction

	DockPath []int
	ViewIdx  int
}

// Frame是程序的主框架窗口, 程序可以有一个或多个主框架
type Frame struct {
	Widget
	root_ IBrick

	mainDock   IDock
	activeDock IDock

	title     string
	mainMenu  *Menu
	dropRect  *geom.Rect
	dragSplit IBrick
	uuid      core.Uuid

	cbClosing func(*Frame)
	cbClosed  func(*Frame)

	toolViews map[string]*_ToolViewInfo

	statusBar   *StatusBar
	toolBar     *ToolBar
	leftSidebar IWidget
}

// 创建框架Widget
// 此函数为了和其他Widget保持相同编码风格, 仅创建框架本身
// 如需同时创建框架窗口请用NewFrameWindow
func NewFrame() *Frame {
	frame := new(Frame)
	frame.Init(frame)
	return frame
}

// 创建空的框架窗口, 初始Uuid为零, 窗口为不可见状态
func NewFrameWindow() *Frame {
	frame := NewFrame()
	frame.SetVisible(false)
	frame.AttachWindow(WtForm)
	win := frame.Window()
	win.SetIcon(AppIcon())
	return frame
}

func (this *Frame) Init(iw IWidget) {
	//core.Warn("")
	this.Widget.Init(iw)
	this.toolViews = make(map[string]*_ToolViewInfo)
	frameMu.Lock()
	frameSet[iw.(*Frame)] = 1
	if defaultFrame == nil {
		defaultFrame = iw.(*Frame)
	}
	frameMu.Unlock()
	brick := NewDock()
	brick.isMainDock = true
	this.setRootBrick(brick)

}

// 判断一个已打开的视图是否本框架的工具视图
func (this *Frame) IsToolView(iw IWidget) bool {
	if iw == nil {
		return false
	}
	for _, v := range this.toolViews {
		if iw == v.Widget {
			return true
		}
	}
	return false
}

// 查找已打开的工具视图
func (this *Frame) ToolViewById(id string) IWidget {
	p, ok := this.toolViews[id]
	if ok {
		return p.Widget
	}
	return nil
}

// 获取/创建工具视图
func (this *Frame) requireToolView(id string) (p *_ToolViewInfo, justCreated bool) {
	var ok bool
	p, ok = this.toolViews[id]
	if !ok {
		this.syncToolViewDefs()
	}
	p, ok = this.toolViews[id]
	if !ok {
		return nil, false
	}
	if p.Widget != nil {
		return p, false
	}
	p.Widget, ok = core.New(id).(IWidget)
	if !ok {
		return nil, false
	}

	if ii, ok := p.Widget.(interface {
		SetTitle(string)
	}); ok {
		ii.SetTitle(p.Name)
	}

	if p.Icon != "" {
		if ii, ok := p.Widget.(interface {
			SetIcon(paint.Icon)
		}); ok {
			ii.SetIcon(LoadIcon(p.Icon))
		}
	}

	return p, true
}

// 把全局的视图定义同步到本框架
func (this *Frame) syncToolViewDefs() {
	for _, v := range toolViewReg {
		p, ok := this.toolViews[v.Id]
		if ok {
			p.Action.SetChecked(p.Widget != nil)
			continue
		}
		p = new(_ToolViewInfo)
		p.ToolViewDef = v

		p.Action = NewAction()
		p.Action.SetObjName(p.Id)
		p.Action.SetText(p.Name)
		if p.Icon != "" {
			p.Action.SetIcon(LoadIcon(p.Icon))
		}
		p.Action.SetExtra(p.Id)

		p.Action.BindFunc1(this.onToolViewAction)
		p.Action.SetChecked(p.Widget != nil)

		this.toolViews[v.Id] = p
	}
}

func (this *Frame) onToolViewAction(a IAction) {
	id := a.Extra().(string)
	if this.IsToolViewVisible(id) {
		this.HideToolView(id)
	} else {
		this.ShowToolView(id)
	}
}

func (this *Frame) IsToolViewVisible(id string) bool {
	return this.ToolViewById(id) != nil
}

func (this *Frame) ShowToolView(id string) bool {
	if this.root_ == nil {
		panic("this.root_ == nil")
	}
	this.syncToolViewDefs()

	p, create := this.requireToolView(id)
	if create {
		dock := FallowDockPath(this.RootBrick(), p.DockPath)
		dock.InsertView(p.ViewIdx, p.Widget)
		//this.SuggestToolDock().AddView(w)
	}

	return true
}

func (this *Frame) CloseToolView(id string) bool {
	p, ok := this.toolViews[id]
	if !ok {
		return false
	}
	if p.Widget == nil {
		return true
	}

	if this.PromptSaveCloseView(p.Widget) {
		p.Widget = nil
		return true
	}
	return false
}

// 此函数正确找到Dock, 然后通过Dock关闭视图
func (this *Frame) PromptSaveCloseView(view IWidget) bool {
	if view == nil {
		return false
	}

	dock := FindOwnerDock(view)
	if dock != nil {
		return dock.PromptSaveCloseView(view)
	} else {
		return PromptSaveClose(this, view)
	}
}

func (this *Frame) HideToolView(id string) {
	this.CloseToolView(id)
}

func (this *Frame) Close() {
	if this.cbClosing != nil {
		this.cbClosing(this)
	}
	this.CloseAllViews()
	this.SetUuid(core.Uuid{})

	frameMu.Lock()
	delete(frameSet, this.Self().(*Frame))

	if defaultFrame == this.Self().(*Frame) {
		defaultFrame = nil
		for p := range frameSet {
			defaultFrame = p
			break
		}
	}
	frameMu.Unlock()

	if this.cbClosed != nil {
		this.cbClosed(this)
	}
}

func (this *Frame) CloseAllViews() {
	for _, dock := range this.AllDocks() {
		dock.Close()
	}
}

func (this *Frame) CloseDocViews() {
	for _, dock := range this.AllDocks() {
		dock.CloseDocViews()
	}
}

func (this *Frame) DirtyList() (list []string) {
	for _, view := range this.AllViews() {
		if idirty, ok := view.(interface {
			DirtyList() []string
		}); ok {
			list = append(list, idirty.DirtyList()...)
		}
	}
	return
}

func (this *Frame) Save() bool {
	core.Debug("Frame.Save()")
	for _, view := range this.AllViews() {
		if isave, ok := view.(interface {
			Save() bool
		}); ok {
			if !isave.Save() {
				return false
			}
		}
	}
	return true
}

func (this *Frame) Title() string {
	return this.title
}

func (this *Frame) SetTitle(s string) {
	this.title = s
}

func (this *Frame) RootBrick() IBrick {
	return this.root_
}

// 查找用于停靠文档视图的停靠区
func (this *Frame) SuggestDocDock() IDock {
	p := findMainDock(this.root_)
	if p == nil {
		p = findLargestToolDock(this.root_)
	}
	return p
}

// 查找用于停靠工具视图的停靠区
func (this *Frame) SuggestToolDock() IDock {
	p := findLargestToolDock(this.root_)
	if p == nil {
		p = findMainDock(this.root_)
	}
	return p
}

func findLargestToolDock(brick IBrick) (ret IDock) {
	if dock, ok := brick.(IDock); ok {
		if dock.IsMainDock() {
			return nil
		}
		return dock
	}

	a := findLargestToolDock(brick.Left())
	b := findLargestToolDock(brick.Right())

	if a == nil && b == nil {
		return nil
	}

	if a != nil && b != nil {
		aa := a.Bounds1().Area()
		ba := b.Bounds1().Area()
		if aa < ba {
			return b
		} else {
			return a
		}
	}

	if b != nil {
		return b
	} else {
		return a
	}
}

// 查找主停靠区
// 主停靠区在没有视图停靠时也不自动关闭, 通常用来停靠文档视图
// 正常情况下框架有0至1个主停靠区
func (this *Frame) MainDock() IDock {
	return findMainDock(this.root_)
}

func findMainDock(brick IBrick) (ret IDock) {
	if dock, ok := brick.(IDock); ok && dock.IsMainDock() {
		ret = dock
	}

	if ret == nil && brick.Left() != nil {
		ret = findMainDock(brick.Left())
	}

	if ret == nil && brick.Right() != nil {
		ret = findMainDock(brick.Right())
	}
	return
}

func (this *Frame) setRootBrick(brick IBrick) {
	this.root_ = brick
	this.mainDock = nil
	if brick != nil {
		brick.setParentBrick(nil)
		brick.setFrame(this)
	}
}

//func (this *Frame) MainWidget() IWidget {
//	return this.mainWidget
//}

//func (this *Frame) SetMainWidget(w IWidget) {
//	this.mainWidget = w
//	w.SetParent(this)
//	this.Ow().Update()
//}

//func (this *Frame) IsVisible() bool {
//	return this.Widget.IsVisible()
//}

func (this *Frame) Layout() {
	//core.Debug("(this *Frame) Layout()")
	hints := this.MainMenu().SizeHints()
	this.mainMenu.SetBounds(0, 0, this.Width(), hints.Height)

	contentTop := hints.Height

	// Reserve space for toolbar below the menu bar
	if this.toolBar != nil && this.toolBar.IsVisible() {
		tbHints := this.toolBar.SizeHints()
		tbH := tbHints.Height
		if tbH < 32 {
			tbH = 32
		}
		this.toolBar.SetBounds(0, contentTop, this.Width(), tbH)
		contentTop += tbH
	}

	contentBottom := this.Height()

	// Reserve space for status bar at the bottom
	if this.statusBar != nil && this.statusBar.IsVisible() {
		sbHints := this.statusBar.SizeHints()
		sbH := sbHints.Height
		contentBottom -= sbH
		this.statusBar.SetBounds(0, contentBottom, this.Width(), sbH)
	}

	// Reserve space for left sidebar
	contentLeft := 0.0
	if this.leftSidebar != nil && this.leftSidebar.IsVisible() {
		lsHints := this.leftSidebar.SizeHints()
		lsW := lsHints.Width
		if lsW < 60 {
			lsW = 60
		}
		this.leftSidebar.SetBounds(0, contentTop, lsW, contentBottom-contentTop)
		contentLeft = lsW
	}

	if this.root_ != nil {
		this.root_.SetBounds(contentLeft, contentTop, this.Width()-contentLeft, contentBottom-contentTop)
	}
	this.Self().Update()
}

// SetStatusBar sets the status bar at the bottom of the frame.
func (this *Frame) SetStatusBar(sb *StatusBar) {
	this.statusBar = sb
	if sb != nil {
		sb.SetParent(this)
	}
	this.Layout()
}

// StatusBar returns the frame's status bar, or nil if none.
func (this *Frame) StatusBar() *StatusBar {
	return this.statusBar
}

// SetToolBar sets an optional toolbar below the menu bar.
func (this *Frame) SetToolBar(tb *ToolBar) {
	this.toolBar = tb
	if tb != nil {
		tb.SetParent(this)
	}
	this.Layout()
}

// ToolBar returns the frame's toolbar, or nil if none.
func (this *Frame) ToolBar() *ToolBar {
	return this.toolBar
}

// SetLeftSidebar sets a fixed-width widget on the left edge of the frame,
// between the toolbar and the dock area. Used for mode selectors.
func (this *Frame) SetLeftSidebar(w IWidget) {
	this.leftSidebar = w
	if w != nil {
		w.SetParent(this)
	}
	this.Layout()
}

// LeftSidebar returns the frame's left sidebar widget, or nil if none.
func (this *Frame) LeftSidebar() IWidget {
	return this.leftSidebar
}

func (this *Frame) MainMenu() *Menu {
	if this.mainMenu == nil {
		this.mainMenu = NewMenu(false)
		this.mainMenu.SetParent(this)
		this.mainMenu.SetVisible(true)
	}
	return this.mainMenu
}

func (this *Frame) Draw(g paint.Painter) {
	g.Rectangle(0, 0, this.w, this.h)
	if this.dragSplit == nil {
		// Use a subtle gray for the splitter gap area (matches theme)
		g.SetBrush1(paint.Color{200, 200, 200, 255})

	} else {
		// Slightly darker when actively dragging a splitter
		g.SetBrush1(paint.Color{180, 180, 190, 255})

	}
	g.Fill()
}

func (this *Frame) DrawOverlay(g paint.Painter) {
	if this.dropRect != nil {
		g.Rectangle(this.dropRect.X+4, this.dropRect.Y+4, this.dropRect.Width-8, this.dropRect.Height-8)
		g.SetBrush1(paint.Color{255, 160, 0, 64})
		g.Fill()
	}

}

func (this *Frame) extractDndHandle(dnd IDndContext) *dockDndHandle {
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

func (this *Frame) isInRootBrick(x, y float64) bool {
	rx, ry, rw, rh := this.RootBrick().Bounds()
	//core.Debug(rx, ry, rw, rh)
	//core.Debug(x, y)
	return x >= rx && y >= ry && x < rx+rw && y < ry+rh
}

func (this *Frame) OnDragEnter(x, y float64, dnd IDndContext) {
	this.dropRect = nil
	if !this.isInRootBrick(x, y) {
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

func (this *Frame) OnDragLeave() {
	if this.dropRect != nil {
		this.dropRect = nil
		this.Self().Update()
	}

}

func (this *Frame) setDropRect(rc *geom.Rect) {
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

func (this *Frame) OnDragMove(x, y float64, dnd IDndContext) {
	if !this.isInRootBrick(x, y) {
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
	rootDock, _ := this.RootBrick().(IDock)
	if handle.dock == rootDock &&
		!rootDock.IsMainDock() &&
		rootDock.ViewCount() == 1 {
		dnd.SetAction(DndIgnore)
		this.setDropRect(nil)
		return
	}
	srcFrame := handle.dock.Frame()
	if srcFrame.IsToolView(handle.view) && srcFrame != this {
		// 工具视图不允许跨Frame拖放
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		return
	}

	dnd.SetAction(DndMove)
	x1 := x
	y1 := y
	//	core.Debug(x1, y1)
	split, vert, left, merge := this.RootBrick().DropSplitHint(x1, y1)
	rc := new(geom.Rect)
	rc.X, rc.Y, rc.Width, rc.Height = this.RootBrick().DropRect(split, vert, left, merge)
	this.setDropRect(rc)

}

func (this *Frame) OnDrop(x, y float64, dnd IDndContext) {
	this.Self().Update()
	if this.dropRect != nil {
		this.dropRect = nil
	}
	if !this.isInRootBrick(x, y) {
		dnd.SetAction(0)
		return
	}
	handle := this.extractDndHandle(dnd)
	if handle == nil {
		dnd.SetAction(DndIgnore)
		return
	}

	rootDock, _ := this.RootBrick().(IDock)
	if handle.dock == rootDock && // 只有一个DOCK
		!rootDock.IsMainDock() && // 且不是MainDock
		rootDock.ViewCount() == 1 { // 且是最后一个视图
		dnd.SetAction(DndIgnore) // 不用操作, 因为拖了以后和拖之前没区别, 仍是只有一个视图
		this.dropRect = nil
		return
	}

	srcFrame := handle.dock.Frame()
	if srcFrame.IsToolView(handle.view) && srcFrame != this {
		// 工具视图不允许跨Frame拖放
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		return
	}

	dnd.SetAction(DndMove)
	x1 := x
	y1 := y
	//this.doDrop(handle, x1, y1)
	split, vert, left, merge := this.RootBrick().DropSplitHint(x1, y1)
	iw := handle.dock.RemoveIndex(handle.index)
	if merge {
		dnd.SetAction(DndIgnore)
		this.dropRect = nil
		return
	} else {
		dock := NewDock()
		dock.SetParent(this)
		dock.AddView(iw)
		//DbgExportGuiGv(true)
		this.RootBrick().Split(dock, split, vert, left)
	}
	//DbgExportGuiGv(true)
	//handle.dock.DetachIfEmpty()
	this.Layout()
}

func (this *Frame) OnLeftDown(x, y float64) {
	this.dragSplit = this.RootBrick().FindSplitter(x, y)
	this.Self().Update()
}

func (this *Frame) OnLeftUp(x, y float64) {
	this.dragSplit = nil
	this.Self().Update()
}

func (this *Frame) OnMouseMove(x, y float64) {
	if this.dragSplit != nil {
		this.dragSplit.SetSplitPoint(x, y)
		this.dragSplit.Layout()
		this.Self().Update()
	}
}

func (this *Frame) OnIdle() {
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

func (this *Frame) AllViews() (ret []IWidget) {
	for _, dock := range this.AllDocks() {
		ret = append(ret, dock.AllViews()...)
	}
	return
}

func (this *Frame) AllDocks() (ret []IDock) {
	if this.root_ == nil {
		return
	}
	stack := []IBrick{this.root_}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if dock, ok := p.(IDock); ok {
			ret = append(ret, dock)
		}
		if p.Left() != nil {
			stack = append(stack, p.Left())
		}
		if p.Right() != nil {
			stack = append(stack, p.Right())
		}
	}
	return
}

func (this *Frame) Uuid() core.Uuid {
	return this.uuid
}

func (this *Frame) SetUuidStr(s string) error {
	uuid, err := core.ParseUuid(s)
	if err != nil {
		return err
	}

	return this.SetUuid(uuid)
}

func (this *Frame) SetUuid(a core.Uuid) error {
	if this.uuid == a {
		return nil
	}

	frameMu.Lock()
	defer frameMu.Unlock()

	_, conflict := frameUuidIndex[a]
	if conflict {
		core.Warn("conflict frame uuid " + a.String())
		return errors.New("conflict frame uuid " + a.String())
	}

	if !this.uuid.IsZero() {
		delete(frameUuidIndex, this.uuid)
	}

	this.uuid = a

	if !this.uuid.IsZero() {
		frameUuidIndex[this.uuid] = this.Self().(*Frame)
	}
	return nil
}

func (this *Frame) SetClosedCallback(fn func(*Frame)) {
	this.cbClosed = fn
}

func (this *Frame) SetClosingCallback(fn func(*Frame)) {
	this.cbClosing = fn
}

//func (this *Frame) IsToolViewOpen(typ string) bool {
//}

//func (this *Frame) GetToolView(typ string, create bool) IWidget {
//}

func AllFrames() (list []*Frame) {
	frameMu.Lock()
	defer frameMu.Unlock()
	for p := range frameSet {
		list = append(list, p)
	}
	return
}

func FindFrameByUuid(uuid core.Uuid) *Frame {
	if uuid.IsZero() {
		return nil
	}

	frameMu.Lock()
	defer frameMu.Unlock()
	p, _ := frameUuidIndex[uuid]
	return p
}

// 指定默认框架
// 打开文档, 显示视图时, 如果没有指定框架, 则放到默认框架中
func SetDefaultFrame(p *Frame) {
	frameMu.Lock()
	defer frameMu.Unlock()
	defaultFrame = p
}

// 获取默认框架, 如果未制定则返回最先创建的框架
// 打开文档, 显示视图时, 如果没有指定框架, 则应放到此框架中
// 如果默认框架已经关闭, 则系统将任意选一个框架作为替补
// 建议应用层把主框架设为默认框架
// 建议应用层在关闭默认框架时退出程序
func DefaultFrame() *Frame {
	frameMu.Lock()
	defer frameMu.Unlock()
	return defaultFrame
}

// 把框架布局加载到当前框架
func (this *Frame) LoadSession(doc *core.TDoc) error {
	if this.win == nil {
		this.AttachWindow(WtForm)
	}
	var uuid core.Uuid
	if err := doc.ReadAttr("uuid", &uuid); err != nil {
		return err
	}
	this.SetUuid(uuid)

	p := doc.ChildByKey("place", false)
	if p != nil {
		var wp WindowPlace
		if err := p.Unmarshal(&wp); err != nil {
			return err
		}
		this.Window().SetPlacement(wp)
	}
	var title string
	if err := doc.ReadAttr("title", &title); err == nil {
		this.Window().SetTitle(title)
	}

	this.CloseAllViews()

	for _, dock := range this.AllDocks() {
		//core.Debug("detach ", i)
		dock.Detach()
	}

	p = doc.ChildByKey("tree", false)
	if p != nil {

		var tpy string
		p.Value(&tpy)

		if tpy == "brick" {
			root := newBrick()
			this.setRootBrick(root)
			root.LoadTDoc(p)
		} else {
			root := NewDock()
			root.isMainDock = true
			this.setRootBrick(root)
			root.LoadTDoc(p)
		}
		this.root_.Layout()
	} else {
		brick := NewDock()
		brick.isMainDock = true
		this.setRootBrick(brick)
	}

	for _, dock := range this.AllDocks() {
		dock.DetachIfEmpty()
	}
	return nil
}

// 保存当前框架布局
// 注: uuid为零时也保存
func (this *Frame) SaveSession() (doc *core.TDoc, err error) {
	win := this.Window()
	if win == nil {
		err = errors.New("frame is not attached to window")
		return nil, err
	}
	doc = core.NewTDoc()

	doc.WriteAttr("uuid", this.Uuid())

	wp := this.Window().Placement()
	p, _ := core.TDocMarshal(wp)
	p.SetKey("place")
	doc.AddChild(p)

	doc.WriteAttr("title", this.Title())

	if this.root_ != nil {
		root := this.root_.SaveTDoc()
		if root != nil {
			root.SetKey("tree")
			doc.AddChild(root)
		}
	}

	return
}

func (this *Frame) ToolViewActions() (list []IAction) {
	this.syncToolViewDefs()
	for _, p := range this.toolViews {
		list = append(list, p.Action)
	}
	SortActions(list)
	return
}

func (this *Frame) SetActiveDock(dock IDock) {
	if dock == nil || this.activeDock == dock {
		return
	}

	if dock.Frame() != this {
		return
	}

	if this.activeDock != nil {
		this.activeDock.Update()
	}
	if dock != nil {
		dock.Update()
	}
	this.activeDock = dock
}

func (this *Frame) ActiveDock() IDock {
	if this.activeDock == nil {
		this.SetActiveDock(this.SuggestDocDock())
	}
	return this.activeDock
}

func (this *Frame) ActiveView() (IWidget, IDock) {
	dock := this.ActiveDock()
	w := dock.ActiveView()
	return w, dock
}

func (this *Frame) CurrentDocView() (IWidget, IDock) {

	view, dock := this.ActiveView()
	if view != nil && !this.IsToolView(view) {
		return view, dock
	}

	dock = this.MainDock()
	if dock != nil {
		view = dock.ActiveView()
		if view != nil && !this.IsToolView(view) {
			return view, dock
		}
	}

	for _, dock := range this.AllDocks() {
		if dock != nil {
			view = dock.ActiveView()
			if view != nil && !this.IsToolView(view) {
				return view, dock
			}
		}
	}
	return nil, nil
}

func (this *Frame) ExportGv(g *gv.Graph) {
	this.Widget.ExportGv(g)
	self := this.Self()
	node := g.Node(self)
	node.Text += "\n" + this.uuid.String()

	edge := g.Edge(node, this.RootBrick())
	edge.Color = "lightgreen"
	edge.TextColor = edge.Color
	edge.Text = "RootBrick"
	edge.Weight = 10

	for _, dock := range this.AllDocks() {
		if dock != nil && g.IsEdgeAdded(self, dock) {
			edge := g.Edge(self, dock)
			edge.Weight = 0
			edge.Color = "pink"
		}
		if g.IsNodeAdded(dock) {
			g.Node(dock).Color = "pink"
		}
	}
}
