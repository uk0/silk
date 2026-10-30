//go:build !windows

package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gv"
	"github.com/uk0/silk/paint"
	"math"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
	screenDpi        float64 = 96
	glfwReady        bool
	glfwInitErr      error
	shouldQuit       bool
	glfwMu           sync.Mutex

	idleTimer Timer
	idleSkip  int
	idleFlag  bool
)

type Window struct {
	glfwWin                *glfw.Window
	nativeKeyCleanup       func()
	transparentFramebuffer bool // actual native capability, not only a requested hint
	backBuffer             paint.Pixmap
	// backPainter is reused across paint cycles so we don't pay the
	// allocation + cairo_create cost every frame. It must be invalidated
	// whenever backBuffer is reallocated (e.g. on resize) — see paint().
	backPainter       paint.Painter
	painting          bool
	destroyAfterPaint bool
	backWidth         int
	backHeight        int
	widget            IWidget
	wt                WindowType
	title             string
	icon              paint.Icon
	glTexture         uint32
	dirty             bool
	enabled           bool

	// Dirty region tracking. fullDirty=true (the default) means the next
	// paint redraws the whole window. dirtyRegion accumulates partial
	// invalidations in window-root logical coordinates; once the user opts
	// into partial repaints by calling MarkDirtyRect instead of Update,
	// the paint pass will use Cairo's clip rect to skip unchanged pixels.
	dirtyRegion geom.Rect
	fullDirty   bool
	framePacer  framePacer
	iconified   bool

	// PBO (Pixel Buffer Object) state for async texture streaming. The PBO
	// is sized to match the upload region; uploadTextureViaPBO orphans + remaps
	// it each frame so the driver can DMA the previous frame's data while
	// the CPU writes the next.
	pbo           uint32
	pboSize       int
	textureWidth  int32
	textureHeight int32
	pboAvailable  bool

	// glOverlays holds per-frame GPU chart-series draw requests. Populated by
	// GPU-mode LineCharts during the Cairo pass and drawn after the texture
	// blit (see gl_overlay.go / gl_overlay_glfw.go).
	glOverlays       []chartOverlay
	sceneComposition sceneCompositionWindow

	pointer      pointerEventState
	mouseEntered bool
	autoCaptured bool
	toCapture    bool
	inModal      bool
	closeOnHide  bool
	// GLFW's shared GL context is not a native owner relationship. Keep the
	// actual popup owner so transient windows cannot outlive its geometry or
	// visibility, and remember logical focus for keyboard routing on dismiss.
	popupOwner             *Window
	popupRestoreFocus      IWidget
	popupFocusCheckPending bool

	// pendingPopupPos stores the global screen position set by
	// setPopupGlobalPos before the window is shown. On macOS,
	// glfwWin.SetPos on a hidden window may not persist, so
	// we re-apply it after glfwWin.Show().
	pendingPopupPos [2]float64
	hasPendingPos   bool

	modalRet interface{}
	modalErr error

	lastDndWidget IWidget
	dndCtx        *dndContext
}

var glfwWakeup = nativeWakeup{fn: glfw.PostEmptyEvent}

func init() {
	runtime.LockOSThread()

	// Only meaningful after LockOSThread: this is now the one thread the event
	// loop will ever run on. Off-thread widget mutation is silent corruption in
	// this framework, so under SILK_DEBUG name the offender (see uithread.go).
	// window_windows.go installs its own for the same reason; without one here
	// the detector was inert on macOS and Linux.
	uiThreadId = currentThreadId()
	uiThreadOwner = func() bool { return currentThreadId() == uiThreadId }

	// Importing the SDK must not require a desktop: model construction and
	// persistence are also used by generators, servers and headless tests.
	// Keep the backend registered; an actual native-window/loop request below
	// reports the initialization error instead of pretending to run a GUI.
	core.SetMainLoop(MainLoop, QuitLoop)
	if runtime.GOOS == "darwin" {
		// GLFW otherwise chdirs into an app bundle's Contents/Resources,
		// silently replacing the caller's workspace with bundled resources.
		glfw.InitHint(glfw.CocoaChdirResources, glfw.False)
	}
	screenDpi, glfwInitErr = probeGLFWDesktop(glfw.Init, func() float64 {
		if mon := glfw.GetPrimaryMonitor(); mon != nil {
			sx, _ := mon.GetContentScale()
			return float64(sx)
		}
		return 1
	})
	glfwReady = glfwInitErr == nil
	// A successful GLFW probe does not mean Cocoa's event queue is ready.
	// Off-thread PostEmptyEvent before the first native window can SIGTRAP
	// inside AppKit. Keep model-only / pre-window Post purely in Go; create()
	// installs the wake hook after a real window has initialized the backend.
	core.Debug("Main screen DPI: ", screenDpi)

	initCursors()

	idleTimer.Start(47, onIdleTimer)
}

// The Go GLFW binding can log a platform failure and return nil from Init,
// then panic with *glfw.Error on the first monitor query. Recover only GLFW
// errors around this initial probe, never arbitrary application/runtime bugs.
func probeGLFWDesktop(initialize func() error, contentScale func() float64) (dpi float64, err error) {
	dpi = 96
	defer func() {
		if value := recover(); value != nil {
			if glfwErr, ok := value.(*glfw.Error); ok {
				err = fmt.Errorf("GLFW desktop initialization: %w", glfwErr)
			} else {
				panic(value)
			}
		}
	}()
	if err = initialize(); err != nil {
		return dpi, fmt.Errorf("GLFW desktop initialization: %w", err)
	}
	if scale := contentScale(); scale > 0 && !math.IsNaN(scale) && !math.IsInf(scale, 0) {
		dpi = scale * 96
	}
	return dpi, nil
}

func nativeDesktopError(operation string) error {
	if glfwReady {
		return nil
	}
	cause := glfwInitErr
	if cause == nil {
		cause = core.StrErr("GLFW is not initialized")
	}
	return fmt.Errorf("gui: %s requires a native desktop (on Linux configure DISPLAY/X11 or Xvfb): %w", operation, cause)
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
	if err := nativeDesktopError("Window.create"); err != nil {
		// AttachWindow has no error result in the public API; its caller drops
		// create errors, so also report this native request explicitly.
		core.Warn(err)
		return err
	}
	if this.glfwWin != nil {
		return core.StrErr("win seems already inited.")
	}
	if wt == WtPopup {
		this.popupOwner = p
		if p == this {
			this.popupOwner = nil
			if parent := this.widget.Parent(); parent != nil {
				this.popupOwner = parent.OwnerWindow()
			}
		}
		this.popupRestoreFocus = focusTargetForOwner(this.popupOwner, focusWidget)
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
	// Hints are global: explicitly disable alpha again for forms/children.
	glfw.WindowHint(glfw.TransparentFramebuffer, popupFramebufferHint(wt))

	switch wt {
	case WtPopup:
		glfw.WindowHint(glfw.Decorated, glfw.False)
		if runtime.GOOS == "darwin" {
			// Cocoa child ordering, not global floating-window level, keeps
			// menus above their owner without pinning them over other apps.
			glfw.WindowHint(glfw.Floating, glfw.False)
		} else {
			glfw.WindowHint(glfw.Floating, glfw.True)
		}
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
	this.transparentFramebuffer = wt == WtPopup && gw.GetAttrib(glfw.TransparentFramebuffer) == glfw.True
	this.wt = wt
	this.enabled = true
	this.dirty = true

	// Initialize OpenGL for this window
	gw.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		core.Warn("gl.Init: ", err)
	}
	initGL()

	// Enable vsync so SwapBuffers blocks until the display retrace.
	// On 60Hz this caps painting at 60fps; on ProMotion / 120Hz / 144Hz
	// monitors it adapts to the display's actual refresh rate. Combined
	// with the WaitEvents-based MainLoop this gives smooth pacing
	// without burning CPU between frames.
	glfw.SwapInterval(1)

	// Set up callbacks
	gw.SetSizeCallback(onWindowResize)
	gw.SetPosCallback(onWindowMove)
	gw.SetIconifyCallback(onWindowIconify)
	gw.SetFocusCallback(onWindowFocus)
	gw.SetCloseCallback(onWindowClose)
	gw.SetCursorPosCallback(onCursorPos)
	gw.SetMouseButtonCallback(onMouseButton)
	gw.SetScrollCallback(onScroll)
	gw.SetKeyCallback(onKey)
	gw.SetCharCallback(onChar)
	gw.SetDropCallback(onDrop)
	gw.SetCursorEnterCallback(onCursorEnter)
	this.nativeKeyCleanup = installNativeTabNavigation(gw)

	if rect.Width != 0 || rect.Height != 0 {
		this.widget.SetSize(rect.Size())
	}

	// Set minimum window size for form windows
	if wt == WtForm {
		gw.SetSizeLimits(320, 240, glfw.DontCare, glfw.DontCare)
	}

	SetUIWakeup(glfwWakeup.wake) // also wakes tasks queued before this window
	core.Debug("window create succeed.")
	return nil
}

func (this *Window) destroy() {
	if this.painting {
		// Draw callbacks may close their owner. Its current painter and GL
		// context stay alive until the frame unwinds; never free them in Draw.
		this.destroyAfterPaint = true
		return
	}
	closeWindowSceneGPUDevice(this)
	this.sceneComposition.layers = nil
	this.releaseNativeKeyRouting()
	this.dismissOwnedPopups()
	dismissOwnedToasts(this.widget)
	if this.wt == WtPopup && this.glfwWin != nil {
		unbindNativePopupOwner(this.glfwWin)
	}
	this.clearWindowInput()
	if this.widget != nil && this.widget.NakedWidget().win == this {
		// closeOnHide popups may be shown again; a destroyed native Window
		// must not remain attached and prevent syncWindow from recreating it.
		// Hide before detaching, or a closed dialog becomes a visible inline
		// child and is painted by its former owner as a floating remnant.
		this.widget.NakedWidget().setVisible(false)
		this.widget.NakedWidget().win = nil
	}
	if this.glTexture != 0 {
		this.glfwWin.MakeContextCurrent()
		gl.DeleteTextures(1, &this.glTexture)
		this.glTexture = 0
	}
	if this.pbo != 0 {
		// glfwWin.MakeContextCurrent already happened above when we deleted
		// the texture (glTexture > 0 path). If glTexture was 0 there's
		// nothing else to free anyway, so guard with a glfwWin nil check.
		if this.glfwWin != nil {
			this.glfwWin.MakeContextCurrent()
		}
		gl.DeleteBuffers(1, &this.pbo)
		this.pbo = 0
		this.pboSize = 0
		this.textureWidth = 0
		this.textureHeight = 0
	}
	this.releaseBackBuffer()
	this.icon = nil
	this.widget = nil
	if pointerWindow == this {
		pointerWindow = nil
	}
	if this.glfwWin != nil {
		delete(winMap, this.glfwWin)
		// Cocoa/GLFW still owns the dispatch stack during a key or close
		// callback. Freeing that native window here made Cmd+Q use freed
		// memory on return. Detach now; release after outer event dispatch.
		native := this.glfwWin
		retireNativeWindow(native.Hide, native.Destroy)
		this.glfwWin = nil
	}
}

// Release references while the native attachment still identifies this
// window. After detaching a modal, its hidden children resolve OwnerWindow to
// the main form and a stale capture would steal all subsequent main-window
// mouse events. Independent preview/dialog windows are not part of this set.
func (this *Window) clearWindowInput() {
	this.cancelPointerGestures()
	belongs := func(widget IWidget) bool {
		return widget != nil && widget.OwnerWindow() == this
	}
	kept := captureStack[:0]
	for _, captured := range captureStack {
		if !belongs(captured) {
			kept = append(kept, captured)
		}
	}
	clear(captureStack[len(kept):])
	captureStack = kept
	if belongs(focusWidget) {
		focusWidget = nil
	}
	if belongs(lastMouseWidget) {
		lastMouseWidget = nil
	}
	if belongs(mouseHoverWidget) {
		setMouseHoverWidget(nil)
	}
	if pointerWindow == this {
		pointerWindow = nil
	}
	this.autoCaptured, this.toCapture, this.mouseEntered = false, false, false
	this.pointer.valid = false
}

func (this *Window) getBackBuffer(width, height int) paint.Pixmap {
	// GLFW uses exact physical dimensions: padding would change the texture
	// row stride and resample pixel-aligned glyphs during presentation.
	this.ensureBackBuffer(width, height)
	return this.backBuffer
}

func (this *Window) paint() {
	if this.painting {
		// A nested native paint must not reset the in-use context/clip stack.
		this.MarkFullDirty()
		return
	}
	if this.widget == nil || this.glfwWin == nil {
		return
	}

	this.painting = true
	started := time.Now()
	defer func() {
		this.painting = false
		if this.destroyAfterPaint {
			this.destroyAfterPaint = false
			this.destroy()
		}
		now := time.Now()
		this.framePacer.record(started, now)
		glfwAnimationPacer.record(started, now)
	}()

	this.glfwWin.MakeContextCurrent()

	// X11 can resize the drawable before its ConfigureNotify reaches the
	// widget callback. Reconcile actual logical dimensions before layout/draw,
	// rather than painting old bounds into a newly enlarged framebuffer.
	ww, wh := this.glfwWin.GetSize()
	if ww <= 0 || wh <= 0 {
		return
	}
	width, height := float64(ww), float64(wh)
	if this.widget.Width() != width || this.widget.Height() != height {
		this.widget.NakedWidget().setSize(width, height)
		this.MarkFullDirty()
		if this.destroyAfterPaint || this.widget == nil || this.glfwWin == nil {
			return // Layout may close its owner; do not allocate/present afterward.
		}
	}

	// Get content scale for Retina/HiDPI displays
	sx, sy := this.glfwWin.GetContentScale()
	fbw, fbh := this.glfwWin.GetFramebufferSize()
	if fbw <= 0 || fbh <= 0 {
		return // Iconified windows have no drawable framebuffer.
	}
	fullDirty, dirtyRegion := this.takeFrameDamage()
	// GPU overlays are not stored in the Cairo surface. A clipped tree pass
	// would forget unchanged charts after the fullscreen blit. Keep those
	// windows (and the dynamic perf overlay) on the conservative full path.
	fullDirty = fullDirty || this.sceneComposition.enabled || this.glTexture == 0 || len(this.glOverlays) > 0 || GlobalPerfStats.IsVisible() ||
		this.textureWidth != int32(fbw) || this.textureHeight != int32(fbh)

	// A replacement invalidates all previous pixels. Explicit release occurs
	// before allocation, keeping peak native memory bounded during resize.
	if this.ensureBackBuffer(fbw, fbh) {
		fullDirty = true
	}
	backBuffer := this.backBuffer
	// Reuse the painter when possible. Reset its state to a known baseline:
	// drain the save stack, clear any clip, reset the CTM, and re-apply the
	// default pen/brush so we match what NewPainter() would have produced.
	// Without the pen/brush reset, the first widget of the new frame would
	// inherit the last widget of the previous frame's stroke style.
	var backPainter paint.Painter
	if this.backPainter != nil {
		backPainter = this.backPainter
		backPainter.RestoreTo(0)
		backPainter.ResetClip()
		backPainter.ResetMatrix()
		backPainter.SetPen(paint.NewPen(paint.Color{0, 0, 0, 255}, 1))
		backPainter.SetBrush(nil)
	} else {
		backPainter = backBuffer.NewPainter()
		this.backPainter = backPainter
	}

	region := framebufferDamage(dirtyRegion, fullDirty, fbw, fbh, float64(sx), float64(sy))
	useClip := region.Dx() != fbw || region.Dy() != fbh

	if useClip {
		// Clip to physical-pixel rect first, then clear only inside the clip.
		// This preserves the unchanged pixels from the previous frame.
		backPainter.Rectangle(float64(region.Min.X), float64(region.Min.Y),
			float64(region.Dx()), float64(region.Dy()))
		backPainter.Clip()
		backPainter.SetOperator(paint.OpClear)
		backPainter.Paint()
		backPainter.SetOperator(paint.OpOver)
	} else {
		// Full redraw: clear the entire surface.
		backPainter.SetOperator(paint.OpClear)
		backPainter.Paint()
		backPainter.SetOperator(paint.OpOver)
	}

	// Reset the GPU chart-overlay queue before the widget tree draws; GPU-mode
	// charts re-enqueue their series lines this frame (drawn after the blit).
	this.resetGLOverlays()

	// Experimental direct composition uses a full tree pass, never an incomplete
	// damage list. The default path retains its existing partial upload behavior.
	var treePainter paint.Painter = backPainter
	if this.sceneComposition.enabled {
		treePainter = this.beginSceneComposition(backPainter)
	}

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
				this.sceneComposition.reject("widget paint panic")
			}
		}()
		DrawWidgetAll(this.widget, treePainter, 0, 0, 0, 0, width, height)
	}()
	GlobalPerfStats.RecordPaint(time.Since(paintStart))

	// Perf overlay: drawn after the widget tree while the logical-coord
	// scale is still in effect, so it lands in the back buffer at the
	// correct size. Only renders when visible (cheap otherwise).
	if GlobalPerfStats.IsVisible() {
		GlobalPerfStats.SetWidgetCount(CountWidgets(this.widget))
		GlobalPerfStats.Draw(treePainter, width, height)
	}

	backPainter.Restore()
	if this.destroyAfterPaint {
		return // teardown after the frame unwinds; do not present a closed owner
	}
	if this.sceneComposition.failed != "" && this.sceneComposition.enabled {
		this.sceneComposition.enabled = false
		this.sceneComposition.layers = nil
		this.MarkFullDirty()
		return // never present a partially cut-out/unsupported frame
	}
	backBuffer.Flush()

	// Get raw Cairo pixel data directly (BGRA format, no copy)
	dataPtr := backBuffer.DataPtr()
	if dataPtr == nil {
		this.MarkFullDirty()
		return
	}
	stride := backBuffer.Stride()

	// Upload to OpenGL texture at physical resolution. We try a PBO upload
	// first (allows the driver to DMA the previous frame async with this
	// frame's CPU work) and fall back to direct glTexSubImage2D if PBOs are unsupported
	// or fail at runtime.
	if this.glTexture == 0 {
		gl.GenTextures(1, &this.glTexture)
		// Enable PBO path for newly created textures. Disabled per-window
		// permanently if uploadTextureViaPBO fails.
		this.pboAvailable = true
		this.textureWidth = 0
		this.textureHeight = 0
	}
	gl.BindTexture(gl.TEXTURE_2D, this.glTexture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	// Texture allocation is independent of PBO support. Neither fallback nor
	// partial paints should reallocate a whole window texture every frame.
	if this.textureWidth != int32(fbw) || this.textureHeight != int32(fbh) {
		gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(fbw), int32(fbh), 0,
			gl.BGRA, gl.UNSIGNED_BYTE, nil)
		this.textureWidth, this.textureHeight = int32(fbw), int32(fbh)
	}

	uploaded := false
	if this.pboAvailable {
		uploaded = this.uploadTextureViaPBO(region, stride, dataPtr)
	}
	if !uploaded {
		gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
		gl.PixelStorei(gl.UNPACK_ROW_LENGTH, int32(stride/4))
		// Cairo stores ARGB32 as BGRA in memory on little-endian
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(region.Min.X), int32(region.Min.Y),
			int32(region.Dx()), int32(region.Dy()), gl.BGRA, gl.UNSIGNED_BYTE,
			unsafe.Add(dataPtr, region.Min.Y*stride+region.Min.X*4))
	}
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		core.Warn("GL error after texture upload: ", glErr)
		this.textureWidth, this.textureHeight = 0, 0
		this.MarkFullDirty()
		return
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
	if len(this.sceneComposition.layers) > 0 {
		if err := this.presentSceneComposition(fbw, fbh); err != nil {
			this.sceneComposition.failed = err.Error()
			this.sceneComposition.enabled = false
			this.sceneComposition.layers = nil
			this.MarkFullDirty()
			return
		}
		drawFullscreenQuadLayer(this.glTexture, int32(fbw), int32(fbh), texU, texV)
	} else {
		drawFullscreenQuadSurface(this.glTexture, int32(fbw), int32(fbh), texU, texV, this.transparentFramebuffer)
	}
	this.sceneComposition.layers = nil

	// GPU chart fast-path: draw queued series line strips over the blit.
	this.drawGLOverlays(int32(fbw), int32(fbh))

	gl.Flush()
	this.glfwWin.SwapBuffers()
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

func (this *Window) releaseNativeKeyRouting() {
	if cleanup := this.nativeKeyCleanup; cleanup != nil {
		this.nativeKeyCleanup = nil
		cleanup()
	}
}

func (this *Window) OnDestroy() {
	this.releaseNativeKeyRouting()
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
	if this.widget != nil {
		if oldX, oldY := this.widget.Pos(); oldX != x || oldY != y {
			this.dismissOwnedPopups()
		}
	}
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
	// Keep local state synchronous; the position callback can arrive later.
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
	if this.widget != nil && (this.widget.Width() != w || this.widget.Height() != h) {
		this.dismissOwnedPopups()
	}
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
	this.fullDirty = true
	this.dirtyRegion = geom.Rect{}
}

func (this *Window) UpdateRect(x, y, width, height float64) {
	// Per task spec: keep full-redraw semantics for the legacy Widget.Update
	// path. Callers wanting partial-region invalidation should call
	// MarkDirtyRect directly. This avoids regressing widgets whose visual
	// extent (shadows, focus halos, hover glows) is wider than their Bounds.
	this.dirty = true
	this.fullDirty = true
	this.dirtyRegion = geom.Rect{}
}

// MarkDirtyRect grows the pending dirty area by the given logical rect.
// Coalesces consecutive calls into a single bounding rect — there is at
// most one accumulated region per frame. If MarkFullDirty was called this
// frame the call is a no-op (the whole window is already going to repaint).
//
// Coordinates are window-root logical pixels, NOT child-local Bounds().
// Map child bounds to the window first. The paint pass applies content scale.
func (this *Window) MarkDirtyRect(x, y, w, h float64) {
	if w <= 0 || h <= 0 || !finite(x) || !finite(y) || !finite(w) || !finite(h) {
		return
	}
	this.dirty = true
	if this.fullDirty {
		return
	}
	r := geom.Rect{X: x, Y: y, Width: w, Height: h}
	if this.dirtyRegion.Width == 0 || this.dirtyRegion.Height == 0 {
		this.dirtyRegion = r
	} else {
		this.dirtyRegion = this.dirtyRegion.UniteCopy(r)
	}
}

// MarkFullDirty marks the entire window for redraw, discarding any
// accumulated partial dirty rect. Used when a global change (theme,
// resize, scroll) invalidates everything.
func (this *Window) MarkFullDirty() {
	this.dirty = true
	this.fullDirty = true
	this.dirtyRegion = geom.Rect{}
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

func (this *Window) clearIcon() {
	this.icon = nil
	if runtime.GOOS != "darwin" && this.glfwWin != nil {
		this.glfwWin.SetIcon(nil)
	}
}

func (this *Window) SetIcon(icon paint.Icon) {
	if this.glfwWin == nil || icon == nil || icon.IsAir() {
		return
	}
	this.icon = icon
	applyGLFWWindowIcon(runtime.GOOS, icon, this.glfwWin.SetIcon)
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
				cancelPointerGesture(cur)
				return true
			}
		}
	}
	captureStack = append(captureStack[:0], w)
	setCapture(w)
	cancelPointerGesture(cur)
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
	drawToastOverlay(iw, g)
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
	v := widgetChildViewport(iw)
	cx, cy, cw, ch = intersectRect(cx, cy, cw, ch, v.X, v.Y, v.Width, v.Height)
	if cw <= 0 || ch <= 0 {
		return
	}
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
	var focusReturn *transientFocusReturn
	if !b {
		if this.wt == WtPopup && this.widget != nil && widgetWithinPopups(focusWidget, map[IWidget]bool{this.widget: true}) {
			focusReturn = prepareTransientFocusReturn(this, this.popupOwner, this.popupRestoreFocus, nativeWindowFocused)
		} else if this.wt == WtPopup {
			debugFocusReturn("prepare-skip", "logical-focus-outside-popup", this, this.popupOwner, transientFocusGeneration, nativeWindowFocused)
		}
		// Return directly to the surviving owner before dismissing descendants.
		// Otherwise a focused submenu activates this root while it is closing,
		// then hiding the root leaves Cocoa without a key window.
		if focusReturn.restorePopupOwnerBeforeHide(func(w *Window) bool { return w.glfwWin != nil && w.IsVisible() }, nativeWindowFocused, nativeApplicationActive, func(w *Window) { w.glfwWin.Focus() }) {
			focusReturn = nil
		}
		this.dismissOwnedPopups()
		if this.wt == WtPopup && this.widget != nil {
			popups := map[IWidget]bool{this.widget: true}
			clearPopupInput(popups, focusTargetForOwner(this.popupOwner, this.popupRestoreFocus), widgetWithinPopups(focusWidget, popups))
		}
	}
	if this.glfwWin == nil {
		return
	}
	if b == this.IsVisible() {
		return
	}
	if b {
		transientFocusGeneration++
		this.refreshPopupOwner()
		if this.wt == WtPopup && focusWidget != this.widget {
			this.popupRestoreFocus = focusTargetForOwner(this.popupOwner, focusWidget)
		}
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
		if this.wt == WtPopup && this.popupOwner != nil && this.popupOwner.glfwWin != nil {
			// Bind only after the final global position and size are applied.
			// Cocoa orderOut removes child ownership, so every Show rebinds.
			bound := bindNativePopupOwner(this.glfwWin, this.popupOwner.glfwWin)
			if runtime.GOOS == "darwin" {
				core.Debug("popup native-owner: bound=", bound)
			}
		}
		this.dirty = true
		this.debugPopupState("show")
	} else {
		duringTransientTeardown(func() {
			if this.wt == WtPopup {
				unbindNativePopupOwner(this.glfwWin)
			}
			this.glfwWin.Hide()
		})
		this.debugPopupState("hide")
		if this.closeOnHide {
			this.destroy()
		}
		focusReturn.queue()
	}
}

func (this *Window) refreshPopupOwner() {
	if this.wt != WtPopup || this.widget == nil {
		return
	}
	this.popupOwner = nil
	if parent := this.widget.Parent(); parent != nil {
		if owner := parent.OwnerWindow(); owner != this {
			this.popupOwner = owner
		}
	}
}

// Existing SILK_DEBUG diagnostics only: geometry and widget types, never
// notification text, menu labels, document names or other window contents.
func (this *Window) debugPopupState(action string) {
	if this.wt != WtPopup || this.glfwWin == nil || !core.IsDebugOn() {
		return
	}
	var owner IWidget
	var ox, oy, ow, oh int
	if this.popupOwner != nil {
		owner = this.popupOwner.widget
		if native := this.popupOwner.glfwWin; native != nil {
			ox, oy = native.GetPos()
			ow, oh = native.GetSize()
		}
	}
	x, y := this.glfwWin.GetPos()
	w, h := this.glfwWin.GetSize()
	core.Debug("popup ", action, ": widget=", reflect.TypeOf(this.widget),
		", owner=", reflect.TypeOf(owner), ", ownerRect=", ox, ",", oy, ",", ow, "x", oh,
		", pos=", x, ",", y, ", size=", w, "x", h)
}

// popupOwnedBy follows only transient ownership, never a WtForm parent chain:
// an independent preview/dialog and its own menus must survive changes to a
// different form. Nested menu popups, however, belong to the whole chain.
func popupOwnedBy(popup, owner *Window) bool {
	for p := popup; p != nil && p.wt == WtPopup; p = p.popupOwner {
		if p.popupOwner == owner {
			return true
		}
		if p.popupOwner == p {
			return false
		}
	}
	return false
}

func widgetWithinPopups(widget IWidget, popups map[IWidget]bool) bool {
	for p := widget; p != nil; p = p.Parent() {
		if popups[p] {
			return true
		}
	}
	return false
}

// dismissOwnedPopups uses the normal Hide path (including Menu.OnHide and
// closeOnHide) rather than destroying reusable ComboBox/Button popup objects.
// Snapshot before callbacks because hiding can remove windows from winMap.
func (this *Window) dismissOwnedPopups() {
	var popups map[IWidget]bool
	for _, candidate := range winMap {
		if candidate != this && candidate.widget != nil && popupOwnedBy(candidate, this) {
			if popups == nil {
				popups = make(map[IWidget]bool)
			}
			popups[candidate.widget] = true
		}
	}
	if len(popups) == 0 {
		return
	}
	core.Debug("popup dismiss: owner=", reflect.TypeOf(this.widget), ", count=", len(popups))
	var restore IWidget
	hadPopupFocus := widgetWithinPopups(focusWidget, popups)
	if hadPopupFocus {
		for p := focusWidget.OwnerWindow(); p != nil && popupOwnedBy(p, this); p = p.popupOwner {
			restore = p.popupRestoreFocus
		}
	}
	if currentTooltip != nil && popups[currentTooltip] {
		HideToolTip()
	}
	for popup := range popups {
		if popup.IsVisible() {
			popup.Hide()
		}
	}
	clearPopupInput(popups, focusTargetForOwner(this, restore), hadPopupFocus)
}

func clearPopupInput(popups map[IWidget]bool, restore IWidget, hadPopupFocus bool) {
	// PopCapture only removes the top of its stack; discard any stale entries
	// left by callback ordering without touching captures from another form.
	kept := captureStack[:0]
	for _, captured := range captureStack {
		if !widgetWithinPopups(captured, popups) {
			kept = append(kept, captured)
		}
	}
	clear(captureStack[len(kept):])
	captureStack = kept
	if widgetWithinPopups(lastMouseWidget, popups) {
		lastMouseWidget = nil
	}
	if widgetWithinPopups(mouseHoverWidget, popups) {
		setMouseHoverWidget(nil)
	}
	if hadPopupFocus && (focusWidget == nil || widgetWithinPopups(focusWidget, popups)) {
		focusWidget = nil
		if restore != nil && !widgetWithinPopups(restore, popups) && restore.IsAllAncentorsVisible() {
			restore.SetFocus()
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
	if win := pointerWindow; win != nil && win.glfwWin != nil && win.pointer.valid && win.IsVisible() {
		return win.mapToGlobal(win.pointer.x, win.pointer.y)
	}
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
	return findGLFWPointerWindow(winMap, func(win *Window) bool {
		if !win.IsVisible() || win.glfwWin == nil {
			return false
		}
		wx, wy := win.glfwWin.GetPos()
		m := win.FrameMargin()
		ww, wh := win.widget.Size()
		lx, ly := float64(wx)+m.L, float64(wy)+m.T
		return xg >= lx && yg >= ly && xg < lx+ww && yg < ly+wh
	}, func(win *Window) bool {
		return win.glfwWin.GetAttrib(glfw.Hovered) == glfw.True
	}, nativeWindowFocused)
}

// Native hover resolves actual occlusion, not Go map iteration order. During a
// captured/released pointer transition GLFW may not yet report hover; only the
// focused, enabled surface is a safe fallback. In particular a modal's disabled
// owner must never receive its drops. A disabled hovered window still occludes
// other windows, and an unfocused background window is not a drop target.
func findGLFWPointerWindow(windows map[*glfw.Window]*Window, contains, hovered, focused func(*Window) bool) *Window {
	var focus *Window
	for _, win := range windows {
		if !contains(win) {
			continue
		}
		if hovered(win) {
			if win.IsEnabled() {
				return win
			}
			return nil
		}
		if win.IsEnabled() && focused(win) {
			focus = win
		}
	}
	return focus
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
	if eventModifiers != nil {
		if held, modifier := modifierKeyState(key, *eventModifiers); modifier {
			return held, false
		}
	}
	for gw := range winMap {
		// Any one of the GLFW keys behind this VK counts as down: Ctrl, Shift
		// and Alt each have a left and a right key, and the user may hold
		// either.
		for _, glfwKey := range vkToGLFWKeys(key) {
			if gw.GetKey(glfwKey) == glfw.Press {
				down = true
				break
			}
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
		if gw.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press {
			return true
		}
	}
	return false
}

func IsMouseRightDown() bool {
	for gw := range winMap {
		if gw.GetMouseButton(glfw.MouseButtonRight) == glfw.Press {
			return true
		}
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
	if !glfwReady {
		return 0, 0, 1920, 1080
	}
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
	if glfwReady && len(winMap) > 0 {
		glfw.PostEmptyEvent()
	}
}

// serviceUIFrame runs the per-iteration service work an event loop owes the
// rest of the framework, after this frame's input has been taken and before its
// paint pass: the watchdog heartbeat, due timers, tasks queued from background
// goroutines, and one animation frame. Both loops in this file call it, so
// neither can quietly drift from the other — ShowModal open-coded a subset
// (timers and animation, no heartbeat, no drain), and for as long as a modal
// dialog was up everything gui.Post had queued (dlv callbacks, LSP results, git
// refreshes, toasts) sat in the queue with nothing to drain it while the
// watchdog, fed only from MainLoop, reported the whole dialog as a stall.
// window_windows.go keeps a serviceUIFrame of its own for the same reason.
//
// The order is the contract. The heartbeat is stamped first, so "time since the
// last beat" covers the whole iteration and a callback that blocks inside it is
// still caught. The drain comes after input and timers and before the paint
// pass, so a task's mutation reaches the screen in this frame rather than the
// next. That drain runs its batch unlocked (see uiqueue.go), which is what keeps
// the re-entrant cases safe: a task may end this modal — the loop finishes the
// iteration and its own top-of-loop condition stops it — or open a nested one,
// whose serviceUIFrame sees only what was queued after the outer snapshot.
func serviceUIFrame() {
	uiHeartbeat()
	processTimers()
	drainUITasks()
	now := time.Now()
	if continuousFrameWanted() && glfwAnimationPacer.delay(now) == 0 && hasRenderableGLFWWindow() {
		fullRepaint := animationsRequireFullRepaint()
		pacerBefore := glfwAnimationPacer.finished
		AnimationTick()
		for _, win := range winMap {
			if fullRepaint && win.canRenderFrame() {
				win.Update()
			}
		}
		glfwAnimationPacer.recordUnlessNestedPaint(pacerBefore, now, time.Now())
	}
}

// MainLoop runs the GLFW event loop.
//
// Frames are driven by damage or continuous animation, with 25 FPS quiet /
// 60 FPS interaction ceilings by default. The pacer reserves CPU headroom
// after expensive frames. WaitEventsTimeout is interrupted by input/Post;
// the rendering ceiling never throttles input delivery. Vsync is an additional
// presentation limit, not the event-loop clock.
func MainLoop() {
	if err := nativeDesktopError("MainLoop"); err != nil {
		// Outside the legacy event-loop recovery: callers must be able to
		// detect that a native GUI was not started.
		panic(err)
	}
	core.Debug("MainLoop()")
	defer closeSceneGPUDevice()
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("Recover MainLoop() : %v ", e))
		}
	}()

	if len(winMap) == 0 {
		flushNativeDestruction()
		core.Warn("no windows.")
		return
	}

	for !shouldQuit {
		waitAdaptiveGLFWFrame()
		flushNativeDestruction()
		serviceUIFrame()

		painted := false
		// Repaint dirty windows: non-popup windows first, then popup windows.
		// This ensures popups always render on top of the main window.
		for _, win := range winMap {
			if win.dirty && win.canRenderFrame() && win.framePacer.delay(time.Now()) == 0 && win.wt != WtPopup {
				win.paint()
				painted = true
			}
		}
		for _, win := range winMap {
			if win.dirty && win.canRenderFrame() && win.framePacer.delay(time.Now()) == 0 && win.wt == WtPopup {
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

	flushNativeDestruction()
	SetUIWakeup(nil)
	glfwWakeup.stop() // join producers that already copied the old hook
	closeSceneGPUDevice()
	glfwReady = false
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
	if win.widget.Width() != float64(width) || win.widget.Height() != float64(height) {
		win.dismissOwnedPopups()
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
	if oldX, oldY := win.widget.Pos(); oldX != x || oldY != y {
		win.dismissOwnedPopups()
	}
	win.widget.NakedWidget().setPos(x, y)
}

func onWindowIconify(gw *glfw.Window, iconified bool) {
	if win := winMap[gw]; win != nil {
		win.iconified = iconified
		if !iconified {
			win.Update()
		}
	}
	if win := winMap[gw]; win != nil && iconified {
		win.dismissOwnedPopups()
	}
}

// Check after the whole native focus transfer, not in the owner's blur
// callback: clicking a menu naturally moves focus from its form to a popup.
// Only when the entire family is unfocused (another app/form is active) may
// the floating native popups be dismissed.
func onWindowFocus(gw *glfw.Window, focused bool) {
	win := winMap[gw]
	if win == nil {
		return
	}
	noteNativeFocusTransfer()
	if core.IsDebugOn() {
		core.Debug("focus-return native-event: window=", focusReturnWindowTag(win), ", focused=", focused,
			", generation=", transientFocusGeneration, ", teardown-depth=", transientTeardownDepth)
	}
	if focused {
		if win.wt == WtForm {
			activeForm = win
		}
		return
	}
	win.cancelPointerGestures()
	owner := win
	for owner.wt == WtPopup && owner.popupOwner != nil && owner.popupOwner != owner {
		owner = owner.popupOwner
	}
	if owner.popupFocusCheckPending {
		return
	}
	owner.popupFocusCheckPending = true
	Post(func() {
		owner.popupFocusCheckPending = false
		owner.dismissPopupsIfUnfocused(func(candidate *Window) bool {
			return candidate.glfwWin != nil && candidate.IsVisible() && candidate.glfwWin.GetAttrib(glfw.Focused) == glfw.True
		})
	})
}

func (this *Window) dismissPopupsIfUnfocused(focused func(*Window) bool) {
	if this.widget == nil || focused(this) {
		return
	}
	for _, candidate := range winMap {
		if candidate.widget != nil && popupOwnedBy(candidate, this) && focused(candidate) {
			return
		}
	}
	this.dismissOwnedPopups()
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

	// The frame gets the first word, before anything is torn down: an app
	// guarding unsaved work has to be able to answer "no" here, since by the
	// time Frame.Close() reports the close its views are already shut.
	if f, ok := win.widget.(*Frame); ok && !f.CanClose() {
		return
	}

	if PromptSaveClose(win.widget, win) {
		// PromptSaveClose has already removed the closing window from winMap.
		// A remaining main form must survive closing a preview/tool form.
		if !hasOpenFormWindow() {
			core.Quit()
		}
	}
}

func onMouseButton(gw *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
	noteUIInteraction(time.Now())
	previousMods := eventModifiers
	eventModifiers = &mods
	defer func() { eventModifiers = previousMods }()
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

	// Cocoa mouseDown/up do not emit a cursor-position callback first.
	// Prefer their own event coordinates over a stale move or a separately
	// polled physical pointer (AX/remote input need not move that pointer).
	cx, cy, fromEvent := mouseButtonEventPosition(win.glfwWin, button, action)
	if fromEvent {
		win.pointer = pointerEventState{x: cx, y: cy, valid: true}
		pointerWindow = win
	} else {
		cx, cy = win.pointer.position(gw.GetCursorPos)
	}
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
	dispatchMouseButtonAt(win, button, action, x, y)
}

// Both native-window and translated cross-window events use the same gesture
// routing. Capture controls delivery; a fresh hit-test controls hover. A button
// event need not have a preceding MOVE (stationary clicks, Cocoa/AX, remote UI).
func dispatchMouseButtonAt(win *Window, button glfw.MouseButton, action glfw.Action, x, y float64) {
	if win == nil || win.widget == nil || !win.enabled {
		return
	}
	capWidget := curCapture()
	hit := win.widget.FindWidgetAt(x, y)
	if action == glfw.Press {
		if win.wt == WtForm {
			activeForm = win
		}
		lastMouseWidget = hit
		if capWidget != nil {
			if capWidget.OwnerWindow() != win {
				return
			}
			lastMouseWidget = capWidget
		}
		if button == glfw.MouseButtonLeft || button == glfw.MouseButtonRight {
			win.toCapture = true
			// Popup/container capture can choose a child on PRESS only. Once
			// pressed, the child owns RELEASE even if it arrives over another row.
			if capWidget != nil && capWidget.NakedWidget().child != nil {
				x1, y1 := capWidget.MapFromWindow(x, y)
				if child := capWidget.FindWidgetAt(x1, y1); child != nil && child != capWidget {
					lastMouseWidget = child
					win.autoCaptured = pushCapture(child)
					win.toCapture = false
				}
			}
		}
	}
	target := lastMouseWidget
	if target == nil || target.OwnerWindow() != win {
		return
	}
	// Do not activate a captured button after release outside its hit area,
	// and do not require stale hover state to activate a stationary menu row.
	if hit == target {
		setMouseHoverWidget(target)
	} else {
		setMouseHoverWidget(nil)
	}
	// Enter/leave handlers are host code and may close or reparent the target.
	if lastMouseWidget != target || target.OwnerWindow() != win || !target.IsAllAncentorsVisible() {
		return
	}
	lastMouseTime = time.Now()
	if button == glfw.MouseButtonLeft && core.IsDebugOn() {
		core.Debug("mouse left dispatch: receiver=", reflect.TypeOf(win.widget), ", action=", action,
			", pos=", x, ",", y, ", target=", reflect.TypeOf(target),
			", hover=", reflect.TypeOf(mouseHoverWidget), ", capture=", reflect.TypeOf(capWidget))
	}
	x1, y1 := target.MapFromWindow(x, y)
	if action == glfw.Press {
		switch button {
		case glfw.MouseButtonLeft:
			if i, ok := target.(IEventLeftDown); ok {
				i.OnLeftDown(x1, y1)
			}
		case glfw.MouseButtonRight:
			if i, ok := target.(IEventRightDown); ok {
				i.OnRightDown(x1, y1)
			}
		case glfw.MouseButtonMiddle:
			// Middle clicks never arm automatic drag capture.
			if i, ok := target.(IEventMiddleDown); ok {
				i.OnMiddleDown(x1, y1)
			}
		}
	} else if action == glfw.Release {
		switch button {
		case glfw.MouseButtonLeft:
			if i, ok := target.(IEventLeftUp); ok {
				i.OnLeftUp(x1, y1)
			}
		case glfw.MouseButtonRight:
			if i, ok := target.(IEventRightUp); ok {
				i.OnRightUp(x1, y1)
			}
		case glfw.MouseButtonMiddle:
			if i, ok := target.(IEventMiddleUp); ok {
				i.OnMiddleUp(x1, y1)
			}
		}
		if button == glfw.MouseButtonLeft || button == glfw.MouseButtonRight {
			if win.autoCaptured {
				popCapture(target)
			}
			win.autoCaptured, win.toCapture = false, false
		}
	}
}

// Translated coordinates stay in the capture window's client space. Never
// re-hit-test the RELEASE target or bypass the ordinary capture/hover rules.
func deliverCapturedMouseButton(capWin *Window, button glfw.MouseButton, action glfw.Action, winX, winY float64) {
	dispatchMouseButtonAt(capWin, button, action, winX, winY)
}

func onCursorPos(gw *glfw.Window, xpos, ypos float64) {
	noteUIInteraction(time.Now())
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onCursorPos: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil || win.widget == nil {
		return
	}
	// The nested drag loop still needs new event coordinates even while
	// ordinary widget mouse dispatch is suspended.
	win.pointer = pointerEventState{x: xpos, y: ypos, valid: true}
	pointerWindow = win
	if dndActive || !win.enabled {
		return
	}
	gx, gy := win.mapToGlobal(xpos, ypos)
	if observeMenuPointerMotion(gx, gy) {
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
	noteUIInteraction(time.Now())
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
	cx, cy, fromEvent := scrollEventPosition(gw)
	if !fromEvent {
		cx, cy = gw.GetCursorPos()
	}

	gx, gy := win.widget.MapToGlobal(cx, cy)
	dispatchMouseWheel(win.widget, gx, gy, yoff)
}

var glfwCompositionActive = nativeHasMarkedText

func onKey(gw *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	// Like OLE, a drag owns keyboard input until it ends. Remember Escape
	// at event time: press and release may both arrive in one PollEvents batch.
	if dndActive {
		if key == glfw.KeyEscape && action == glfw.Press {
			dndCancelRequested = true
		}
		return
	}
	// Cocoa delivers GLFW keys before interpretKeyEvents/insertText. While
	// composing, IME owns the keystroke; do not submit/cancel/navigate the form.
	if glfwReady && glfwCompositionActive(gw) {
		return
	}
	noteUIInteraction(time.Now())
	previousMods := eventModifiers
	eventModifiers = &mods
	defer func() { eventModifiers = previousMods }()
	defer func() {
		if r := recover(); r != nil {
			core.Warn("panic in onKey: ", r)
		}
	}()
	win := winMap[gw]
	if win == nil {
		return
	}
	if key == glfw.KeyEscape && core.IsDebugOn() {
		core.Debug("key Escape: receiver=", reflect.TypeOf(win.widget), ", target=", reflect.TypeOf(focusWidget),
			", enabled=", win.enabled, ", modal=", win.inModal, ", action=", action)
	}
	if !win.enabled {
		return
	}

	vk := translateKey(key, mods)
	if vk == 0 {
		return
	}
	if (action == glfw.Press || action == glfw.Repeat) && (win.dispatchEscape(vk, action == glfw.Repeat) || win.dispatchMessageBoxEnter(vk, action == glfw.Repeat)) {
		core.Debug("key Escape routed: receiver=", reflect.TypeOf(win.widget), ", modal=", win.inModal)
		return
	}

	// Global F12: toggle the perf-stats overlay. Handled at the window
	// layer so it works regardless of which widget has focus (otherwise a
	// focused CodeEditor would consume the key). Only fires on press.
	if vk == KeyF12 && action == glfw.Press && win.allowsApplicationShortcuts() && ShortcutHandler(currentMods(), vk) == nil {
		GlobalPerfStats.Toggle()
		return
	}

	// Application-level shortcuts (Cmd+S / Cmd+O / Cmd+Z etc.) are
	// checked before focus routing — otherwise a focused CodeEditor
	// would eat Cmd+Z for its own undo before the IDE's design-canvas
	// undo could fire. dispatchShortcut returns true when the key was
	// consumed by a registered shortcut.
	if action == glfw.Press && win.dispatchWindowShortcut(vk) {
		return
	}
	if win.dispatchCharacterPalette(key, action, mods) {
		return
	}
	if (action == glfw.Press || action == glfw.Repeat) && win.dispatchTabNavigation(vk) {
		return
	}

	// Tab / Shift+Tab moves keyboard focus along the focus chain (see
	// focus.go). Widgets such as CodeEditor and the designer can claim it
	// for their own input before the window traverses focus.
	// Only fires on press/repeat; Shift+Tab goes backward.
	if vk == KeyTab && (action == glfw.Press || action == glfw.Repeat) {
		// A focused widget may claim Tab for itself (see IWantsTab): the design
		// canvas walks the widgets it is editing, which is the only keyboard
		// route to one of them.
		if i, ok := win.keyTarget().(IWantsTab); ok && i.WantsTab(IsKeyDown(KeyShift)) {
			return
		}
		root := win.widget
		if root != nil {
			forward := !IsKeyDown(KeyShift)
			next := nextFocusable(root, focusWidget, forward)
			if next != nil {
				next.SetFocus() // OnFocusChanged -> Update() refreshes focus rings
			}
		}
		return
	}

	// Determine the target widget: use focusWidget if set, otherwise
	// fall back to the root widget of the window receiving the key event.
	target := win.keyTarget()
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
	if dndActive {
		return
	}
	noteUIInteraction(time.Now())
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

	if utf8.ValidRune(char) && !unicode.IsControl(char) {
		if input := win.textInputTarget(); input != nil {
			input.OnTextInput(string(char))
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
		win.pointer.valid = false
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
	if err := nativeDesktopError("Window.ShowModal"); err != nil {
		panic(err)
	}
	if this.inModal {
		core.Warn("already modal, cannot ShowModal again")
		return nil
	}

	this.inModal = true
	this.modalErr = nil
	owner := this.ParentWindow()
	restoreTarget := focusTargetForOwner(owner, focusWidget)

	if this.wt == WtChild {
		core.Warn("ShowModal on child window may cause confusion")
	}

	enabledMap := make(map[*Window]bool)
	// Install rollback before native activation and user initialization.
	defer func() {
		if e := recover(); e != nil {
			this.modalErr = reportModalPanic(e)
			this.modalRet = nil // Failed initialization must never return acceptance.
		}
		this.inModal = false
		retParam = this.modalRet
		this.modalRet = nil
		focusReturn := prepareTransientFocusReturn(this, owner, restoreTarget, nativeWindowFocused)

		for pw := range enabledMap {
			pw.SetEnabled(true)
		}
		returned := focusReturn.restoreModalOwnerBeforeHide(func(w *Window) bool {
			return w.glfwWin != nil && w.IsVisible()
		}, nativeWindowFocused, nativeApplicationActive, func(w *Window) { w.glfwWin.Focus() })
		this.Close()
		if !returned {
			focusReturn.queue()
		}
	}()

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

	for this.inModal && !shouldQuit {
		waitAdaptiveGLFWFrame()
		// Same per-iteration service work as MainLoop, for the same reasons: a
		// modal dialog is an event loop like any other, so posted tasks must
		// keep running and the watchdog must keep being fed while it is open,
		// and the animation engine must keep ticking or any spinner/fade/slide
		// freezes for the life of the dialog.
		serviceUIFrame()

		// Paint non-popup windows first, then popups on top
		for _, win := range winMap {
			if win.dirty && win.canRenderFrame() && win.framePacer.delay(time.Now()) == 0 && win.wt != WtPopup {
				win.paint()
			}
		}
		for _, win := range winMap {
			if win.dirty && win.canRenderFrame() && win.framePacer.delay(time.Now()) == 0 && win.wt == WtPopup {
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

// vkToGLFWKeys maps a VK code back to every GLFW key that produces it, for
// KeyState queries. The mapping is many-to-one — KeyCtrl comes from both
// LeftControl and RightControl, and KeyShift and KeyMenu likewise cover two
// keys each — so a caller must probe the whole set. Returning the first match
// instead handed back a key Go's randomised map iteration picked at random,
// which reported a held modifier as up whenever it landed on the other side of
// the keyboard.
func vkToGLFWKeys(vk int) (keys []glfw.Key) {
	for gk, v := range glfwKeyToVK {
		if v == vk {
			keys = append(keys, gk)
		}
	}
	return
}
