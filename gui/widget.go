package gui

import (
	"silk/core"
	"silk/geom"
	"silk/gv"
	"silk/paint"
	"fmt"
	//"reflect"
	//	"runtime"
	//"sync"
	//"unsafe"
)

type IWidget interface {

	// (控件)对象本身
	Self() IWidget

	// 裸的(内层)控件指针
	NakedWidget() *Widget

	// 和本Widget配对的窗口, 可能为空
	Window() *Window

	// 附加到窗口上
	AttachWindow(wt WindowType)

	// 附加到窗口上, 但延迟到第一次显示时才创建窗口
	// 因为创建窗口通常较慢, 所以此方法可能有助于提高创建效率
	LazyAttachWindow(wt WindowType)

	// 和窗口分离
	DetachWindow()

	// 是否已经附加到一个WtPopup窗口上
	// 注: 即使是延迟附加, 这个方法也会返回true
	IsPopup() bool

	// 本Widget所在的窗口,
	// 此窗口可能是直接和本Widget绑定, 也可能是和某祖先Widget绑定
	// 因为Widget只能显示在窗口里, 所以当Widget可见时, 此函数返回值一定是有效的窗口
	OwnerWindow() *Window

	// 父控件
	Parent() IWidget

	// 设置父控件
	SetParent(parent IWidget)

	// 根控件
	RootWidget() IWidget

	// 绘图接口
	Draw(g paint.Painter)

	// 获取X坐标
	X() float64
	// 获取Y坐标
	Y() float64
	// 获取宽度
	Width() float64
	// 获取高度
	Height() float64

	Pos() (x, y float64)
	SetPos(x, y float64)
	Size() (width, height float64)
	SetSize(width, height float64)

	Bounds() (x, y, width, height float64)
	SetBounds(x, y, width, height float64)
	Bounds1() (rect geom.Rect)
	SetBounds1(rect geom.Rect)

	UpdateRect(x, y, width, height float64)
	Update()

	// 鼠标是否在控件内部
	IsHover() bool

	MapToWindow(x, y float64) (x1, y1 float64)
	MapFromWindow(x, y float64) (x1, y1 float64)
	MapToGlobal(x, y float64) (x1, y1 float64)
	MapFromGlobal(x, y float64) (x1, y1 float64)

	FindWidgetAt(x, y float64) IWidget

	SetVisible(bool)
	IsVisible() bool
	IsAllAncentorsVisible() bool

	SetEnabled(bool)
	IsEnabled() bool

	// 显示控件, 相当于SetVisible(true)
	Show()

	// 隐藏控件, 相当于SetVisible(false)
	Hide()

	HasFocus() bool
	SetFocus()

	SetRedrawParent(b bool)
	IsRedrawParent() bool

	Cursor() *Cursor

	SizeHints() SizeHints

	PushCapture()
	PopCapture()
	HasCapture() bool

	Detach()

	Children() []IWidget

	DoDragDrop(content paint.Pixmap, availableActions DndAction, data ...interface{}) DndAction

	ExtraData() interface{}
	SetExtraData(a interface{})
}

type Widget struct {
	//	core.Obj
	self       IWidget
	x, y, w, h float64

	hidden       bool
	disabled     bool
	redrawParent bool

	extraData interface{}

	wt  WindowType
	win *Window

	parent IWidget
	next   *Widget
	prev   *Widget
	child  *Widget

	dt *string

	//objname string
}

func (this *Widget) Init(o IWidget) {
	if this.self != nil {
		panic("widget allready initialized")
	}

	if core.IsDebugOn() {
		this.dt = core.LiveCycleTrace(o)
	}

	this.self = o
}

func (this *Widget) Self() IWidget {
	o := this.self
	if o == nil {
		core.Warn(`invalid self value <nil>, forgot Widget.Init(self)?`)
	}
	return o
}

func (this *Widget) NakedWidget() *Widget {
	return this
}

func (this *Widget) Parent() IWidget {
	if this.parent != nil && this.parent != this.parent.Self() {
		core.Warn("this.parent != this.parent.Self()")
	}
	return this.parent
}

func (this *Widget) Detach() {
	this.SetParent(nil)
}

func (this *Widget) SetParent(parent IWidget) {
	np := parent
	if np != nil {
		np = np.Self()
	}

	op := this.parent
	if op == np {
		return
	}

	if op != nil {
		bop := op.NakedWidget()
		if this.next == this {
			bop.child = nil
		} else {
			if bop.child == this {
				bop.child = this.next
			}
			this.next.prev = this.prev
			this.prev.next = this.next
		}
	}

	if np == nil {
		this.next = nil
		this.prev = nil
		this.parent = nil
	} else {
		bnp := np.NakedWidget()
		if bnp.child == nil {
			bnp.child = this
			this.next = this
			this.prev = this
		} else {
			this.next = bnp.child
			this.prev = bnp.child.prev
			this.prev.next = this
			this.next.prev = this
		}
		this.parent = np
	}
	this.syncWindow(this.IsVisible(), true)
}

func (this *Widget) X() float64 {
	return this.x
}

func (this *Widget) Y() float64 {
	return this.y
}

func (this *Widget) Width() float64 {
	return this.w
}

func (this *Widget) Height() float64 {
	return this.h
}

func (this *Widget) SetX(x float64) {
	this.SetPos(x, this.y)
}

func (this *Widget) SetY(y float64) {
	this.SetPos(this.x, y)
}

func (this *Widget) SetWidth(w float64) {
	this.SetSize(w, this.h)
}

func (this *Widget) SetHeight(h float64) {
	this.SetSize(this.w, h)
}

func (this *Widget) Pos() (x, y float64) {
	return this.x, this.y
}

func (this *Widget) setPos(x, y float64) {
	if this.x != x || this.y != y {
		this.x, this.y = x, y
		this.Self().(IWidgetEvent).OnMove()
	}
}

func (this *Widget) SetPos(x, y float64) {
	if this.win != nil {
		// 已绑定窗口, 消息的流程如下:
		// 1  widget.SetPos()
		// 2  window.SetPos()
		// 3  system.MoveWindow()
		// 4  system.WM_MOVE
		// 5  window.On_WM_MOVE
		// 6  widget.setPos()
		// 7  widget.OnMove()
		this.win.SetPos(x, y)
		return
	}
	// 未绑定窗口, 消息的流程如下:
	// 1  widget.SetPos()
	// 2  widget.setPos()
	// 3  widget.OnMove()
	this.setPos(x, y)
}

func (this *Widget) Size() (width, height float64) {
	return this.w, this.h
}

func (this *Widget) setSize(width, height float64) {
	if this.w != width || this.h != height {
		this.w, this.h = width, height
		this.Self().(IWidgetEvent).OnResize()
	}
}

func (this *Widget) SetSize(width, height float64) {
	if this.win != nil {
		// 已绑定窗口, 消息的流程如下:
		// 1  widget.SetSize()
		// 2  window.SetSize()
		// 3  system.ResizeWindow()
		// 4  system.WM_SIZE
		// 5  window.On_WM_SIZE
		// 6  widget.setSize()
		// 7  widget.OnResize()
		this.win.SetSize(width, height)
		return
	}
	// 未绑定窗口, 消息的流程如下:
	// 1  widget.SetSize()
	// 2  widget.setSize()
	// 3  widget.OnResize()
	this.setSize(width, height)
}

func (this *Widget) Bounds() (x, y, width, height float64) {
	return this.x, this.y, this.w, this.h
}

func (this *Widget) SetBounds(x, y, width, height float64) {
	this.SetPos(x, y)
	this.SetSize(width, height)
}

func (this *Widget) Bounds1() (rect geom.Rect) {
	return geom.Rect{this.x, this.y, this.w, this.h}
}

func (this *Widget) SetBounds1(rect geom.Rect) {
	this.SetBounds(rect.X, rect.Y, rect.Width, rect.Height)
}

//func (this *Widget) Place() interface{} {
//	return this.place
//}

//func (this *Widget) SetPlace(p interface{}) {
//	this.place = p
//}

func (this *Widget) Window() *Window {
	//if this.win == nil && this.wt != WtInherit {
	//	this.syncWindow(!this.hidden, true)
	//}
	return this.win
}

func (this *Widget) OwnerWindow() *Window {
	for p := this.Self(); p != nil; p = p.Parent() {
		bw := p.Window()
		if bw != nil {
			return bw
		}
	}
	return nil
}

func (this *Widget) WindowWidget() IWidget {
	for p := this.Self(); p != nil; p = p.Parent() {
		bw := p.Window()
		if bw != nil {
			return p
		}
	}
	return nil
}

func drawErrCross(g paint.Painter, x, y, w, h float64) {
	g.Save()
	g.Rectangle(x+1, y+1, w-2, h-2)
	g.SetBrush1(paint.Color{240, 255, 240, 240})
	g.FillPreserve()
	g.MoveTo(x, y)
	g.LineTo(x+w, y+h)
	g.MoveTo(x+w, y)
	g.LineTo(x, y+h)
	pen := paint.NewPen(paint.Color{255, 0, 0, 240}, 3)
	g.SetPen(pen)
	g.Stroke()
	g.Restore()
}

func (this *Widget) Draw(g paint.Painter) {
	//
	if core.IsDebugOn() {
		drawErrCross(g, 0, 0, this.w, this.h)
	}
}

func (this *Widget) Update() {
	// TODO: 优化
	x := 0.0
	y := 0.0
	for p := this.Self(); p != nil; p = p.Parent() {
		win := p.Window()
		if win != nil {
			win.UpdateRect(x, y, this.w, this.h)
			break
		}
		x1, y1, _, _ := p.Bounds()
		x += x1
		y += y1
	}

	if this.redrawParent && this.Parent() != nil {
		this.Parent().Update()
		//core.Debug(`this.Parent().Update()`)
	}
}

func (this *Widget) UpdateRect(x, y, width, height float64) {
	for p := this.Self(); p != nil; p = p.Parent() {
		win := p.Window()
		if win != nil {
			win.UpdateRect(x, y, this.w, this.h)
			return
		}
		x1, y1, _, _ := p.Bounds()
		x += x1
		y += y1
	}
}

func (this *Widget) SizeHints() SizeHints {
	return SizeHints{Width: 32, Height: 32}
}

func (this *Widget) OnResize() {
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

// InvalidateLayout requests a layout recalculation on this widget.
// If this widget implements ILayout, its Layout() is called immediately,
// then Update() is called to trigger a repaint.
func (this *Widget) InvalidateLayout() {
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

// InvalidateParentLayout requests the parent container to recalculate layout.
// This should be called when a child's size hints change.
func (this *Widget) InvalidateParentLayout() {
	p := this.Self().Parent()
	if p != nil {
		if i, ok := p.(ILayout); ok {
			i.Layout()
		}
		p.Update()
	}
}

func (this *Widget) OnMove() {

}

func (this *Widget) IsHover() bool {
	return this.Self() == mouseHoverWidget
}

func findInParentCoord(this IWidget, xp, yp, cx, cy, cw, ch float64) IWidget {
	wx, wy, ww, wh := this.Bounds()
	cx1, cy1, cw1, ch1 := intersectRect(cx, cy, cw, ch, wx, wy, ww, wh)
	if xp >= cx1 && yp >= cy1 && xp < cx1+cw1 && yp < cy1+ch1 {
		head := this.NakedWidget().child
		if head != nil {
			for c := head.prev; ; c = c.prev {
				ic := c.Self()
				if ic.IsVisible() && ic.Window() == nil {
					p := findInParentCoord(ic, xp-wx, yp-wy, cx1-wx, cy1-wy, cw1, ch1)
					if p != nil {
						return p
					}
				}
				if c == head {
					break
				}
			}
		}

		return this.Self()
	}
	return nil
}

func (this *Widget) FindWidgetAt(x, y float64) IWidget {
	ww, wh := this.Size()
	if x >= 0 && y >= 0 && x < ww && y < wh {
		head := this.NakedWidget().child
		if head != nil {
			for c := head.prev; ; c = c.prev {
				ic := c.Self()
				if ic.IsVisible() && ic.Window() == nil {
					p := findInParentCoord(ic, x, y, 0, 0, ww, wh)
					if p != nil {
						return p
					}
				}
				if c == head {
					break
				}
			}
		}

		return this.Self()
	}
	return nil
}

func (this *Widget) MapToWindow(x, y float64) (x1, y1 float64) {
	x1, y1 = x, y
	for p := this.Self(); p.Parent() != nil && p.Window() == nil; p = p.Parent() {
		px, py := p.Pos()
		x1 += px
		y1 += py
	}
	return
}

func (this *Widget) MapFromWindow(x, y float64) (x1, y1 float64) {
	x1, y1 = x, y
	for p := this.Self(); p.Parent() != nil && p.Window() == nil; p = p.Parent() {
		px, py := p.Pos()
		x1 -= px
		y1 -= py
	}
	return
}

func (this *Widget) MapToGlobal(x, y float64) (x1, y1 float64) {
	x1, y1 = x, y
	p := this.Self()
	for p.Parent() != nil && p.Window() == nil {
		px, py := p.Pos()
		x1 += px
		y1 += py
		p = p.Parent()
	}
	window := p.Window()
	if window != nil {
		x1, y1 = window.mapToGlobal(x1, y1)
	}
	return
}

func (this *Widget) MapFromGlobal(x, y float64) (x1, y1 float64) {
	x1, y1 = x, y
	p := this.Self()
	for p.Parent() != nil && p.Window() == nil {
		px, py := p.Pos()
		x1 -= px
		y1 -= py
		p = p.Parent()
	}
	window := p.Window()
	if window != nil {
		x1, y1 = window.mapFromGlobal(x1, y1)
	}
	return
}

func (this *Widget) RootWidget() IWidget {
	p := this.Self()
	for p.Parent() != nil {
		p = p.Parent()
	}
	return p
}

func (this *Widget) SetRedrawParent(b bool) {
	this.redrawParent = b
}

func (this *Widget) IsRedrawParent() bool {
	return this.redrawParent
}

func (this *Widget) HasFocus() bool {
	return focusWidget == this.Self()
}

func (this *Widget) SetFocus() {
	iw := this.Self()
	if focusWidget == iw {
		return
	}
	//core.Debug("SetFocus()")
	oldFocusWidget := focusWidget
	focusWidget = iw
	if ie, ok := oldFocusWidget.(IEventFocusChanged); ok {
		ie.OnFocusChanged(focusWidget, oldFocusWidget)
	}
	if ie, ok := focusWidget.(IEventFocusChanged); ok {
		ie.OnFocusChanged(focusWidget, oldFocusWidget)
	}

	dock := FindOwnerDock(this.Self())
	if dock != nil {
		dock.Frame().SetActiveDock(dock)
	}
}

func (this *Widget) OnFocusChanged(newFocusWidget, oldFocusWidget IWidget) {
	iw := this.Self()
	iw.Update()
	//core.Debug(`OnFocusChanged() "`, newFocusWidget, `" "`, oldFocusWidget, `"`)
	//core.Debug(`OnFocusChanged() "`, iw, `"`)
}

//func (this *Widget) IsRealVisible() bool {
//	for p := this.Self(); p != nil; p = p.Parent() {
//		if !p.IsVisible() {
//			return false
//		}
//		if sw := p.Window(); sw != nil {
//			return sw.IsVisible()
//		}
//	}
//	return true
//}

func (this *Widget) CheckAllAncestors(checkFunc func(IWidget) bool) bool {
	for w := this.Parent(); w != nil; w = w.Parent() {
		if !checkFunc(w) {
			return false
		}
		if w.Window() == nil {
			break
		}
	}
	return true
}

func (this *Widget) CheckAnyAncestor(checkFunc func(IWidget) bool) bool {
	for w := this.Parent(); w != nil; w = w.Parent() {
		if checkFunc(w) {
			return true
		}
		if w.Window() == nil {
			break
		}
	}
	return false
}

func (this *Widget) IsEnabled() bool {
	return !this.disabled
}

func (this *Widget) SetEnabled(b bool) {
	if this.disabled == !b {
		return
	}
	this.disabled = !b
	this.Update()
}

func (this *Widget) IsVisible() bool {
	return !this.hidden
}

func (this *Widget) SetVisible(b bool) {
	this.syncWindow(b, !b)
	this.setVisible(b)
}

func (this *Widget) setVisible(b bool) {
	if this.hidden == !b {
		return
	}
	//core.Debug("(this *Widget) setVisible(b bool)")
	this.hidden = !b

	if this.hidden {
		if i, ok := this.Self().(IEventHide); ok {
			i.OnHide()
		}
		//core.Debug("enter pop capture")
		this.PopCapture()
		//core.Debug("leave pop capture")
	} else {
		if i, ok := this.Self().(IEventShow); ok {
			i.OnShow()
		}
	}
}

func (this *Widget) Show() {
	this.SetVisible(true)
}

func (this *Widget) Hide() {
	this.SetVisible(false)
}

//func (this *Widget) OnShow() {
//}

//func (this *Widget) OnHide() {
//}

func (this *Widget) syncWindow(visible, lazy bool) {
	if this.win != nil && this.wt != this.win.wt {
		w := this.win
		this.win = nil
		w.destroy()
	}

	if this.win == nil && this.wt != WtInherit {
		if lazy && !visible {
			return
		}
		if this.wt == WtChild && this.OwnerWindow() == nil {
			panic("")
		}

		this.win = new(Window)
		this.win.widget = this.Self()
		err := this.win.create(this.OwnerWindow(), this.wt)
		if err != nil {
			this.win = nil
		}
	}

	if this.win != nil {
		this.win.setVisible(visible)
	}
}

func (this *Widget) DetachWindow() {
	this.wt = WtInherit
	this.syncWindow(this.IsVisible(), false)
}

func (this *Widget) LazyAttachWindow(wt WindowType) {
	this.wt = wt
	this.syncWindow(this.IsVisible(), true)
}

func (this *Widget) AttachWindow(wt WindowType) {
	this.wt = wt
	this.syncWindow(this.IsVisible(), false)
}

func (this *Widget) IsPopup() bool {
	return this.wt == WtPopup
}

func (this *Widget) Cursor() *Cursor {
	return cursorArrow
}

func (this *Widget) Children() []IWidget {
	if this == nil {
		return nil
	}
	head := this.child
	if head == nil {
		return nil
	}
	ret := make([]IWidget, 0, 10)

	end := head.prev
	for p := head; ; p = p.next {
		ret = append(ret, p.Self())
		if p == end {
			break
		}
	}
	return ret
}

func (this *Widget) HasCapture() bool {
	return curCapture() == this.Self()
}

func (this *Widget) PushCapture() {
	pushCapture(this.Self())
}

func (this *Widget) PopCapture() {
	popCapture(this.Self())
}

func (this *Widget) DoDragDrop(content paint.Pixmap, availableActions DndAction, data ...interface{}) DndAction {
	return this.OwnerWindow().DoDragDrop(this.Self(), content, availableActions, data...)
}

func FindWidgetUnderMouse() IWidget {
	return FindWidgetGlobal(MousePosition())
}

func FindWidgetGlobal(xg, yg float64) IWidget {
	win := FindTopWindow(xg, yg)
	if win == nil {
		return nil
	}
	widget := win.Widget()
	x, y := widget.MapFromGlobal(xg, yg)
	return widget.FindWidgetAt(x, y)
}

func (this *Widget) IsAllAncentorsVisible() bool {
	return this.CheckAllAncestors(func(iw IWidget) bool { return iw.IsVisible() })
}

func (this *Widget) ExtraData() interface{} {
	return this.extraData
}

func (this *Widget) SetExtraData(a interface{}) {
	this.extraData = a
}

func (this *Widget) ExportGv(g *gv.Graph) {
	self := this.Self()
	node := g.Node(self)
	node.Text = core.VisualString(this.Bounds1())
	node.Text += "\n" + GetDbgText(self)

	i := 0
	for _, child := range this.Children() {
		cn := g.Node(child.Self())
		edge := g.Edge(node, cn)
		edge.Text = fmt.Sprint(i)
		edge.Weight = 2
		i++
	}

	//g.Node(this.parent)

	//g.Node(this.win)

}

//func (this *Widget) ObjName() string {
//	return this.objname
//}

//func (this *Widget) SetObjName(a string) {
//	this.objname = a
//}
