//go:build windows

package gui

// Windows uses the wgl blit path; the GL chart overlay is not wired there, so
// GPU-mode charts fall back to Cairo series drawing (supportsGLOverlay reports
// false and the enqueue/reset/draw hooks are no-ops).

func (this *Window) supportsGLOverlay() bool            { return false }
func (this *Window) enqueueChartOverlay(o chartOverlay) {}
func (this *Window) resetGLOverlays()                   {}
func (this *Window) drawGLOverlays(fbw, fbh int32)      {}
