package gui

import (
	"silk/core"
	"silk/geom"
	"silk/gv"
	"silk/paint"
	"silk/win32"
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

const (
	DefultLineEnd  = "\r\n"
	formClassName  = "SILK_GUI_FORM"
	popupClassName = "SILK_GUI_POPUP"
	WM_END_MODAL   = 0x8677
)

type WindowType int

const (
	WtInherit WindowType = iota
	WtForm
	WtPopup
	WtChild
)

func (wt WindowType) String() string {
	switch wt {
	case WtInherit:
		return `WtInherit`
	case WtForm:
		return `WtForm`
	case WtPopup:
		return `WtPopup`
	case WtChild:
		return `WtChild`
	default:
		return `WtUnkown`
	}
}

// 记录窗口的位置
type WindowPlace struct {
	Monitor     string
	FrameBounds geom.Rect
	Maximized   bool
	Minimized   bool
	FullScreen  bool
}

type WinId win32.HWND

var (
	hInstance        = win32.GetModuleHandle("")
	wndProcCallback  = syscall.NewCallback(wndProcFunc)
	winMap           = make(map[win32.HWND]*Window)
	mouseHoverWidget IWidget
	lastMouseWidget  IWidget
	lastMouseTime    time.Time
	mouseMoving      bool
	focusWidget      IWidget
	debugUIThreadId  uintptr
	idleTimer        Timer
	idleSkip         int
	idleFlag         bool
	activeForm       *Window
	captureStack     []IWidget
	screenDpi        float64
)

type Window struct {
	hWnd        win32.HWND
	backPainter paint.Painter
	backBuffer  paint.Pixmap
	backWidth   int
	backHeight  int
	widget      IWidget
	wt          WindowType
	title       string
	//	staticTitle  bool
	autoCaptured bool
	toCapture    bool
	mouseEntered bool
	alowDnd      bool
	inModal      bool
	closeOnHide  bool

	// 临时存放模块窗口的返回值
	modalRet interface{}

	lastDndWidget IWidget
	dndContext    *dndContext
}

func init() {
	debugUIThreadId = uintptr(win32.GetCurrentThread())
	registerWndClasses()
	idleTimer.Start(47, onIdleTimer)
	screenDpi = float64(win32.GetDeviceCaps(0, win32.LOGPIXELSX))
	if screenDpi == 0 {
		screenDpi = 96
	}
	core.Debug("Main screen DPI: ", screenDpi)
	//	core.SetMainLoop(mainLoop, quitLoop)
}

func registerWndClasses() {
	hIcon := win32.LoadIcon(hInstance, (*uint16)(unsafe.Pointer(uintptr(100))))
	if hIcon == 0 {
		hIcon = win32.LoadIcon(0, (*uint16)(unsafe.Pointer(uintptr(win32.IDI_APPLICATION))))
	}
	var wc win32.WNDCLASSEX
	wc = win32.WNDCLASSEX{
		Size:      uint32(unsafe.Sizeof(wc)),
		Instance:  hInstance,
		ClassName: syscall.StringToUTF16Ptr(formClassName),
		WndProc:   wndProcCallback,
		Icon:      hIcon,
		Style:     win32.CS_DBLCLKS | win32.CS_VREDRAW | win32.CS_HREDRAW}

	if 0 == win32.RegisterClassEx(&wc) {
		log.Panic("win32.RegisterClassEx")
	}

	wc.ClassName = syscall.StringToUTF16Ptr(popupClassName)
	wc.Style = win32.CS_DBLCLKS | win32.CS_DROPSHADOW | win32.CS_SAVEBITS
	wc.Icon = 0

	if 0 == win32.RegisterClassEx(&wc) {
		log.Panic("win32.RegisterClassEx")
	}
}

func setMouseHoverWidget(iw IWidget) {
	if mouseHoverWidget == iw {
		return
	}
	old := mouseHoverWidget
	mouseHoverWidget = iw
	if old != nil {
		i, ok := old.(IEventMouseLeave)
		if ok {
			i.OnMouseLeave()
		}
	}
	if iw != nil {
		i, ok := iw.(IEventMouseEnter)
		if ok {
			i.OnMouseEnter()
		}

	}
	//updateCursor()
}

func SetCursor(p *Cursor) {
	win32.SetCursor(p.Native())
}
func updateCursor() {
	if overrideCursor != nil {
		win32.SetCursor(overrideCursor.Native())
		return
	}

	if lastMouseWidget != nil {
		c := lastMouseWidget.Cursor()
		if c != nil {
			win32.SetCursor(c.Native())
			return
		}
	}
}

//func (this *Window) NakedWindow() *Window {
//	return this
//}d

//func newWindow(widget *Widget, wt WindowType) *Window {
//	p := new(Window)
//	p.widget = widget.Self()
//	err := p.create(widget.OwnerWindow(), wt)
//	if err != nil {
//		return nil
//	}
//	return p
//}
var uiThread win32.HANDLE

func (this *Window) create(p *Window, wt WindowType) error {
	ht := win32.GetCurrentThread()
	if uiThread == 0 {
		uiThread = ht
	} else {
		if uiThread != ht {
			panic("uiThread != ht")
		}
	}
	if this.hWnd != 0 {
		return core.StrErr("win seams already inited.")
	}

	var parentHwnd win32.HWND

	if p != nil {
		parentHwnd = p.hWnd
		core.Debug(`create "`, wt, `" for `, reflect.TypeOf(this.widget).String(),
			`, parent = `, reflect.TypeOf(p.widget).String(), ` `, unsafe.Pointer(p))
	} else {
		core.Debug(`create "`, wt, `" for `, reflect.TypeOf(this.widget).String(),
			`, parent = nil`)
	}

	var className = formClassName

	var ws uint
	var wsex uint

	var x, y, w, h int = win32.CW_USEDEFAULT, win32.CW_USEDEFAULT,
		win32.CW_USEDEFAULT, win32.CW_USEDEFAULT

	rect := this.widget.Bounds1()

	switch wt {
	case WtPopup:
		ws |= win32.WS_POPUP
		wsex |= win32.WS_EX_TOPMOST
		wsex |= win32.WS_EX_TOOLWINDOW
		x, y, w, h = 10, 10, 100, 100
		className = popupClassName
	default:
		fallthrough
	case WtForm:
		ws |= win32.WS_OVERLAPPEDWINDOW
		//w, h = 800, 600
	case WtChild:
		ws |= win32.WS_CHILD
		x, y, w, h = 10, 10, 100, 100
	case WtInherit:
		panic("try create WtInherit Window.")
	}

	hWnd := win32.CreateWindowEx(
		wsex,
		syscall.StringToUTF16Ptr(className),
		nil,
		ws|win32.WS_CLIPCHILDREN|win32.WS_CLIPSIBLINGS,
		x, y, w, h,
		parentHwnd, 0, hInstance, unsafe.Pointer(nil))

	if !win32.IsWindow(hWnd) {
		return core.StrErr("win32.CreateWindowEx")
	}

	winMap[hWnd] = this
	this.hWnd = hWnd
	this.wt = wt

	//this.widget.NakedWidget().w = -1
	//this.widget.NakedWidget().h = -1
	//core.Debug("this.widget.SetBounds1():", rect)
	if rect.Width != 0 || rect.Height != 0 {
		this.widget.SetSize(rect.Size())
	}

	if wt == WtForm {
		err := win32.RegisterDragDrop(this)
		this.alowDnd = err == nil
		if !this.alowDnd {
			core.Warn("Failed to register drag and drop: ", err)
		}
	}

	core.Debug("window create succeed.")

	return nil
}

func (this *Window) destroy() {
	this.widget = nil
	win32.DestroyWindow(this.hWnd)
	this.hWnd = 0
}

func (this *Window) getBackBuffer(s0 paint.Surface, width, height int) (backPainter paint.Painter, backBuffer paint.Pixmap) {
	width = (width + 15) / 16 * 16
	height = (height + 15) / 16 * 16
	if width == 0 {
		width = 1
	}
	if height == 0 {
		height = 1
	}
	if width != this.backWidth || height != this.backHeight {
		this.backWidth = width
		this.backHeight = height
		this.backBuffer = paint.NewPixmap(width, height)
		//this.backBuffer = s0.NewSimilar(width, height, true, false)
		this.backPainter = this.backBuffer.NewPainter()
		//core.Debug("backBuffer.type = ", this.backBuffer.SurfaceType())
		//if w1, h1 := this.backBuffer.Width(), this.backBuffer.Height(); w1 != width || h1 != height {
		//	core.Debug("failed to create backbuffer: ", w1, width, h1, height)
		//}
	}

	return this.backPainter, this.backBuffer
}

func (this *Window) Close() {
	core.Debug("(this *Window) Close()")
	// 防止重复关闭
	if this.hWnd == 0 {
		return
	}

	// 关闭模态窗口
	if this.inModal {
		this.EndModal(nil)
		return
	}

	if iclose, ok := this.widget.(interface {
		Close()
	}); ok {
		iclose.Close()
	}

	win32.DestroyWindow(this.hWnd)
}

func (this *Window) Save() bool {
	core.Debug("Window.Save()")
	if isave, ok := this.widget.(interface {
		Save() bool
	}); ok {
		return isave.Save()
	}
	return true
}

func (this *Window) DirtyList() []string {
	if idirty, ok := this.widget.(interface {
		DirtyList() []string
	}); ok {
		return idirty.DirtyList()
	}
	return nil
}

func (this *Window) Native() WinId {
	return WinId(this.hWnd)
}

func (this *Window) OnDestroy() {
	if this.hWnd == 0 {
		core.Warn("window seams already destroyed.")
		return
	} else {
		core.Debug(`window destroyed. title = "` + this.Title() +
			`", widget = "` + core.ObjInfo(this.widget) + `"`)
	}
	delete(winMap, this.hWnd)
	this.hWnd = 0
}

func (this *Window) SetFrameBounds(x, y, w, h float64) {
	m := this.FrameMargin()
	x1, y1, w1, h1 := m.Apply(x, y, w, h)
	this.SetBounds(x1, y1, w1, h1)
}

func (this *Window) FrameBounds() (x, y, w, h float64) {
	m := this.FrameMargin()
	x, y, w, h = this.Bounds()
	x -= m.L
	y -= m.T
	w += m.L + m.R
	h += m.T + m.B
	return
}

func (this *Window) SetFrameBounds1(rect geom.Rect) {
	this.SetFrameBounds(rect.X, rect.Y, rect.Width, rect.Height)
}

func (this *Window) FrameBounds1() geom.Rect {
	x, y, w, h := this.FrameBounds()
	return geom.Rect{x, y, w, h}
}

func (this *Window) ParentWindow() *Window {
	p := this.widget.Parent()
	if p != nil {
		return p.OwnerWindow()
	}
	return nil
}

func (this *Window) WindowType() WindowType {
	return this.wt
}

//func (this *Window) setBounds(x, y, width, height float64) {
//	rect := &win32.RECT{int32(x), int32(y), int32(x + width), int32(y + height)}
//	if this.wt == WtForm {
//		style := win32.GetWindowLongPtr(this.hWnd, win32.GWL_STYLE)
//		win32.AdjustWindowRect(rect, uint(style), false)
//	}
//	win32.MoveWindow(this.hWnd, int(rect.Left), int(rect.Top),
//		int(rect.Right-rect.Left), int(rect.Bottom-rect.Top), true)
//}

func (this *Window) Bounds() (x, y, width, height float64) {
	// 在没有BUG时, widget的Bounds和窗口的位置是对应的
	// 理论上直接返回widget的Bounds即可
	return this.widget.Bounds()
}

func (this *Window) SetBounds(x, y, width, height float64) {
	// SetBounds只是把两步合成一步
	this.SetPos(x, y)
	this.SetSize(width, height)
}

func (this *Window) SetPos(x, y float64) {
	// 完整数据流程参见Widget.SetPos
	// 此方法负责把坐标传给系统API, 不能反过来调用Widget.SetPos

	// 注意: ParentWindow和win32.GetParent可能不一致

	// 转换为全局坐标
	xg, yg := x, y
	pw := this.Widget().Parent()
	if pw != nil {
		xg, yg = pw.MapToGlobal(xg, yg)
	}
	//	core.Debug("x, y =", x, y)
	//	core.Debug("xg, yg =", xg, yg)

	// 转换为系统层面的父窗口的客户区坐标
	var xc, yc float64
	hParent := win32.GetParent(this.hWnd)
	if hParent != 0 {
		x1, y1, _ := win32.ScreenToClient(hParent, int(xg), int(yg))
		xc, yc = float64(x1), float64(y1)
	} else {
		xc, yc = xg, yg
	}

	m := this.FrameMargin()

	// 调用相应API
	//core.Debug("before win32.SetWindowPos")
	//core.Warn("before win32.SetWindowPos")
	win32.SetWindowPos(this.hWnd, 0, int(xc-m.L), int(yc-m.T), 0, 0,
		win32.SWP_NOOWNERZORDER|win32.SWP_NOZORDER|win32.SWP_NOSIZE)
	//core.Warn("after win32.SetWindowPos")

	//func (this *Window) setBounds(x, y, width, height float64) {
	//	rect := &win32.RECT{int32(x), int32(y), int32(x + width), int32(y + height)}
	//	if this.wt == WtForm {
	//		style := win32.GetWindowLongPtr(this.hWnd, win32.GWL_STYLE)
	//		win32.AdjustWindowRect(rect, uint(style), false)
	//	}
	//	win32.MoveWindow(this.hWnd, int(rect.Left), int(rect.Top),
	//		int(rect.Right-rect.Left), int(rect.Bottom-rect.Top), true)
	//}

}

func (this *Window) FrameMargin() (ret Padding) {
	if this.wt == WtForm {
		rect := &win32.RECT{100, 100, 200, 200}
		style := win32.GetWindowLongPtr(this.hWnd, win32.GWL_STYLE)
		win32.AdjustWindowRect(rect, uint(style), false)
		ret.L = float64(100 - rect.Left)
		ret.T = float64(100 - rect.Top)
		ret.R = float64(rect.Right - 200)
		ret.B = float64(rect.Bottom - 200)
	}
	return
}

func (this *Window) SetSize(w, h float64) {
	// 完整数据流程参见Widget.SetSize
	// 此方法负责把坐标传给系统API, 不能反过来调用Windeget.SetSize
	// core.Debug("(this *Window) SetSize()", w, h)
	m := this.FrameMargin()
	// 调用相应API

	win32.SetWindowPos(this.hWnd, 0, 0, 0, int(w+m.L+m.R), int(h+m.T+m.B),
		win32.SWP_NOOWNERZORDER|win32.SWP_NOZORDER|win32.SWP_NOMOVE)

}

func (win *Window) on_WM_SIZE(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	if wParam == win32.SIZE_MINIMIZED {
		return 0
	}

	w := float64(win32.LOWORD(uint32(lParam)))
	h := float64(win32.HIWORD(uint32(lParam)))
	win.widget.NakedWidget().setSize(w, h)
	return 0
}

func (win *Window) on_WM_MOVE(msg uint32, wParam, lParam uintptr) (ret uintptr) {

	xc, yc := posFromLParam(lParam)
	m := win.FrameMargin()
	xc += m.L
	yc += m.T

	// 转换为全局坐标
	var xg, yg float64
	hParent := win32.GetParent(win.hWnd)
	if hParent != 0 {
		x1, y1 := win32.ClientToScreen(hParent, int(xc), int(yc))
		xg, yg = float64(x1), float64(y1)
	} else {
		xg, yg = xc, yc
	}

	var x, y float64
	pw := win.Widget().Parent()
	if pw != nil {
		x, y = pw.MapFromGlobal(xg, yg)
	} else {
		x, y = xg, yg
	}

	win.widget.NakedWidget().setPos(x, y)
	return 0
}

func (this *Window) Update() {
	win32.InvalidateRect(this.hWnd, nil, false)
}

func (this *Window) UpdateRect(x, y, width, height float64) {
	//core.MoreDebug(x, y, width, height)
	rect := &win32.RECT{int32(x), int32(y), int32(x + width), int32(y + height)}
	win32.InvalidateRect(this.hWnd, rect, false)
}

func (this *Window) mapFromGlobal(x, y float64) (x1, y1 float64) {
	x0, y0, _ := win32.ScreenToClient(this.hWnd, int(x), int(y))
	return float64(x0), float64(y0)
}

func (this *Window) mapToGlobal(x, y float64) (x1, y1 float64) {
	x0, y0 := win32.ClientToScreen(this.hWnd, int(x), int(y))
	return float64(x0), float64(y0)
}

func (this *Window) IsValid() bool {
	return this.hWnd != 0
}

func (this *Window) Title() string {
	//core.Debug("-> J")
	//s := win32.GetWindowText(this.hWnd)
	//core.Debug("-> K")
	return this.title
}

func (this *Window) SetTitle(s string) {
	win32.SetWindowText(this.hWnd, s)
	this.title = s
}

func (this *Window) setIcon(icon paint.Icon, small bool) {
	var szType int
	var iconType uintptr
	if small {
		szType = win32.SM_CXSMICON
		iconType = win32.ICON_SMALL
	} else {
		szType = win32.SM_CXICON
		iconType = win32.ICON_BIG
	}

	sz := win32.GetSystemMetrics(szType)
	hIcon, err := NewCursorFromIcon(icon, sz, 0, 0)
	//core.Debug(hIcon, err)
	if err == nil {
		r := win32.SendMessage(this.hWnd, win32.WM_SETICON, iconType, uintptr(hIcon.Detach()))
		if r != 0 {
			win32.DestroyIcon(win32.HICON(r))
		}
	} else {
		core.Warn(err)
	}
}

func (this *Window) SetIcon(icon paint.Icon) {
	if icon == nil {
		return
	}
	this.setIcon(icon, true)
	this.setIcon(icon, false)
}

func (this *Window) updateTitle() {
	//	return
	var newTitle string
	if iTitle, ok := this.widget.Self().(ITitle); ok {
		newTitle = iTitle.Title()
	} else if iText, ok := this.widget.Self().(IText); ok {
		newTitle = iText.Text()
	} else if iString, ok := this.widget.Self().(IString); ok {
		newTitle = iString.String()
	}
	this.SetTitle(newTitle)
}

func getCaptureWin() *Window {
	hWnd := win32.GetCapture()
	if hWnd == 0 {
		return nil
	}
	p := winMap[hWnd]
	//if p != nil {
	//	core.MoreDebug("getCaptureWin: ", core.ObjInfo(p.widget))
	//} else {
	//	core.MoreDebug("getCaptureWin: nil")
	//}
	return p
}

func releaseCapture() {
	win32.ReleaseCapture()
}

func setCapture(iw IWidget) {
	win := iw.OwnerWindow()
	if win == nil {
		return
	}
	win32.SetCapture(win.hWnd)
	lastMouseWidget = iw
}

func curCapture() IWidget {
	n := len(captureStack)
	if n == 0 {
		return nil
	} else {
		return captureStack[n-1]
	}
}

func pushCapture(w IWidget) bool {
	cur := curCapture()
	if cur == w {
		return false
	}
	if cur != nil {
		//core.MoreDebug("pushCapture: cur != nil")
		for p := w.Parent(); p != nil; p = p.Parent() {
			if p == cur {
				//core.MoreDebug("pushCapture: p == cur")
				captureStack = append(captureStack, w)
				setCapture(w)
				return true
			}
		}
	}
	captureStack = append(captureStack[:0], w)
	//core.MoreDebug("pushCapture: captureStack = append(captureStack[:0], w)")
	setCapture(w)
	return true
}

func popCapture(w IWidget) {
	if w == nil {
		return
	}
	cur := curCapture()
	if cur != w {
		return
	}
	releaseCapture()
	n := len(captureStack)
	for n := n - 2; n >= 0; n-- {
		p := captureStack[n]
		if p.IsVisible() {
			//core.MoreDebug("popCapture: p.IsVisible()")
			captureStack = captureStack[:n+1]
			setCapture(p)
			return
		}
	}

	captureStack = captureStack[:0]
	//core.MoreDebug("popCapture: captureStack = captureStack[:0]")
}

func (this *Window) OnIdle() {
	if this.wt == WtForm && this.Title() == "" {
		this.updateTitle()
	}

	if im, ok := this.widget.(interface {
		OnIdle()
	}); ok {
		im.OnIdle()
	}
	//core.Debug("(this *Window) OnIdle(): runtime.GC()")
	//runtime.GC()
}

func intersectRect(x0, y0, w0, h0, x1, y1, w1, h1 float64) (x, y, w, h float64) {
	r0 := x0 + w0
	b0 := y0 + h0
	r1 := x1 + w1
	b1 := y1 + h1
	x = math.Max(x0, x1)
	y = math.Max(y0, y1)
	w = math.Min(r0, r1) - x
	h = math.Min(b0, b1) - y
	return
}

func drawWidgetSelf(iw IWidget, g paint.Painter) (ok bool) {
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover drawWidgetSelf(...): %s", e))
		}
	}()

	iw.Draw(g)
	return true
}

func drawWidgetOverlay(iw IWidget, g paint.Painter) (ok bool) {
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover drawWidgetOverlay(...): %s", e))
		}
	}()
	if ida, ok := iw.(IDrawOverlay); ok {
		ida.DrawOverlay(g)
	}
	if core.IsDebugOn() {
		g.SetPen1(paint.Color{255, 127, 0, 64}, 1)
		_, _, w, h := iw.Bounds()
		g.Rectangle(0, 0, w, h)
		g.Stroke()
	}
	return true
}

func drawWidgetChildren(iw IWidget, g paint.Painter) {
	cx, cy, cw, ch := g.ClipBounds()
	head := iw.NakedWidget().child
	if head != nil {
		end := head.prev
		for c := head; ; c = c.next {
			ic := c.Self()
			if ic.IsVisible() && ic.Window() == nil {
				x, y, width, height := c.Bounds()
				cx1, cy1, cw1, ch1 := intersectRect(x, y, width, height, cx, cy, cw, ch)
				DrawWidgetAll(ic, g, x, y, cx1, cy1, cw1, ch1)
			}
			if c == end {
				break
			}
		}
	}
}

func DrawWidgetAll(ic IWidget, g paint.Painter, tx, ty, cx1, cy1, cw1, ch1 float64) {
	if cw1 <= 0 || ch1 <= 0 {
		return
	}

	sn0 := g.Save()
	g.Rectangle(cx1, cy1, cw1, ch1)
	g.Clip()
	g.Translate(tx, ty)

	if ok := drawWidgetSelf(ic, g); ok {
		sn1 := g.CurrentState()
		if sn0+1 != sn1 {
			core.Warn(`unbalance painter save()/restore() in "`, reflect.TypeOf(ic).Elem().Name(), `"`)
			g.RestoreTo(sn0 + 1)
		}
	} else {
		g.RestoreTo(sn0)
		drawErrCross(g, tx, ty, ic.Width(), ic.Height())
		sn0 = g.Save()
		g.Rectangle(cx1, cy1, cw1, ch1)
		g.Clip()
		g.Translate(tx, ty)
	}

	drawWidgetChildren(ic, g)

	if ok := drawWidgetOverlay(ic, g); ok {
		sn1 := g.Restore()
		if sn0 != sn1 {
			core.Warn(`unbalance painter save()/restore() in "`, reflect.TypeOf(ic).Elem().Name(), `"`)
			g.RestoreTo(sn0)
		}

	} else {
		g.RestoreTo(sn0)
		drawErrCross(g, tx, ty, ic.Width(), ic.Height())
	}

}

func (this *Window) IsVisible() bool {
	return win32.IsWindowVisible(this.hWnd)
}

func (this *Window) setVisible(b bool) {
	//core.Debug("enter (this *Window) setVisible(), hwnd=", fmt.Sprintf("%08X", this.hWnd))
	if b == this.IsVisible() {
		return
	}
	if b {
		win32.ShowWindow(this.hWnd, win32.SW_SHOW)
	} else {
		win32.ShowWindow(this.hWnd, win32.SW_HIDE)
	}
	//core.Debug("leave (this *Window) setVisible()")
}

func (this *Window) SetVisible(b bool) {
	this.widget.SetVisible(b)
}

func (this *Window) IsActive() bool {
	return win32.GetActiveWindow() == this.hWnd
}

func (this *Window) Widget() IWidget {
	return this.widget
}

func (this *Window) NakedWindow() *Window {
	return this
}

func MousePosition() (x, y float64) {
	x1, y1, _ := win32.GetCursorPos()
	return float64(x1), float64(y1)
}

/*
	ofn.hwndOwner = null;
	ofn.lpstrFile = szFile;
	ofn.lpstrFile[0] = L'\0';
	ofn.nMaxFile = MAX_PATH;
	ofn.lpstrFilter = L"Supported files\0*.mid;*.midi;*.ogg;*.oga;*.wav;*.wave\0All files\0*.*\0\0";
	ofn.nFilterIndex = 1;
	ofn.lpstrFileTitle = null;
	ofn.nMaxFileTitle = 0;
	ofn.lpstrInitialDir = null;
	ofn.Flags = OFN_PATHMUSTEXIST | OFN_FILEMUSTEXIST | OFN_HIDEREADONLY;

*/

// silkUIFilterUTF16 builds the GetOpenFileName/GetSaveFileName filter buffer.
//
// The Windows OPENFILENAME.lpstrFilter format is a series of UTF-16 strings
// terminated by a pair of consecutive NUL characters, with each filter being
// two strings: a human-readable description, then the file mask. We put the
// SilkUI filter first so it becomes the default when the dialog opens.
//
// Legacy design files (.cml / .silk / .form) remain accepted via the
// second entry so older projects still open cleanly.
func silkUIFilterUTF16() []uint16 {
	parts := []string{
		"SilkUI Files (*.silkui)", "*.silkui",
		"Legacy Design Files (*.cml;*.silk;*.form)", "*.cml;*.silk;*.form",
		"All Files (*.*)", "*.*",
	}
	var buf []uint16
	for _, s := range parts {
		for _, r := range s {
			buf = append(buf, uint16(r))
		}
		buf = append(buf, 0)
	}
	buf = append(buf, 0) // extra terminating NUL
	return buf
}

func OpenFileDialog() string {
	var fileBuf = make([]uint16, win32.MAX_PATH)
	ofn := win32.OPENFILENAME{}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	core.Debug("ofn.StructSize = ", ofn.StructSize)
	ofn.Instance = hInstance
	ofn.Owner = 0
	ofn.File = &fileBuf[0]
	fileBuf[0] = 0
	ofn.MaxFile = win32.MAX_PATH
	filter := silkUIFilterUTF16()
	ofn.Filter = &filter[0]
	ofn.FilterIndex = 1
	ofn.Flags = win32.OFN_PATHMUSTEXIST | win32.OFN_FILEMUSTEXIST | win32.OFN_HIDEREADONLY
	ok := win32.GetOpenFileName(&ofn)
	if ok {
		s := syscall.UTF16ToString(fileBuf)
		s = strings.Replace(s, `\`, `/`, -1)
		return s
	}
	return ""
}

func SaveFileDialog() string {
	var fileBuf = make([]uint16, win32.MAX_PATH)
	ofn := win32.OPENFILENAME{}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	core.Debug("ofn.StructSize = ", ofn.StructSize)
	ofn.Instance = hInstance
	ofn.Owner = 0
	ofn.File = &fileBuf[0]
	fileBuf[0] = 0
	ofn.MaxFile = win32.MAX_PATH
	filter := silkUIFilterUTF16()
	ofn.Filter = &filter[0]
	ofn.FilterIndex = 1
	// Default extension used when the user types a name without one.
	defExt := []uint16{'s', 'i', 'l', 'k', 'u', 'i', 0}
	ofn.DefExt = &defExt[0]
	ofn.Flags = win32.OFN_HIDEREADONLY | win32.OFN_OVERWRITEPROMPT
	ok := win32.GetSaveFileName(&ofn)
	if ok {
		s := syscall.UTF16ToString(fileBuf)
		s = strings.Replace(s, `\`, `/`, -1)
		return s
	}
	return ""
}

const nc_active_hack_code = 11111

func wndProcFunc(hWnd win32.HWND, msg uint32, wParam, lParam uintptr) (ret uintptr) {
	//core.Debug("wndProcFunc, hWnd=", fmt.Sprintf("%08X", hWnd))
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover wndProcFunc(%X, %d, %d, %d) : %v ", hWnd, msg, wParam, lParam, e))
			ret = win32.DefWindowProc(hWnd, msg, wParam, lParam)
		}
	}()

	win := winMap[hWnd]
	if win == nil {
		return win32.DefWindowProc(hWnd, msg, wParam, lParam)
	}

	switch msg {
	case win32.WM_KEYDOWN:
		if i, ok := focusWidget.(IEventKeyDown); ok {
			i.OnKeyDown(int(wParam), 0x40000000&lParam != 0)
		}

	case win32.WM_KEYUP:
		if i, ok := focusWidget.(IEventKeyUp); ok {
			i.OnKeyUp(int(wParam))
		}
	case win32.WM_MOVE:
		return win.on_WM_MOVE(msg, wParam, lParam)
	case win32.WM_SIZE:
		return win.on_WM_SIZE(msg, wParam, lParam)
	case win32.WM_SHOWWINDOW:
		return win.on_WM_SHOWWINDOW(msg, wParam, lParam)
	case win32.WM_DESTROY:
		win.OnDestroy()
		if len(winMap) == 0 {
			win32.PostQuitMessage(0)
		}
	case win32.WM_PAINT:
		//core.MoreDebug("WM_PAINT ")
		return win.on_WM_PAINT(msg, wParam, lParam)
	case win32.WM_LBUTTONDOWN:
		return win.on_WM_LBUTTONDOWN(msg, wParam, lParam)
	case win32.WM_LBUTTONUP:
		return win.on_WM_LBUTTONUP(msg, wParam, lParam)
	case win32.WM_RBUTTONDOWN:
		return win.on_WM_RBUTTONDOWN(msg, wParam, lParam)
	case win32.WM_RBUTTONUP:
		return win.on_WM_RBUTTONUP(msg, wParam, lParam)
	case win32.WM_MOUSEMOVE:
		return win.on_WM_MOUSEMOVE(msg, wParam, lParam)
	case win32.WM_MOUSEWHEEL:
		return win.on_WM_MOUSEWHEEL(msg, wParam, lParam)
	case win32.WM_ERASEBKGND:
		return 1
	case win32.WM_CHAR:
		if it, ok := focusWidget.(IEventTextInput); ok {
			r := rune(wParam)
			if !unicode.IsControl(r) {
				utf16 := []uint16{uint16(wParam), 0}
				it.OnTextInput(syscall.UTF16ToString(utf16))
				return 0
			}
		}
		return win32.DefWindowProc(hWnd, msg, wParam, lParam)
	case win32.WM_ACTIVATE:
		if win.wt == WtForm {
			isActivate := win32.LOWORD(uint32(wParam)) == win32.WA_ACTIVE ||
				win32.LOWORD(uint32(wParam)) == win32.WA_CLICKACTIVE
			if isActivate && activeForm != win {
				if activeForm != nil && activeForm.hWnd != 0 {
					hackPaintActiveDecorator(activeForm.hWnd, 0)
				}
				activeForm = win
			}
		}
		return win32.DefWindowProc(hWnd, msg, wParam, lParam)
	case win32.WM_ACTIVATEAPP:
		hackPaintActiveDecorator(hWnd, wParam)
		return win32.DefWindowProc(hWnd, msg, wParam, lParam)
	case win32.WM_NCACTIVATE:
		if lParam == nc_active_hack_code {
			return win32.DefWindowProc(hWnd, msg, 0, lParam)
		} else {
			return win32.DefWindowProc(hWnd, msg, 1, lParam)
		}
	case win32.WM_CLOSE:
		core.Debug(`Receive "Close Window" command from system.`)
		PromptSaveClose(win.widget, win)
	default:
		return win32.DefWindowProc(hWnd, msg, wParam, lParam)
	}
	return 0
}

func (win *Window) on_WM_PAINT(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	paintStruct := win32.PAINTSTRUCT{}
	win32.BeginPaint(win.hWnd, &paintStruct)
	winSurface := paint.NewWin32Surface(uintptr(paintStruct.Hdc))
	//if win.DoubleBuffered() {
	width, height := win.widget.Size()
	backPainter, backBuffer := win.getBackBuffer(winSurface, int(width), int(height))

	backPainter.Save()
	rc := paintStruct.RcPaint
	DrawWidgetAll(win.widget, backPainter,
		0, 0, float64(rc.Left), float64(rc.Top),
		float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top))
	backPainter.Restore()
	backBuffer.Flush()

	c := winSurface.NewPainter()
	//	c.SetSourceSurface(backBuffer, 0, 0)
	c.Rectangle(float64(rc.Left), float64(rc.Top),
		float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top))
	c.Clip()
	//c.Paint()
	c.DrawPixmap(backBuffer)

	winSurface.Finish()
	win32.EndPaint(win.hWnd, &paintStruct)
	return 0
}
func (win *Window) on_WM_SHOWWINDOW(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	visible := wParam != 0
	win.widget.(iWidget).setVisible(visible)
	if !visible && win.closeOnHide {
		win.Close()
		return 0
	}
	return win32.DefWindowProc(win.hWnd, msg, wParam, lParam)
}

func (win *Window) on_WM_LBUTTONDOWN(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	lastMouseTime = time.Now()
	win.toCapture = true
	x, y := posFromLParam(lParam)
	if lastMouseWidget == nil {
		p := win.widget.FindWidgetAt(x, y)
		lastMouseWidget = p
	}
	if lastMouseWidget != nil {
		x1, y1 := lastMouseWidget.MapFromWindow(x, y)
		widget := lastMouseWidget.NakedWidget()
		if widget.child != nil && curCapture() == lastMouseWidget {
			p1 := widget.FindWidgetAt(x1, y1)
			if p1 != nil && p1 != widget {
				lastMouseWidget = p1
				x1, y1 = lastMouseWidget.MapFromWindow(x, y)
				// 在子控件里点击, 无论是否拖动, 都要让子控件捕获鼠标
				win.autoCaptured = pushCapture(lastMouseWidget)
				win.toCapture = false
			}
		}
		if i, ok := lastMouseWidget.(IEventLeftDown); ok {
			i.OnLeftDown(x1, y1)
		}
	}
	return 0
}

func (win *Window) on_WM_LBUTTONUP(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	//core.MoreDebug("on_WM_LBUTTONUP")
	lastMouseTime = time.Now()
	if lastMouseWidget != nil {
		cw := lastMouseWidget
		if i, ok := lastMouseWidget.(IEventLeftUp); ok {
			x, y := posFromLParam(lParam)
			x1, y1 := lastMouseWidget.MapFromWindow(x, y)
			i.OnLeftUp(x1, y1)
		}
		if win.autoCaptured {
			popCapture(cw)
		}
	}
	win.autoCaptured = false
	win.toCapture = false
	return 0
}

func (win *Window) on_WM_RBUTTONDOWN(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	lastMouseTime = time.Now()
	win.toCapture = true
	x, y := posFromLParam(lParam)
	if lastMouseWidget == nil {
		p := win.widget.FindWidgetAt(x, y)
		lastMouseWidget = p
	}
	if lastMouseWidget != nil {
		x1, y1 := lastMouseWidget.MapFromWindow(x, y)
		widget := lastMouseWidget.NakedWidget()
		if widget.child != nil && curCapture() == lastMouseWidget {
			p1 := widget.FindWidgetAt(x1, y1)
			if p1 != nil && p1 != widget {
				lastMouseWidget = p1
				x1, y1 = lastMouseWidget.MapFromWindow(x, y)
				// 在子控件里点击, 无论是否拖动, 都要让子控件捕获鼠标
				win.autoCaptured = pushCapture(lastMouseWidget)
				win.toCapture = false
			}
		}
		if i, ok := lastMouseWidget.(IEventRightDown); ok {
			i.OnRightDown(x1, y1)
		}
	}
	return 0
}

func (win *Window) on_WM_RBUTTONUP(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	//core.MoreDebug("on_WM_LBUTTONUP")
	lastMouseTime = time.Now()
	if lastMouseWidget != nil {
		cw := lastMouseWidget
		if i, ok := lastMouseWidget.(IEventRightUp); ok {
			x, y := posFromLParam(lParam)
			x1, y1 := lastMouseWidget.MapFromWindow(x, y)
			i.OnRightUp(x1, y1)
		}
		if win.autoCaptured {
			popCapture(cw)
		}
	}
	win.autoCaptured = false
	win.toCapture = false
	return 0
}

func (win *Window) on_WM_MOUSEWHEEL(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	lastMouseTime = time.Now()
	if lastMouseWidget != nil {
		if i, ok := lastMouseWidget.(IEventMouseWheel); ok {
			x, y := posFromLParam(lParam)
			x1, y1 := lastMouseWidget.MapFromGlobal(x, y)
			uz := win32.HIWORD(uint32(wParam))
			z := float64(int16(uz)) / 120.0
			i.OnMouseWheel(x1, y1, z)
		}
	}

	return 0

}

func (win *Window) on_WM_MOUSEMOVE(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	lastMouseTime = time.Now()
	mouseMoving = true

	// 拖动时, 自动捕获鼠标
	if win.toCapture {
		win.toCapture = false
		if lastMouseWidget != nil {
			win.autoCaptured = pushCapture(lastMouseWidget)
		}
	}

	_, _, ww, wh := win.widget.Bounds()
	x, y := posFromLParam(lParam)
	inClient := x >= 0 && y >= 0 && x < ww && y < wh
	win.mouseEntered = inClient
	var capWin *Window
	capWidget := curCapture()
	if capWidget != nil {
		capWin = getCaptureWin()
	}
	if capWin == win &&
		lastMouseWidget != nil {
		p := lastMouseWidget
		_, _, pw, ph := p.Bounds()
		x1, y1 := p.MapFromWindow(x, y)

		widget := p.NakedWidget()
		redirect := false
		if widget.child != nil && capWidget == lastMouseWidget &&
			!IsMouseLeftDown() && !IsMouseRightDown() {
			p1 := widget.FindWidgetAt(x1, y1)
			if p1 != nil && p1 != widget {
				x1, y1 := p1.MapFromWindow(x, y)
				setMouseHoverWidget(p1)

				if i, ok := p1.(IEventMouseMove); ok {
					//if ip, ok := p1.(IOnPrepare); ok {
					//	ip.OnPrepare()
					//}
					i.OnMouseMove(x1, y1)
				}
				redirect = true
			}
		}
		if !redirect {
			if x1 >= 0 && y1 >= 0 && x1 < pw && y1 < ph {
				setMouseHoverWidget(p)
			} else {
				setMouseHoverWidget(nil)
			}

			if i, ok := p.(IEventMouseMove); ok {
				//if ip, ok := p.(IOnPrepare); ok {
				//	ip.OnPrepare()
				//}
				i.OnMouseMove(x1, y1)
			}
		}
	} else if inClient {

		p := win.widget.FindWidgetAt(x, y)
		//core.MoreDebug("inClient ", x, y)
		setMouseHoverWidget(p)
		lastMouseWidget = p
		if win == capWin {
			win32.ReleaseCapture()
		}
		if p != nil {
			if i, ok := p.Self().(IEventMouseMove); ok {
				//if ip, ok := p.(IOnPrepare); ok {
				//	ip.OnPrepare()
				//}
				x1, y1 := p.MapFromWindow(x, y)
				i.OnMouseMove(x1, y1)
			}
		}
	} else {
		core.Debug("!inClient")
	}
	updateCursor()
	return 0
}

func hackPaintActiveDecorator(hWnd win32.HWND, active uintptr) {
	win32.SendMessage(hWnd, win32.WM_NCACTIVATE, active, nc_active_hack_code)
}

func handleNonUIMessages(msg uint32, wParam, lParam uintptr) (ret uintptr) {
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover handleNonUIMessages(%d, %d, %d) : %v ", msg, wParam, lParam, e))
			ret = win32.DefWindowProc(0, msg, wParam, lParam)
		}
	}()
	switch msg {
	case win32.WM_TIMER:
		if f, ok := timerMap[wParam]; ok {
			f()
			return 0
		}
	}
	return win32.DefWindowProc(0, msg, wParam, lParam)
}

func onMouseTimer() {
	if !mouseMoving {
		return
	}
	d := time.Now().Sub(lastMouseTime)
	if d > 0 && d <= 300*time.Millisecond {
		return
	}
	lastMouseTime = time.Now()
	mouseMoving = false
	if lastMouseWidget == nil {
		return
	}
	p := lastMouseWidget
	win := p.OwnerWindow()
	if win == nil || !win.IsValid() {
		lastMouseWidget = nil
		return
	}
	x, y := win.mapFromGlobal(MousePosition())

	w0, h0 := win.widget.Size()
	if x < 0 || y < 0 || x > w0 || y > h0 {
		setMouseHoverWidget(nil)
	}

	x1, y1 := p.MapFromWindow(x, y)
	widget := p.NakedWidget()
	if widget.child != nil && curCapture() == p {
		p1 := widget.FindWidgetAt(x1, y1)
		if p1 != nil && p1 != widget {
			p = p1
			x1, y1 = p.MapFromWindow(x, y)
		}
	}

	if p != nil {
		if i, ok := p.Self().(IEventMouseStop); ok {
			//if ip, ok := p.(IOnPrepare); ok {
			//	ip.OnPrepare()
			//}
			i.OnMouseStop(x1, y1)
		}
	}

}

func onIdle() {
	idleFlag = false
	idleSkip = 0
	for _, win := range winMap {
		win.OnIdle()
	}

	//core.OnIdle()
}

func onIdleTimer() {
	onMouseTimer()

	if idleSkip == 6 {
		onIdle()
		return
	}

	if win32.GetQueueStatus(0x05ff) == 0 {
		if idleFlag {
			onIdle()
			return
		} else {
			idleFlag = true
		}
	}

	idleSkip++
}

func HideConsoleWindow() {
	hConsoleWindow := win32.GetConsoleWindow()
	if hConsoleWindow != 0 {
		win32.ShowWindow(hConsoleWindow, win32.SW_HIDE)
	}
}

func ShowConsoleWindow() {
	hConsoleWindow := win32.GetConsoleWindow()
	if hConsoleWindow != 0 {
		win32.ShowWindow(hConsoleWindow, win32.SW_SHOW)
	}
}

// markAllWindowsDirty is the Windows counterpart to the GLFW one. The Win32
// backend does not cache a per-window dirty flag (it repaints on WM_PAINT),
// so we invalidate every known window. This is used by the F12 perf overlay
// toggle.
func markAllWindowsDirty() {
	for hWnd := range winMap {
		if hWnd != 0 {
			win32.InvalidateRect(hWnd, nil, false)
		}
	}
}

func MainLoop() {
	core.Debug("MainLoop()")
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover MainLoop() : %v ", e))
		}

		// 退出运行之前保存设置
		//		core.OnExit()
	}()

	if len(winMap) == 0 {
		core.Warn("no windows.")
		return
	}

	if !core.IsDebugOn() {
		HideConsoleWindow()
	}

	msg := win32.MSG{}
	for {
		if !win32.GetMessage(&msg, 0, 0, 0) || msg.Message == win32.WM_QUIT {
			break
		}

		if msg.Hwnd == 0 {
			handleNonUIMessages(msg.Message, msg.WParam, msg.LParam)
		} else {
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		}
	}
}

func posFromLParam(lParam uintptr) (x, y float64) {
	ux := win32.LOWORD(uint32(lParam))
	uy := win32.HIWORD(uint32(lParam))
	x = float64(int16(ux))
	y = float64(int16(uy))
	return
}

func DesktopArea() (x, y, w, h float64) {
	const SPI_GETWORKAREA = 0x0030
	var rc win32.RECT
	if win32.SystemParametersInfo(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&rc)), 0) {
		x = float64(rc.Left)
		y = float64(rc.Top)
		w = float64(rc.Right - rc.Left)
		h = float64(rc.Bottom - rc.Top)
	} else {
		x, y, w, h = 0, 0, 1024, 768
	}
	return
}

// 主屏幕DPI, 即每英寸多少像素.
// 如果像素纵横比不同则以横向为准, 此时软件仍能运行, 但图形可能会变形
func ScreenDpi() float64 {
	return screenDpi
}

// 主屏幕DPMM, 即每毫米多少像素.
// 如果像素纵横比不同则以横向为准, 此时软件仍能运行, 但图形可能会变形
func ScreenDpmm() float64 {
	return screenDpi / 25.4
}

// 主屏幕像素距离换算成毫米距离
func PixelToMm(pixelLen float64) (mmLen float64) {
	mmLen = 25.4 * pixelLen / screenDpi
	return
}

// 主屏幕毫米距离换算为像素距离
func MmToPixel(mmLen float64) (pixelLen float64) {
	pixelLen = mmLen * screenDpi / 25.4
	return
}

// 主屏幕毫米距离换算为像素距离, 并舍入到整数
func MmToPixelZ(mmLen float64) (pixelLen float64) {
	a := mmLen * screenDpi / 25.4
	b := paint.Round(a)
	return b
}

func emulateMouseDown(left bool) {
	x, y, ok := win32.GetCursorPos()
	if !ok {
		return
	}
	var input win32.INPUT
	input.Type = win32.INPUT_MOUSE
	input.Mi.Dx = int32(x)
	input.Mi.Dy = int32(y)
	input.Mi.DwFlags = win32.MOUSEEVENTF_ABSOLUTE
	if left {
		input.Mi.DwFlags |= win32.MOUSEEVENTF_LEFTDOWN
	} else {
		input.Mi.DwFlags |= win32.MOUSEEVENTF_RIGHTDOWN
	}

	win32.SendInput([]win32.INPUT{input})
}

func FindTopWindow(xg, yg float64) *Window {
	//core.MoreDebug("FindTopWindow(", xg, ",", yg, ")")
	hWnd := win32.WindowFromPoint(int32(xg), int32(yg))
	winp := winMap[hWnd]
	if winp == nil {
		if hWnd != 0 {
			core.Debug("FindTopWindow(", xg, ",", yg,
				"): found a Window create by other system: hWnd=", hWnd)
		}
		return nil
	}
	return winp
}

func FindTopWindowUnderMouse() *Window {
	return FindTopWindow(MousePosition())
}

func AnyWindowId() (ret WinId) {
	for id, win := range winMap {
		ret = WinId(id)
		if win.wt == WtForm {
			return
		}
	}
	return
}

func KeyState(key int) (down, checked bool) {
	state := win32.GetKeyState(key)
	down = state&0x0100 != 0
	checked = state&0x0001 != 0
	return
}

func IsKeyDown(key int) bool {
	ret, _ := KeyState(key)
	return ret
}

func IsMouseLeftDown() bool {
	ret, _ := KeyState(win32.VK_LBUTTON)
	return ret
}

func IsMouseRightDown() bool {
	ret, _ := KeyState(win32.VK_RBUTTON)
	return ret
}

func AllWindows() (list []*Window) {
	for _, v := range winMap {
		list = append(list, v)
	}
	return
}

func (this *Window) ShowModal(cbOnShow func()) (retParam interface{}) {
	if this.inModal {
		core.Warn("已经是模态窗口, 不能重复ShowModal")
		return nil
	}

	this.inModal = true

	if this.wt == WtChild {
		core.Warn("对子窗口调用ShowModal可能会引发混乱")
	}

	// 记住并修改祖先窗口的禁用状态
	enabledMap := make(map[*Window]bool)

	for pw := this.ParentWindow(); pw != nil; pw = pw.ParentWindow() {
		if pw.IsEnabled() {
			enabledMap[pw] = true
			pw.SetEnabled(false)
		}
		if !pw.IsVisible() {
			pw.SetVisible(true)
		}
	}

	this.SetVisible(true)

	if cbOnShow != nil {
		cbOnShow()
	}

	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover modalLoop() : %v ", e))
		}
		this.inModal = false
		retParam = this.modalRet
		this.modalRet = nil

		for pw := range enabledMap {
			pw.SetEnabled(true)
		}
		this.Close()
	}()

	// TODO: 应该过滤掉所有其他窗口的鼠标/键盘消息

	msg := win32.MSG{}
	for {
		if !win32.PeekMessage(&msg, 0, 0, 0, win32.PM_REMOVE) {
			win32.Sleep(1)
			continue
		}

		if msg.Message == win32.WM_QUIT {
			win32.PostQuitMessage(int(msg.WParam))
			break
		}

		if msg.Message == WM_END_MODAL {
			break
		}

		if msg.Hwnd == 0 {
			handleNonUIMessages(msg.Message, msg.WParam, msg.LParam)
		} else {
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		}
	}
	return
}

func (this *Window) EndModal(retParam interface{}) {
	if !this.inModal {
		return
	}
	this.modalRet = retParam
	win32.PostMessage(this.hWnd, WM_END_MODAL, 0, 0)
}

func (this *Window) SetEnabled(b bool) {
	win32.EnableWindow(this.hWnd, b)
}

func (this *Window) IsEnabled() bool {
	return win32.IsWindowEnabled(this.hWnd)
}

func (this *Window) MoveToCenter() {
	ref := this.Widget().Parent()
	var x, y, w, h float64
	if ref != nil {
		_, _, w, h = ref.Bounds()
		x, y = 0, 0
	} else {
		x, y, w, h = DesktopArea()
	}
	x1, y1, w1, h1 := this.FrameBounds()
	x1 = x + (w-w1)*0.5
	y1 = y + (h-h1)*0.5

	this.SetFrameBounds(x1, y1, w1, h1)

	this.EnsureInDesktopArea()
}

func (this *Window) EnsureInDesktopArea() {
	ref := this.Widget().Parent()

	x, y, w, h := this.FrameBounds()

	if ref != nil {
		x, y = ref.MapToGlobal(x, y)
	}

	xd, yd, wd, hd := DesktopArea()

	if x+w >= xd+wd {
		x = xd + wd - w
	}

	if y+h >= yd+hd {
		y = yd + hd - h
	}

	if x < xd {
		x = xd
	}

	if y < yd {
		y = yd
	}

	if ref != nil {
		x, y = ref.MapFromGlobal(x, y)
	}

	this.SetFrameBounds(x, y, w, h)
}

func (this *Window) IsMinimized() bool {
	style := win32.GetWindowLongPtr(this.hWnd, win32.GWL_STYLE)
	return style&win32.WS_MINIMIZE != 0
}

func (this *Window) SetMinimized(b bool) {
	if b == this.IsMinimized() {
		return
	}
	if b {
		win32.ShowWindow(this.hWnd, win32.SW_MINIMIZE)
	} else {
		win32.ShowWindow(this.hWnd, win32.SW_RESTORE)
	}
}

func (this *Window) IsMaximized() bool {
	style := win32.GetWindowLongPtr(this.hWnd, win32.GWL_STYLE)
	return style&win32.WS_MAXIMIZE != 0
}

func (this *Window) SetMaximized(b bool) {
	if b == this.IsMaximized() {
		return
	}
	if b {
		win32.ShowWindow(this.hWnd, win32.SW_MAXIMIZE)
	} else {
		win32.ShowWindow(this.hWnd, win32.SW_RESTORE)
	}
}

func (this *Window) SetPlacement(a WindowPlace) {
	this.SetVisible(true) // 确保可见
	this.SetFrameBounds1(a.FrameBounds)
	this.SetMaximized(a.Maximized)
	this.SetMinimized(a.Minimized)
}

func (this *Window) Placement() (ret WindowPlace) {
	wp, ok := win32.GetWindowPlacement(this.hWnd)
	if ok && this.Widget().Parent() == nil {
		// TODO: this.Widget().Parent() != nil 时, 应该做坐标转换
		ret.FrameBounds = geom.Rect{float64(wp.NormalPosition.Left),
			float64(wp.NormalPosition.Top),
			float64(wp.NormalPosition.Right - wp.NormalPosition.Left),
			float64(wp.NormalPosition.Bottom - wp.NormalPosition.Top)}
	} else {
		ret.FrameBounds = this.FrameBounds1()
	}
	ret.Maximized = this.IsMaximized()
	ret.Minimized = this.IsMinimized()
	return
}

func (this *Window) SetCloseOnHide(b bool) {
	this.closeOnHide = b
}

// 退出消息循环
func QuitLoop() {
	win32.PostQuitMessage(0)
}

func DbgExportGuiGv(open bool, a ...interface{}) {
	g := &gv.Graph{}
	g.SetName("GUI")
	g.AutoTypeInfo = true

	g.DefaultNode.Shape = "box"

	g.DefaultEdge.Color = "lightgray"

	for _, win := range AllWindows() {
		g.Node(win)
	}

	for _, frame := range AllFrames() {
		g.Node(frame)
	}

	for _, p := range a {
		g.Node(p)
	}

	path := core.LocalDataDir() + "/gui.gv.svg"
	err := g.GenDotOutput(path, "svg")
	if err != nil {
		core.Warn("failed to gen dot output: ", err.Error())
		return
	}

	if open {
		err = core.ShellOpen(path)
		if err != nil {
			core.Warn(`failed to open file: "`, path, `"`, err.Error())
		}
	}
}

func (this *Window) ExportGv(g *gv.Graph) {
	node := g.Node(this)
	node.Shape = "box"
	node.Color = "darkblue"
	node.TextColor = node.Color

	widget := g.Node(this.widget)

	edge := g.Edge(node, widget)
	edge.ArrowHead = "none"

}
