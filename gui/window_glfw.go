//go:build !windows

package gui

import (
	"silk/core"
	"silk/geom"
	"silk/glui"
	"silk/gv"
	"silk/paint"
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"strconv"
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
	// backPainter is reused across paint cycles so we don't pay the
	// allocation + cairo_create cost every frame. It must be invalidated
	// whenever backBuffer is reallocated (e.g. on resize) — see paint().
	backPainter paint.Painter
	backWidth  int
	backHeight int
	widget      IWidget
	wt          WindowType
	title       string
	glTexture   uint32
	dirty   bool
	enabled bool

	// Dirty region tracking. fullDirty=true (the default) means the next
	// paint redraws the whole window. dirtyRegion accumulates partial
	// invalidations in logical (widget) coordinates; once the user opts
	// into partial repaints by calling MarkDirtyRect instead of Update,
	// the paint pass will use Cairo's clip rect to skip unchanged pixels.
	dirtyRegion geom.Rect
	fullDirty   bool

	// PBO (Pixel Buffer Object) state for async texture streaming. The PBO
	// is sized to match the texture; uploadTextureViaPBO orphans + remaps
	// it each frame so the driver can DMA the previous frame's data while
	// the CPU writes the next.
	pbo          uint32
	pboSize      int
	pboTexW      int32
	pboTexH      int32
	pboAvailable bool

	// useGlui: when true (opt-in via SILK_GLUI=1) the window renders
	// directly through the silk/glui pure-OpenGL pipeline, bypassing the
	// Cairo back buffer + texture upload path. Off by default — the legacy
	// Cairo path remains the production renderer.
	useGlui     bool
	gluiCtx     *glui.Context
	gluiPainter *glui.CairoCompat // reused across frames so the font atlas
	// (and its GL textures) survives — allocating a fresh painter per frame
	// would leak texture IDs at 60fps.
	gluiFps *glui.FPSCounter // rolling 1-second FPS measurement; surfaced
	// as a top-right overlay when SILK_GLUI_FPS=1.

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

// msaaSampleCount returns the multisample sample count to request via
// glfw.Samples. SILK_GLUI_MSAA overrides the default; valid values are
// 0 (disabled), 2, 4, 8, 16. Any non-numeric or negative value falls back
// to the default (4). Hardware that does not support the requested count
// is handled by GLFW + the driver, which silently downgrades; we don't
// crash on values like 32 even though most desktop GPUs cap at 16.
func msaaSampleCount() int {
	const defaultSamples = 4
	v := os.Getenv("SILK_GLUI_MSAA")
	if v == "" {
		return defaultSamples
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultSamples
	}
	// Round-trip through allowed values rather than passing arbitrary input
	// to the driver. 0 is honoured (explicit disable); other values are
	// snapped to the nearest supported tier.
	switch {
	case n == 0:
		return 0
	case n <= 2:
		return 2
	case n <= 4:
		return 4
	case n <= 8:
		return 8
	default:
		return 16
	}
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

	// Multisample antialiasing. The default 4× covers the visible jaggies
	// on triangulated stroke quads and ear-clipped concave polygons (where
	// the SDF rect shader's per-pixel AA doesn't apply). SILK_GLUI_MSAA
	// overrides the sample count: 0 disables MSAA entirely, otherwise the
	// value is passed through as the GLFW samples hint. The driver clamps
	// to whatever the hardware supports — we do not validate against that
	// because GLFW silently falls back when 16× isn't available.
	glfw.WindowHint(glfw.Samples, msaaSampleCount())

	// Stencil bits. 8 is sufficient for ~256 nested clip paths — well
	// beyond what any real widget tree reaches. Asking for stencil here
	// reserves the buffer at framebuffer-creation time so future work on
	// path-shaped clipping can light up the stencil pipeline without an
	// API churn that breaks existing windows.
	glfw.WindowHint(glfw.StencilBits, 8)

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

	// Opt-in glui path: SILK_GLUI=1 switches this window's paint() to the
	// pure-OpenGL renderer in silk/glui instead of the Cairo back buffer.
	// Init must run after gl.Init() because it compiles GLSL shaders.
	//
	// In silk_no_cairo builds the Cairo back buffer path is replaced by
	// a no-op nullPainter, so glui is the only renderer that produces
	// pixels. forceGluiPath returns true under that build tag so the
	// window auto-enables glui without needing the env var.
	if os.Getenv("SILK_GLUI") == "1" || forceGluiPath() {
		this.useGlui = true
		this.gluiCtx = glui.NewContext()
		if err := this.gluiCtx.Init(); err != nil {
			core.Warn("glui init failed:", err)
			this.useGlui = false
			this.gluiCtx = nil
		} else {
			// FPS counter is allocated unconditionally — Tick() is cheap
			// and the env-gated overlay reads from it. Allocating only
			// when the env is set would leave the counter unwired if the
			// user toggles it on at runtime via reload.
			this.gluiFps = glui.NewFPSCounter()
		}
	}

	// Enable vsync so SwapBuffers blocks until the display retrace.
	// On 60Hz this caps painting at 60fps; on ProMotion / 120Hz / 144Hz
	// monitors it adapts to the display's actual refresh rate. Combined
	// with the WaitEvents-based MainLoop this gives smooth pacing
	// without burning CPU between frames.
	glfw.SwapInterval(1)

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
	if this.gluiCtx != nil {
		// Release glui's GPU resources before tearing down the GL context.
		if this.glfwWin != nil {
			this.glfwWin.MakeContextCurrent()
		}
		this.gluiCtx.Destroy()
		this.gluiCtx = nil
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
		this.pboTexW = 0
		this.pboTexH = 0
	}
	// Drop painter and back buffer in a defined order: painter first, so
	// the cairo Context releases its surface reference before we drop the
	// surface. Both have finalizers that handle the underlying C resources;
	// dropping the references here just makes the destruction order
	// deterministic instead of GC-driven.
	this.backPainter = nil
	this.backBuffer = nil
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

	if this.useGlui && this.gluiCtx != nil {
		this.paintGlui()
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
	// Only allocate a new pixmap when the framebuffer size differs. A size
	// change invalidates any partial dirty region — there's no valid prior
	// content to preserve outside it. Also drop the cached painter, which
	// is bound to the previous surface.
	if this.backBuffer == nil || this.backWidth != fbw || this.backHeight != fbh {
		this.backBuffer = paint.NewPixmap(fbw, fbh)
		this.backWidth = fbw
		this.backHeight = fbh
		this.fullDirty = true
		this.dirtyRegion = geom.Rect{}
		this.backPainter = nil
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

	// Decide whether to clip to a dirty subrect. We clip when:
	//   - fullDirty is false
	//   - dirtyRegion is non-empty
	//   - the region is meaningfully smaller than the window (otherwise the
	//     overhead of clip setup outweighs any savings)
	useClip := false
	var clipX, clipY, clipW, clipH float64
	if !this.fullDirty &&
		this.dirtyRegion.Width > 0 && this.dirtyRegion.Height > 0 {
		// Inflate by 1 logical pixel each side: anti-aliasing and 1px borders
		// straddle their nominal coordinate, so a tight rect can leave fringes.
		r := this.dirtyRegion.ExpandCopy(1)
		// Intersect with the window rect.
		r = r.IntersectCopy(geom.Rect{X: 0, Y: 0, Width: width, Height: height})
		if r.Width > 0 && r.Height > 0 &&
			r.Area() < width*height*0.85 {
			clipX, clipY, clipW, clipH = r.X, r.Y, r.Width, r.Height
			useClip = true
		}
	}

	if useClip {
		// Clip to physical-pixel rect first, then clear only inside the clip.
		// This preserves the unchanged pixels from the previous frame.
		backPainter.Rectangle(clipX*float64(sx), clipY*float64(sy),
			clipW*float64(sx), clipH*float64(sy))
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

	// Upload to OpenGL texture at physical resolution. We try a PBO upload
	// first (allows the driver to DMA the previous frame async with this
	// frame's CPU work) and fall back to glTexImage2D if PBOs are unsupported
	// or fail at runtime.
	if this.glTexture == 0 {
		gl.GenTextures(1, &this.glTexture)
		// Enable PBO path for newly created textures. Disabled per-window
		// permanently if uploadTextureViaPBO fails.
		this.pboAvailable = true
		this.pboTexW = 0
		this.pboTexH = 0
	}
	gl.BindTexture(gl.TEXTURE_2D, this.glTexture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	uploaded := false
	if this.pboAvailable {
		uploaded = this.uploadTextureViaPBO(int32(this.backWidth),
			int32(this.backHeight), stride, dataPtr)
	}
	if !uploaded {
		gl.PixelStorei(gl.UNPACK_ROW_LENGTH, int32(stride/4))
		// Cairo stores ARGB32 as BGRA in memory on little-endian
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
			int32(this.backWidth), int32(this.backHeight),
			0, gl.BGRA, gl.UNSIGNED_BYTE, dataPtr)
	}

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		core.Warn("GL error after texture upload: ", glErr)
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
	this.fullDirty = false
	this.dirtyRegion = geom.Rect{}
}

// paintGlui renders the window via the silk/glui pure-OpenGL pipeline.
//
// Currently a smoke-test implementation: it draws a fixed pattern of shapes
// proving the renderer + shaders work end-to-end. The full widget-tree
// integration (replacing DrawWidgetAll with a glui-aware visitor) lands in
// a follow-up. Until then, this method exists to validate the GL path on
// real hardware while the existing Cairo path keeps shipping pixels.
func (this *Window) paintGlui() {
	this.glfwWin.MakeContextCurrent()

	// Sample frame timing first thing so the rolling 1-second window is
	// stamped with this frame even if a panic later in the function
	// short-circuits the overlay draw. The counter ignores nil receivers
	// only via this guard, not internally.
	if this.gluiFps != nil {
		this.gluiFps.Tick()
	}

	fbw, fbh := this.glfwWin.GetFramebufferSize()
	sx, _ := this.glfwWin.GetContentScale()

	width, height := this.widget.Size()
	if width <= 0 || height <= 0 {
		ww, wh := this.glfwWin.GetSize()
		if ww > 0 && wh > 0 {
			width, height = float64(ww), float64(wh)
			this.widget.NakedWidget().setSize(width, height)
		} else {
			return
		}
	}

	this.gluiCtx.Resize(float32(width), float32(height), float32(sx))

	gl.Viewport(0, 0, int32(fbw), int32(fbh))
	gl.ClearColor(0.95, 0.95, 0.97, 1.0)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	r := this.gluiCtx.Begin(float32(width), float32(height))

	// Bridge the renderer through the paint.Painter facade so the existing
	// 62-widget set — which calls paint.Painter methods inside every Draw()
	// — can render through the GPU pipeline without modifications. Full
	// fidelity to Cairo's path/blend semantics is a non-goal; widgets that
	// need exotic operators or pattern brushes degrade gracefully (see
	// CairoCompat documentation for the supported subset).
	//
	// The painter (and its FontCache) lives on the Window so the GL glyph
	// atlas survives across frames — allocating a fresh painter every paint
	// would generate (and leak) one set of textures per frame.
	if this.gluiPainter == nil {
		this.gluiPainter = glui.NewCairoCompat(r)
	} else {
		this.gluiPainter.BindRenderer(r)
	}
	// Advance the painter's frame clock + run cache eviction. Must come
	// after BindRenderer (which preserves the texture maps) and before
	// DrawWidgetAll so every Draw* call below stamps the new frame's
	// counter on its cache hits.
	this.gluiPainter.BeginFrame()

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				core.Warn("paint panic in DrawWidgetAll (glui):", rec)
			}
		}()
		DrawWidgetAll(this.widget, this.gluiPainter, 0, 0, 0, 0, width, height)
	}()

	// Optional FPS overlay. Drawing through the renderer directly (not via
	// gluiPainter) because no transform/clip/state is needed — just a pair
	// of text quads in screen space. setBatch flushes whatever batch
	// DrawWidgetAll left open, so we don't need an explicit flush.
	if this.gluiFps != nil && os.Getenv("SILK_GLUI_FPS") == "1" {
		fps := glui.DefaultFont(12)
		msg := this.gluiFps.Format()
		// Right-align the readout to a 100-point margin from the right
		// edge so it stays visible regardless of widget content width.
		r.DrawText(fps, msg, float32(width)-100, 16, glui.RGBA8(255, 200, 0, 220))
	}

	r.End()
	this.glfwWin.SwapBuffers()
	this.dirty = false
	this.fullDirty = false
	this.dirtyRegion = geom.Rect{}
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
// Coordinates are widget-local (the same coordinate system widget Bounds()
// uses). The paint pass converts to physical pixels via the content scale.
func (this *Window) MarkDirtyRect(x, y, w, h float64) {
	if w <= 0 || h <= 0 {
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
//
// Frame-pacing strategy:
//   - SwapInterval(1) is enabled per-window in create(), so SwapBuffers blocks
//     until the next display retrace. On ProMotion / 120Hz / 144Hz displays
//     this adapts to the panel's actual refresh rate instead of a hard 60fps.
//   - When the UI is idle (no animations, no live perf overlay) we use a long
//     wait timeout so timers still fire (idle timer = 47ms) and the loop can
//     react to off-thread wake-ups, but the CPU stays asleep most of the time.
//   - When animations are running or the perf overlay is live we use the
//     classic ~16ms tick so redraws happen smoothly.
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
		// Tick rate selection. The 47ms idle-timer interval is the longest
		// quiescent loop we can afford without breaking blinking cursors and
		// hover-stop detection.
		needsTick := GlobalPerfStats.IsVisible() || HasActiveAnimations()
		if needsTick {
			glfw.WaitEventsTimeout(1.0 / 60.0)
		} else {
			glfw.WaitEventsTimeout(0.047)
		}
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
		// Same wait strategy as MainLoop: short timeout when continuous
		// redraw is needed, longer (47ms idle-timer interval) otherwise.
		// SwapInterval(1) handles vsync.
		needsTick := GlobalPerfStats.IsVisible() || HasActiveAnimations()
		if needsTick {
			glfw.WaitEventsTimeout(1.0 / 60.0)
		} else {
			glfw.WaitEventsTimeout(0.047)
		}
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
