//go:build windows

package gui

// The Win32 backend has no OpenGL context at all: on_WM_PAINT draws the widget
// tree into a Cairo pixmap and blits that straight onto the window HDC
// (paint.NewWin32Surface), and no file compiled for windows links go-gl. There
// is therefore no framebuffer for a chart overlay to draw into and no blit for
// it to draw after — these no-ops are the finished state, not an unfinished
// wiring job. Implementing the overlay means first giving Win32 a WGL context,
// a texture-upload path and a swap chain, i.e. porting gl_renderer.go and the
// present path of window_glfw.go; the overlay is the last and smallest step of
// that, not the first.
//
// Until then supportsGLOverlay reports false and LineChart.drawSeriesGPU
// Cairo-strokes the series rather than enqueuing them (chart_line.go), so a
// GPU-mode chart still renders here, just on the CPU. resetGLOverlays and
// drawGLOverlays exist to keep the seam symmetric; their only callers live in
// window_glfw.go, so on Windows they are never reached.

func (this *Window) supportsGLOverlay() bool            { return false }
func (this *Window) enqueueChartOverlay(o chartOverlay) {}
func (this *Window) resetGLOverlays()                   {}
func (this *Window) drawGLOverlays(fbw, fbh int32)      {}
