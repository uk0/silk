//go:build !windows

package gui

import (
	"silk/core"
	"silk/geom"
	"silk/gv"
	"silk/paint"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	DefultLineEnd = "\r\n"
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

type WindowPlace struct {
	Monitor     string
	FrameBounds geom.Rect
	Maximized   bool
	Minimized   bool
	FullScreen  bool
}

type WinId uintptr

var (
	winMap           = make(map[*glfw.Window]*Window)
	mouseHoverWidget IWidget
	lastMouseWidget  IWidget
	lastMouseTime    time.Time
	mouseMoving      bool
	focusWidget      IWidget
	activeForm       *Window
	captureStack     []IWidget
	screenDpi        float64
	shouldQuit       bool
	glfwMu           sync.Mutex

	idleTimer Timer
	idleSkip  int
	idleFlag  bool
)

type Window struct {
	glfwWin    *glfw.Window
	backBuffer paint.Pixmap
	backWidth  int
	backHeight int
	widget      IWidget
	wt          WindowType
	title       string
	glTexture   uint32
	dirty   bool
	enabled bool

	mouseEntered bool
	autoCaptured bool
	toCapture    bool
	inModal      bool
	closeOnHide  bool

	// pendingPopupPos stores the global screen position set by
	// setPopupGlobalPos before the window is shown. On macOS,
	// glfwWin.SetPos on a hidden window may not persist, so
	// we re-apply it after glfwWin.Show().
	pendingPopupPos [2]float64
	hasPendingPos   bool

	modalRet interface{}

	lastDndWidget IWidget
	dndCtx        *dndContext
}

func init() {
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		panic("failed to init GLFW: " + err.Error())
	}

	// Get DPI from primary monitor
	mon := glfw.GetPrimaryMonitor()
	if mon != nil {
		sx, _ := mon.GetContentScale()
		screenDpi = float64(sx) * 96.0
	}
	if screenDpi == 0 {
		screenDpi = 96
	}
	core.Debug("Main screen DPI: ", screenDpi)

	initCursors()

	idleTimer.Start(47, onIdleTimer)

	core.SetMainLoop(MainLoop, QuitLoop)
}

func setMouseHoverWidget(iw IWidget) {
	if mouseHoverWidget == iw {
		return
	}
	old := mouseHoverWidget
	mouseHoverWidget = iw
	if old != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					core.Warn("panic in OnMouseLeave: ", r)
				}
			}()
			if i, ok := old.(IEventMouseLeave); ok {
				i.OnMouseLeave()
			}
		}()
	}
	if iw != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					core.Warn("panic in OnMouseEnter: ", r)
				}
			}()
			if i, ok := iw.(IEventMouseEnter); ok {
				i.OnMouseEnter()
			}
		}()
	}
}

func SetCursor(p *Cursor) {
	if p == nil {
		return
	}
	p.apply()
}

func updateCursor() {
	if overrideCursor != nil {
		overrideCursor.apply()
		return
	}

	if lastMouseWidget != nil {
		// Walk up the widget tree to find the first widget with a custom cursor
		for w := lastMouseWidget; w != nil; w = w.Parent() {
			c := w.Cursor()
			if c != nil {
				c.apply()
				return
			}
		}
	}

	cursorArrow.apply()
}

func (this *Window) create(p *Window, wt WindowType) error {
	if this.glfwWin != nil {
		return core.StrErr("win seems already inited.")
	}

	if p != nil {
		core.Debug(`create "`, wt, `" for `, reflect.TypeOf(this.widget).String(),
			`, parent = `, reflect.TypeOf(p.widget).String(), ` `, unsafe.Pointer(p))
	} else {
		core.Debug(`create "`, wt, `" for `, reflect.TypeOf(this.widget).String(),
			`, parent = nil`)
	}

	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.Resizable, glfw.True)

	switch wt {
	case WtPopup:
		glfw.WindowHint(glfw.Decorated, glfw.False)
		glfw.WindowHint(glfw.Floating, glfw.True)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
	case WtForm:
		glfw.WindowHint(glfw.Decorated, glfw.True)
		glfw.WindowHint(glfw.Floating, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.True)
	case WtChild:
		glfw.WindowHint(glfw.Decorated, glfw.False)
		glfw.WindowHint(glfw.Floating, glfw.False)
	case WtInherit:
		panic("try create WtInherit Window.")
	}

	width, height := 800, 600
	rect := this.widget.Bounds1()
	if rect.Width > 0 && rect.Height > 0 {
		width = int(rect.Width)
		height = int(rect.Height)
	}

	// Share the GL context with the parent window so popup windows can render
	// using the same OpenGL state. This prevents popups from appearing blank
	// or rendering behind the main window on macOS.
	var shareWin *glfw.Window
	if p != nil {
		shareWin = p.glfwWin
	}

	gw, err := glfw.CreateWindow(width, height, "", nil, shareWin)
	if err != nil {
		return core.StrErr("glfw.CreateWindow: " + err.Error())
	}

	winMap[gw] = this
	this.glfwWin = gw
	this.wt = wt
	this.enabled = true
	this.dirty = true

	// Initialize OpenGL for this window
	gw.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		core.Warn("gl.Init: ", err)
	}
	initGL()

	// Set up callbacks
	gw.SetSizeCallback(onWindowResize)
	gw.SetPosCallback(onWindowMove)
	gw.SetCloseCallback(onWindowClose)
	gw.SetCursorPosCallback(onCursorPos)
	gw.SetMouseButtonCallback(onMouseButton)
	gw.SetScrollCallback(onScroll)
	gw.SetKeyCallback(onKey)
	gw.SetCharCallback(onChar)
	gw.SetDropCallback(onDrop)
	gw.SetCursorEnterCallback(onCursorEnter)

	if rect.Width != 0 || rect.Height != 0 {
		this.widget.SetSize(rect.Size())
	}

	// Set minimum window size for form windows
	if wt == WtForm {
		gw.SetSizeLimits(320, 240, glfw.DontCare, glfw.DontCare)
	}

	core.Debug("window create succeed.")
	return nil
}

func (this *Window) destroy() {
	if this.glTexture != 0 {
		this.glfwWin.MakeContextCurrent()
		gl.DeleteTextures(1, &this.glTexture)
		this.glTexture = 0
	}
	this.widget = nil
	if this.glfwWin != nil {
		delete(winMap, this.glfwWin)
		this.glfwWin.Destroy()
		this.glfwWin = nil
	}
}

func (this *Window) getBackBuffer(width, height int) paint.Pixmap {
	// Use the exact framebuffer dimensions instead of aligning to multiples
	// of 16. The old alignment caused a mismatch: TexImage2D received the
	// real framebuffer size (fbw x fbh) while the pixel buffer was allocated
	// at the larger aligned size, leading to row-stride / data-offset errors.
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	if width != this.backWidth || height != this.backHeight {
		this.backWidth = width
		this.backHeight = height
		this.backBuffer = paint.NewPixmap(width, height)
	}

	return this.backBuffer
}

func (this *Window) paint() {
	if this.widget == nil || this.glfwWin == nil {
		return
	}

	this.glfwWin.MakeContextCurrent()

	// Widget layout uses logical pixels (window size)
	width, height := this.widget.Size()
	if width <= 0 || height <= 0 {
		// Fallback: use window size (logical pixels) when widget has no size yet
		ww, wh := this.glfwWin.GetSize()
		if ww > 0 && wh > 0 {
			width, height = float64(ww), float64(wh)
			this.widget.NakedWidget().setSize(width, height)
		} else {
			return
		}
	}

	// Get content scale for Retina/HiDPI displays
	sx, sy := this.glfwWin.GetContentScale()
	fbw, fbh := this.glfwWin.GetFramebufferSize()

	// Reuse the back buffer when dimensions haven't changed.
	// Only allocate a new pixmap when the framebuffer size differs.
	// Clear to the form background before each paint to prevent ghost artifacts.
	if this.backBuffer == nil || this.backWidth != fbw || this.backHeight != fbh {
		this.backBuffer = paint.NewPixmap(fbw, fbh)
		this.backWidth = fbw
		this.backHeight = fbh
	}
	backBuffer := this.backBuffer
	backPainter := backBuffer.NewPainter()
	// Clear the entire surface to avoid ghost artifacts from previous frame
	backPainter.SetOperator(paint.OpClear)
	backPainter.Paint()
	backPainter.SetOperator(paint.OpOver)

	// Scale Cairo context: logical coords -> physical pixels.
	// Wrap the widget tree draw in panic recovery so a single broken widget
	// cannot crash the entire paint cycle.
	backPainter.Save()
	backPainter.Scale(float64(sx), float64(sy))
	paintStart := time.Now()
	func() {
		defer func() {
			if r := recover(); r != nil {
				core.Warn("paint panic in DrawWidgetAll: ", r)
			}
		}()
		DrawWidgetAll(this.widget, backPainter, 0, 0, 0, 0, width, height)
	}()
	GlobalPerfStats.RecordPaint(time.Since(paintStart))

	// Perf overlay: drawn after the widget tree while the logical-coord
	// scale is still in effect, so it lands in the back buffer at the
	// correct size. Only renders when visible (cheap otherwise).
	if GlobalPerfStats.IsVisible() {
		GlobalPerfStats.SetWidgetCount(CountWidgets(this.widget))
		GlobalPerfStats.Draw(backPainter, width, height)
	}

	backPainter.Restore()
	backBuffer.Flush()

	// Get raw Cairo pixel data directly (BGRA format, no copy)
	dataPtr := backBuffer.DataPtr()
	if dataPtr == nil {
		return
	}
	stride := backBuffer.Stride()

	// Upload to OpenGL texture at physical resolution
	if this.glTexture == 0 {
		gl.GenTextures(1, &this.glTexture)
	}
	gl.BindTexture(gl.TEXTURE_2D, this.glTexture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, int32(stride/4))
	// Cairo stores ARGB32 as BGRA in memory on little-endian
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		int32(this.backWidth), int32(this.backHeight),
		0, gl.BGRA, gl.UNSIGNED_BYTE, dataPtr)

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		core.Warn("GL error after TexImage2D: ", glErr)
	}

	// Ensure GL state is set up for this context
	initGL()

	// The texture may be slightly larger than the framebuffer when the
	// backbuffer dimensions were rounded up. Compute the sub-region of
	// the texture that maps to the actual framebuffer viewport.
	texU := float32(fbw) / float32(this.backWidth)
	texV := float32(fbh) / float32(this.backHeight)

	// Draw fullscreen quad at framebuffer resolution
	gl.Viewport(0, 0, int32(fbw), int32(fbh))
	drawFullscreenQuadUV(this.glTexture, int32(fbw), int32(fbh), texU, texV)

	gl.Flush()
	this.glfwWin.SwapBuffers()
	this.dirty = false
}

func (this *Window) Close() {
	core.Debug("(this *Window) Close()")
	if this.glfwWin == nil {
		return
	}

	if this.inModal {
		this.EndModal(nil)
		return
	}

	if iclose, ok := this.widget.(interface {
		Close()
	}); ok {
		iclose.Close()
	}

	this.destroy()
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
	if this.glfwWin == nil {
		return 0
	}
	return WinId(uintptr(unsafe.Pointer(this.glfwWin)))
}

func (this *Window) OnDestroy() {
	if this.glfwWin == nil {
		core.Warn("window seems already destroyed.")
		return
	}
	core.Debug(`window destroyed. title = "` + this.Title() +
		`", widget = "` + core.ObjInfo(this.widget) + `"`)
	delete(winMap, this.glfwWin)
	this.glfwWin = nil
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

func (this *Window) Bounds() (x, y, width, height float64) {
	x, y, _, _ = this.widget.Bounds()
	if this.glfwWin != nil {
		w, h := this.glfwWin.GetSize()
		width, height = float64(w), float64(h)
	} else {
		_, _, width, height = this.widget.Bounds()
	}
	return
}

func (this *Window) SetBounds(x, y, width, height float64) {
	this.SetPos(x, y)
	this.SetSize(width, height)
}

func (this *Window) SetPos(x, y float64) {
	xg, yg := x, y
	pw := this.Widget().Parent()
	if pw != nil {
		xg, yg = pw.MapToGlobal(xg, yg)
	}

	m := this.FrameMargin()
	if this.glfwWin != nil {
		// Use math.Round instead of int() truncation to avoid sub-pixel
		// jitter that causes popup menus to shift position erratically.
		this.glfwWin.SetPos(int(math.Round(xg-m.L)), int(math.Round(yg-m.T)))
	}

	// In Win32, SetWindowPos triggers WM_MOVE which calls widget.setPos().
	// GLFW has no position-change callback, so update the widget directly.
	this.widget.NakedWidget().setPos(x, y)
}

func (this *Window) FrameMargin() (ret Padding) {
	if this.wt == WtForm && this.glfwWin != nil {
		l, t, r, b := this.glfwWin.GetFrameSize()
		ret.L = float64(l)
		ret.T = float64(t)
		ret.R = float64(r)
		ret.B = float64(b)
	}
	return
}

func (this *Window) SetSize(w, h float64) {
	if this.glfwWin != nil {
		this.glfwWin.SetSize(int(w), int(h))
		this.dirty = true
	}
}

// setPopupGlobalPos positions a popup widget at global screen coordinates,
// directly setting the GLFW window position without going through the
// widget coordinate chain (which caused double-conversion bugs).
//
// On macOS, glfwSetWindowPos on a hidden window is ignored by the window
// manager. We store the position and apply it inside setVisible(true)
// using a zero-flicker strategy: size the window to 0x0 before Show,
// then resize + reposition after Show in the same frame.
func setPopupGlobalPos(popup IWidget, gx, gy float64) {
	win := popup.Window()
	if win != nil && win.NakedWindow().glfwWin != nil {
		nw := win.NakedWindow()
		nw.pendingPopupPos = [2]float64{gx, gy}
		nw.hasPendingPos = true
		// Also try setting directly (works if window is already visible)
		nw.glfwWin.SetPos(int(math.Round(gx)), int(math.Round(gy)))
	} else {
		// Fallback for non-windowed popups
		pp := popup.Parent()
		if pp != nil {
			gx, gy = pp.MapFromGlobal(gx, gy)
		}
		popup.SetPos(gx, gy)
	}
}

func (this *Window) Update() {
	this.dirty = true
}

func (this *Window) UpdateRect(x, y, width, height float64) {
	this.dirty = true
}

func (this *Window) mapFromGlobal(x, y float64) (x1, y1 float64) {
	if this.glfwWin == nil {
		return x, y
	}
	// GLFW GetPos() returns the content area position (below title bar,
	// inside window borders), so widget coordinates map directly to
	// screen coordinates without adding frame decoration offsets.
	wx, wy := this.glfwWin.GetPos()
	x1 = x - float64(wx)
	y1 = y - float64(wy)
	return
}

func (this *Window) mapToGlobal(x, y float64) (x1, y1 float64) {
	if this.glfwWin == nil {
		return x, y
	}
	// GLFW GetPos() returns the content area position (below title bar,
	// inside window borders). Widget (0,0) = content area top-left,
	// which is already at screen (wx, wy). No frame margin needed.
	wx, wy := this.glfwWin.GetPos()
	x1 = x + float64(wx)
	y1 = y + float64(wy)
	return
}

func (this *Window) IsValid() bool {
	return this.glfwWin != nil
}

func (this *Window) Title() string {
	return this.title
}

func (this *Window) SetTitle(s string) {
	this.title = s
	if this.glfwWin != nil {
		this.glfwWin.SetTitle(s)
	}
}

func (this *Window) SetIcon(icon paint.Icon) {
	// GLFW icon setting is limited; skip for now
}

func (this *Window) updateTitle() {
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
	cap := curCapture()
	if cap == nil {
		return nil
	}
	return cap.OwnerWindow()
}

func releaseCapture() {
	// GLFW doesn't have explicit capture; managed via captureStack
}

func setCapture(iw IWidget) {
	lastMouseWidget = iw
}

func curCapture() IWidget {
	n := len(captureStack)
	if n == 0 {
		return nil
	}
	return captureStack[n-1]
}

func pushCapture(w IWidget) bool {
	cur := curCapture()
	if cur == w {
		return false
	}
	if cur != nil {
		for p := w.Parent(); p != nil; p = p.Parent() {
			if p == cur {
				captureStack = append(captureStack, w)
				setCapture(w)
				return true
			}
		}
	}
	captureStack = append(captureStack[:0], w)
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
			captureStack = captureStack[:n+1]
			setCapture(p)
			return
		}
	}
	captureStack = captureStack[:0]
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
	if this.glfwWin == nil {
		return false
	}
	return this.glfwWin.GetAttrib(glfw.Visible) == glfw.True
}

func (this *Window) setVisible(b bool) {
	if this.glfwWin == nil {
		return
	}
	if b == this.IsVisible() {
		return
	}
	if b {
		if this.wt == WtPopup && this.hasPendingPos {
			// ── Popup zero-flicker strategy ──
			// macOS ignores SetPos on hidden GLFW windows. To avoid the
			// popup flashing at a wrong position then jumping:
			// 1. Shrink to 1x1 (invisible even if shown at wrong coords)
			// 2. Show the 1x1 window (macOS accepts it)
			// 3. SetPos to correct location (works on visible window)
			// 4. SetSize to actual dimensions
			// All four calls happen in the same frame — no visible flicker.
			_, _, ww, wh := 0.0, 0.0, 0.0, 0.0
			if this.widget != nil {
				_, _, ww, wh = this.widget.Bounds()
			}
			this.glfwWin.SetSize(1, 1)
			this.glfwWin.Show()
			this.glfwWin.SetPos(
				int(math.Round(this.pendingPopupPos[0])),
				int(math.Round(this.pendingPopupPos[1])))
			if ww > 0 && wh > 0 {
				this.glfwWin.SetSize(int(ww), int(wh))
			}
			this.hasPendingPos = false
		} else {
			// ── Normal window show ──
			if this.widget != nil {
				wx, wy, ww, wh := this.widget.Bounds()
				if ww > 0 && wh > 0 {
					this.glfwWin.SetSize(int(ww), int(wh))
				}
				if this.wt != WtPopup {
					xg, yg := wx, wy
					pw := this.widget.Parent()
					if pw != nil {
						xg, yg = pw.MapToGlobal(xg, yg)
					}
					this.glfwWin.SetPos(int(math.Round(xg)), int(math.Round(yg)))
				}
			}
			this.glfwWin.Show()
		}
		this.dirty = true
	} else {
		this.glfwWin.Hide()
		if this.closeOnHide {
			this.destroy()
		}
	}
}

func (this *Window) SetVisible(b bool) {
	this.widget.SetVisible(b)
}

func (this *Window) IsActive() bool {
	return activeForm == this
}

func (this *Window) Widget() IWidget {
	return this.widget
}

func (this *Window) NakedWindow() *Window {
	return this
}

func MousePosition() (x, y float64) {
	for gw, win := range winMap {
		if win.IsVisible() {
			cx, cy := gw.GetCursorPos()
			wx, wy := gw.GetPos()
			m := win.FrameMargin()
			return float64(wx) + m.L + cx, float64(wy) + m.T + cy
		}
	}
	return 0, 0
}

func AllWindows() (list []*Window) {
	for _, v := range winMap {
		list = append(list, v)
	}
	return
}

func FindTopWindow(xg, yg float64) *Window {
	// Find topmost window containing (xg, yg).
	// Popup windows are on top, so check them first.
	var fallback *Window
	for _, win := range winMap {
		if !win.IsVisible() || win.glfwWin == nil {
			continue
		}
		wx, wy := win.glfwWin.GetPos()
		m := win.FrameMargin()
		ww, wh := win.widget.Size()
		lx := float64(wx) + m.L
		ly := float64(wy) + m.T
		if xg >= lx && yg >= ly && xg < lx+ww && yg < ly+wh {
			if win.wt == WtPopup {
				return win // Popup windows have highest priority
			}
			if fallback == nil {
				fallback = win
			}
		}
	}
	return fallback
}

func AnyWindowId() (ret WinId) {
	for gw, win := range winMap {
		ret = WinId(uintptr(unsafe.Pointer(gw)))
		if win.wt == WtForm {
			return
		}
	}
	return
}

func KeyState(key int) (down, checked bool) {
	for gw := range winMap {
		glfwKey := vkToGLFWKey(key)
		if glfwKey != glfw.KeyUnknown {
			state := gw.GetKey(glfwKey)
			down = state == glfw.Press
		}
		break
	}
	return
}

func IsKeyDown(key int) bool {
	ret, _ := KeyState(key)
	return ret
}

func IsMouseLeftDown() bool {
	for gw := range winMap {
		return gw.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press
	}
	return false
}

func IsMouseRightDown() bool {
	for gw := range winMap {
		return gw.GetMouseButton(glfw.MouseButtonRight) == glfw.Press
	}
	return false
}

func HideConsoleWindow() {
	// No-op on non-Windows
}

func ShowConsoleWindow() {
	// No-op on non-Windows
}

func DesktopArea() (x, y, w, h float64) {
	mon := glfw.GetPrimaryMonitor()
	if mon != nil {
		vm := mon.GetVideoMode()
		if vm != nil {
			// On macOS, GetVideoMode already returns screen coordinates
			// (logical points), NOT physical pixels. Dividing by content
			// scale would halve the desktop area on Retina displays,
			// causing popup boundary clamping to push menus to wrong
			// positions. Use the values directly.
			return 0, 0, float64(vm.Width), float64(vm.Height)
		}
	}
	return 0, 0, 1920, 1080
}

func ScreenDpi() float64 {
	return screenDpi
}

func ScreenDpmm() float64 {
	return screenDpi / 25.4
}

func PixelToMm(pixelLen float64) (mmLen float64) {
	mmLen = 25.4 * pixelLen / screenDpi
	return
}

func MmToPixel(mmLen float64) (pixelLen float64) {
	pixelLen = mmLen * screenDpi / 25.4
	return
}

func MmToPixelZ(mmLen float64) (pixelLen float64) {
	a := mmLen * screenDpi / 25.4
	b := paint.Round(a)
	return b
}

// markAllWindowsDirty flags every live window as needing a repaint. Used by
// the perf-overlay toggle so F12 shows/hides the overlay immediately even
// when the UI is otherwise idle.
func markAllWindowsDirty() {
	for _, w := range winMap {
		if w == nil {
			continue
		}
		w.dirty = true
	}
	// Wake the event loop so the repaint happens promptly.
	glfw.PostEmptyEvent()
}

// MainLoop runs the GLFW event loop.
// Uses WaitEventsTimeout for a ~60fps cap that sleeps when idle, saving CPU.
func MainLoop() {
	core.Debug("MainLoop()")
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover MainLoop() : %v ", e))
		}
	}()

	if len(winMap) == 0 {
		core.Warn("no windows.")
		return
	}

	for !shouldQuit {
		// Wait for events with a timeout of ~16ms (60fps cap).
		// This replaces PollEvents+Sleep(4ms) -- it blocks when idle
		// (saving CPU) and wakes immediately when events arrive.
		glfw.WaitEventsTimeout(1.0 / 60.0)
		processTimers()

		// When the perf overlay is visible we request a repaint every
		// iteration so the FPS counter stays live even on an otherwise
		// idle UI.
		if GlobalPerfStats.IsVisible() {
			for _, win := range winMap {
				if win != nil && win.IsVisible() {
					win.dirty = true
				}
			}
		}

		painted := false
		// Repaint dirty windows: non-popup windows first, then popup windows.
		// This ensures popups always render on top of the main window.
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt != WtPopup {
				win.paint()
				painted = true
			}
		}
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt == WtPopup {
				win.paint()
				painted = true
			}
		}

		// Only count frames that actually redrew something, so FPS reflects
		// real rendering cost rather than event-loop wakeups.
		if painted {
			GlobalPerfStats.RecordFrame()
		}

		if len(winMap) == 0 {
			break
		}
	}

	glfw.Terminate()
}

func QuitLoop() {
	shouldQuit = true
}

func emulateMouseDown(left bool) {
	// Simplified emulation for GLFW: find the window under mouse and send event
	mx, my := MousePosition()
	win := FindTopWindow(mx, my)
	if win == nil || win.glfwWin == nil {
		return
	}
	x, y := win.mapFromGlobal(mx, my)
	if left {
		onMouseButton(win.glfwWin, glfw.MouseButtonLeft, glfw.Press, 0)
		_ = x
		_ = y
	} else {
		onMouseButton(win.glfwWin, glfw.MouseButtonRight, glfw.Press, 0)
	}
}

// ---- GLFW Callbacks ----

func onWindowResize(gw *glfw.Window, width, height int) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onWindowResize: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	win.widget.NakedWidget().setSize(float64(width), float64(height))
	win.dirty = true
}

func onWindowMove(gw *glfw.Window, xpos, ypos int) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onWindowMove: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	// Convert screen position back to widget-local coordinates,
	// mirroring the Windows WM_MOVE handler.
	m := win.FrameMargin()
	xg := float64(xpos) + m.L
	yg := float64(ypos) + m.T

	var x, y float64
	pw := win.Widget().Parent()
	if pw != nil {
		x, y = pw.MapFromGlobal(xg, yg)
	} else {
		x, y = xg, yg
	}
	win.widget.NakedWidget().setPos(x, y)
}

func onWindowClose(gw *glfw.Window) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onWindowClose: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil {
		return
	}
	core.Debug(`Receive "Close Window" command from system.`)

	// Prevent GLFW from hiding the window before we handle the close event
	gw.SetShouldClose(false)

	if PromptSaveClose(win.widget, win) {
		// If this was the main form (or the last form), quit the application
		formCount := 0
		for _, w := range winMap {
			if w.wt == WtForm {
				formCount++
			}
		}
		if formCount <= 1 {
			core.Quit()
		}
	}
}

func onMouseButton(gw *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onMouseButton: ", r)
		}
	}()
	// Skip mouse handling during DnD loop to prevent re-entrant item creation
	if dndActive {
		return
	}
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	if !win.enabled {
		return
	}

	cx, cy := gw.GetCursorPos()
	x, y := cx, cy

	// Cross-window capture: when a captured widget lives on a different
	// GLFW window, translate coordinates and deliver to that window instead.
	capWidget := curCapture()
	capWin := getCaptureWin()
	if capWidget != nil && capWin != nil && capWin != win {
		mx, my := win.mapToGlobal(x, y)
		tx, ty := capWin.mapFromGlobal(mx, my)
		deliverCapturedMouseButton(capWin, button, action, tx, ty)
		return
	}

	if action == glfw.Press {
		// Set as active form
		if win.wt == WtForm {
			activeForm = win
		}

		switch button {
		case glfw.MouseButtonLeft:
			lastMouseTime = time.Now()
			win.toCapture = true
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
						win.autoCaptured = pushCapture(lastMouseWidget)
						win.toCapture = false
					}
				}
				if i, ok := lastMouseWidget.(IEventLeftDown); ok {
					i.OnLeftDown(x1, y1)
				}
			}
		case glfw.MouseButtonRight:
			lastMouseTime = time.Now()
			win.toCapture = true
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
						win.autoCaptured = pushCapture(lastMouseWidget)
						win.toCapture = false
					}
				}
				if i, ok := lastMouseWidget.(IEventRightDown); ok {
					i.OnRightDown(x1, y1)
				}
			}
		}
	} else if action == glfw.Release {
		switch button {
		case glfw.MouseButtonLeft:
			lastMouseTime = time.Now()
			if lastMouseWidget != nil {
				cw := lastMouseWidget
				if i, ok := lastMouseWidget.(IEventLeftUp); ok {
					x1, y1 := lastMouseWidget.MapFromWindow(x, y)
					i.OnLeftUp(x1, y1)
				}
				if win.autoCaptured {
					popCapture(cw)
				}
			}
			win.autoCaptured = false
			win.toCapture = false
		case glfw.MouseButtonRight:
			lastMouseTime = time.Now()
			if lastMouseWidget != nil {
				cw := lastMouseWidget
				if i, ok := lastMouseWidget.(IEventRightUp); ok {
					x1, y1 := lastMouseWidget.MapFromWindow(x, y)
					i.OnRightUp(x1, y1)
				}
				if win.autoCaptured {
					popCapture(cw)
				}
			}
			win.autoCaptured = false
			win.toCapture = false
		}
	}
}

// deliverCapturedMouseButton delivers a mouse button event to the captured
// widget's owner window, using translated coordinates.
// For menu popups, we find the actual widget under the cursor rather than
// relying on lastMouseWidget, which might be the menu container instead of
// the specific button the user clicked.
func deliverCapturedMouseButton(capWin *Window, button glfw.MouseButton, action glfw.Action, winX, winY float64) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in deliverCapturedMouseButton: ", r)
		}
	}()

	if capWin == nil || capWin.widget == nil {
		return
	}

	// Find the actual widget under the cursor in the capture window.
	// This is critical for menus: lastMouseWidget may point to the menu
	// container, but the user clicked a specific button inside it.
	target := lastMouseWidget
	if capWin.widget != nil {
		found := capWin.widget.FindWidgetAt(winX, winY)
		if found != nil {
			target = found
		}
	}
	if target == nil {
		return
	}

	if action == glfw.Press {
		lastMouseWidget = target
		switch button {
		case glfw.MouseButtonLeft:
			lastMouseTime = time.Now()
			x1, y1 := target.MapFromWindow(winX, winY)
			if i, ok := target.(IEventLeftDown); ok {
				i.OnLeftDown(x1, y1)
			}
		case glfw.MouseButtonRight:
			lastMouseTime = time.Now()
			x1, y1 := target.MapFromWindow(winX, winY)
			if i, ok := target.(IEventRightDown); ok {
				i.OnRightDown(x1, y1)
			}
		}
	} else if action == glfw.Release {
		switch button {
		case glfw.MouseButtonLeft:
			lastMouseTime = time.Now()
			cw := target
			x1, y1 := target.MapFromWindow(winX, winY)
			if i, ok := cw.(IEventLeftUp); ok {
				i.OnLeftUp(x1, y1)
			}
			if capWin.autoCaptured {
				popCapture(cw)
			}
			capWin.autoCaptured = false
			capWin.toCapture = false
		case glfw.MouseButtonRight:
			lastMouseTime = time.Now()
			cw := target
			x1, y1 := target.MapFromWindow(winX, winY)
			if i, ok := cw.(IEventRightUp); ok {
				i.OnRightUp(x1, y1)
			}
			if capWin.autoCaptured {
				popCapture(cw)
			}
			capWin.autoCaptured = false
			capWin.toCapture = false
		}
	}
}

func onCursorPos(gw *glfw.Window, xpos, ypos float64) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onCursorPos: ", r)
		}
	}()
	if dndActive {
		return
	}
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	if !win.enabled {
		return
	}

	lastMouseTime = time.Now()
	mouseMoving = true

	x, y := xpos, ypos

	if win.toCapture {
		win.toCapture = false
		if lastMouseWidget != nil {
			win.autoCaptured = pushCapture(lastMouseWidget)
		}
	}

	_, _, ww, wh := win.widget.Bounds()
	inClient := x >= 0 && y >= 0 && x < ww && y < wh
	win.mouseEntered = inClient

	capWidget := curCapture()
	capWin := getCaptureWin()

	// Cross-window capture: when capture is active but the cursor is on a
	// different GLFW window, translate coordinates via global position and
	// deliver the event to the captured widget's window instead.
	if capWidget != nil && capWin != nil && capWin != win {
		mx, my := win.mapToGlobal(x, y)
		cx, cy := capWin.mapFromGlobal(mx, my)
		deliverCapturedMouseMove(capWidget, capWin, cx, cy)
		updateCursor()
		return
	}

	if capWin == win && lastMouseWidget != nil {
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
				i.OnMouseMove(x1, y1)
			}
		}
	} else if inClient {
		p := win.widget.FindWidgetAt(x, y)
		setMouseHoverWidget(p)
		lastMouseWidget = p
		if p != nil {
			if i, ok := p.Self().(IEventMouseMove); ok {
				x1, y1 := p.MapFromWindow(x, y)
				i.OnMouseMove(x1, y1)
			}
		}
	}
	updateCursor()
}

// deliverCapturedMouseMove delivers a mouse-move event to the captured widget
// using window-local coordinates of the capture widget's owner window.
func deliverCapturedMouseMove(capWidget IWidget, capWin *Window, winX, winY float64) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in deliverCapturedMouseMove: ", r)
		}
	}()

	if capWidget == nil || capWin == nil || capWin.widget == nil {
		return
	}

	p := lastMouseWidget
	if p == nil {
		p = capWidget
	}
	if p == nil {
		return
	}
	_, _, pw, ph := p.Bounds()
	x1, y1 := p.MapFromWindow(winX, winY)

	widget := p.NakedWidget()
	if widget == nil {
		return
	}
	redirect := false
	if widget.child != nil && capWidget == lastMouseWidget &&
		!IsMouseLeftDown() && !IsMouseRightDown() {
		p1 := widget.FindWidgetAt(x1, y1)
		if p1 != nil && p1 != widget {
			cx1, cy1 := p1.MapFromWindow(winX, winY)
			setMouseHoverWidget(p1)
			if i, ok := p1.(IEventMouseMove); ok {
				i.OnMouseMove(cx1, cy1)
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
			i.OnMouseMove(x1, y1)
		}
	}
}

func onScroll(gw *glfw.Window, xoff, yoff float64) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onScroll: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	if !win.enabled {
		return
	}

	lastMouseTime = time.Now()
	cx, cy := gw.GetCursorPos()

	// Deliver to captured widget first, then widget under cursor
	target := curCapture()
	if target == nil {
		target = lastMouseWidget
	}
	if target == nil {
		target = win.widget.FindWidgetAt(cx, cy)
	}

	// Walk up the widget tree to find a widget that handles mouse wheel
	for w := target; w != nil; w = w.Parent() {
		if i, ok := w.(IEventMouseWheel); ok {
			x1, y1 := w.MapFromWindow(cx, cy)
			i.OnMouseWheel(x1, y1, yoff)
			return
		}
	}
}

func onKey(gw *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onKey: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil {
		return
	}
	if !win.enabled {
		return
	}

	vk := translateKey(key, mods)
	if vk == 0 {
		return
	}

	// Global F12: toggle the perf-stats overlay. Handled at the window
	// layer so it works regardless of which widget has focus (otherwise a
	// focused CodeEditor would consume the key). Only fires on press.
	if vk == KeyF12 && action == glfw.Press {
		GlobalPerfStats.Toggle()
		return
	}

	// Determine the target widget: use focusWidget if set, otherwise
	// fall back to the root widget of the window receiving the key event.
	target := focusWidget
	if target == nil {
		target = win.widget
	}
	if target == nil {
		return
	}

	switch action {
	case glfw.Press:
		if i, ok := target.(IEventKeyDown); ok {
			i.OnKeyDown(vk, false)
		}
	case glfw.Repeat:
		if i, ok := target.(IEventKeyDown); ok {
			i.OnKeyDown(vk, true)
		}
	case glfw.Release:
		if i, ok := target.(IEventKeyUp); ok {
			i.OnKeyUp(vk)
		}
	}
}

func onChar(gw *glfw.Window, char rune) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onChar: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil {
		return
	}
	if !win.enabled {
		return
	}

	target := focusWidget
	if target == nil {
		target = win.widget
	}
	if target == nil {
		return
	}

	if it, ok := target.(IEventTextInput); ok {
		if !unicode.IsControl(char) {
			it.OnTextInput(string(char))
		}
	}
}

func onCursorEnter(gw *glfw.Window, entered bool) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onCursorEnter: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	if !entered {
		win.mouseEntered = false
		// When a capture is active, do NOT clear the hover widget on window
		// leave. GLFW fires cursor-leave when the pointer moves to a popup
		// window, but the captured widget still needs hover state to work
		// properly. Without this guard, the leave event breaks the menu
		// capture chain and causes a show/hide bounce loop.
		if curCapture() == nil {
			setMouseHoverWidget(nil)
		}
	} else {
		win.mouseEntered = true
	}
	updateCursor()
}

func onDrop(gw *glfw.Window, names []string) {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onDrop: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}

	cx, cy := gw.GetCursorPos()
	widget := win.widget.FindWidgetAt(cx, cy)
	if widget == nil {
		return
	}

	ctx := &dndContext{
		pa:      DndCopy,
		action:  DndCopy,
		formats: []string{"text/uri-list"},
		data:    map[string]interface{}{"text/uri-list": names},
	}

	for w := widget; w != nil; w = w.Parent() {
		if i, ok := w.(IOnDrop); ok {
			x, y := w.MapFromWindow(cx, cy)
			i.OnDragEnter(x, y, ctx)
			i.OnDrop(x, y, ctx)
			if ctx.Action() != DndIgnore {
				return
			}
		}
	}
}

// ---- Idle / Mouse timer ----

func onMouseTimer() {
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onMouseTimer: ", r)
		}
	}()
	if !mouseMoving {
		return
	}
	d := time.Since(lastMouseTime)
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
			i.OnMouseStop(x1, y1)
		}
	}
}

func onIdle() {
	idleFlag = false
	idleSkip = 0
	for _, win := range winMap {
		func() {
			defer func() {
				if r := recover(); r != nil {
					core.Warn("panic in OnIdle: ", r)
				}
			}()
			win.OnIdle()
		}()
	}
}

func onIdleTimer() {
	onMouseTimer()

	idleSkip++
	if idleSkip >= 6 {
		idleSkip = 0
		onIdle()
		// Only mark windows dirty if there are active animations or
		// blinking cursors that need periodic repainting.
		// Previously this unconditionally dirtied ALL windows every
		// 6 ticks (~100ms), causing constant full repaints even when
		// nothing changed — a major source of UI stutter.
		if HasActiveAnimations() || idleFlag {
			for _, win := range winMap {
				if win.IsVisible() {
					win.dirty = true
				}
			}
		}
	}
}

// ---- Modal ----

func (this *Window) ShowModal(cbOnShow func()) (retParam interface{}) {
	if this.inModal {
		core.Warn("already modal, cannot ShowModal again")
		return nil
	}

	this.inModal = true

	if this.wt == WtChild {
		core.Warn("ShowModal on child window may cause confusion")
	}

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

	for this.inModal && !shouldQuit {
		glfw.WaitEventsTimeout(1.0 / 60.0)
		processTimers()

		// Paint non-popup windows first, then popups on top
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt != WtPopup {
				win.paint()
			}
		}
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt == WtPopup {
				win.paint()
			}
		}
	}
	return
}

func (this *Window) EndModal(retParam interface{}) {
	if !this.inModal {
		return
	}
	this.modalRet = retParam
	this.inModal = false
}

func (this *Window) SetEnabled(b bool) {
	this.enabled = b
}

func (this *Window) IsEnabled() bool {
	return this.enabled
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
	_, _, w1, h1 := this.FrameBounds()

	x1 := x + (w-w1)*0.5
	y1 := y + (h-h1)*0.5

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
	if this.glfwWin == nil {
		return false
	}
	return this.glfwWin.GetAttrib(glfw.Iconified) == glfw.True
}

func (this *Window) SetMinimized(b bool) {
	if this.glfwWin == nil {
		return
	}
	if b == this.IsMinimized() {
		return
	}
	if b {
		this.glfwWin.Iconify()
	} else {
		this.glfwWin.Restore()
	}
}

func (this *Window) IsMaximized() bool {
	if this.glfwWin == nil {
		return false
	}
	return this.glfwWin.GetAttrib(glfw.Maximized) == glfw.True
}

func (this *Window) SetMaximized(b bool) {
	if this.glfwWin == nil {
		return
	}
	if b == this.IsMaximized() {
		return
	}
	if b {
		this.glfwWin.Maximize()
	} else {
		this.glfwWin.Restore()
	}
}

func (this *Window) SetPlacement(a WindowPlace) {
	this.SetVisible(true)
	this.SetFrameBounds1(a.FrameBounds)
	this.SetMaximized(a.Maximized)
	this.SetMinimized(a.Minimized)
}

func (this *Window) Placement() (ret WindowPlace) {
	ret.FrameBounds = this.FrameBounds1()
	ret.Maximized = this.IsMaximized()
	ret.Minimized = this.IsMinimized()
	return
}

func (this *Window) SetCloseOnHide(b bool) {
	this.closeOnHide = b
}

// ---- GV export ----

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

// vkToGLFWKey maps a VK code back to GLFW key for KeyState queries
func vkToGLFWKey(vk int) glfw.Key {
	for gk, v := range glfwKeyToVK {
		if v == vk {
			return gk
		}
	}
	return glfw.KeyUnknown
}
