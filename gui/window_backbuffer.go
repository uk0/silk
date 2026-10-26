package gui

import "github.com/uk0/silk/paint"

// Window backbuffers are exclusively owned by the UI thread. Retire the
// context before its surface, rather than leaving large native allocations
// alive until Go decides that their small wrappers need collection.
func (win *Window) releaseBackBuffer() {
	paint.ClosePainter(win.backPainter)
	paint.CloseSurface(win.backBuffer)
	win.backPainter, win.backBuffer = nil, nil
	win.backWidth, win.backHeight = 0, 0
}

// ensureBackBuffer returns whether prior pixels were invalidated. Platform
// callers choose their own physical sizing/alignment; unchanged frames reuse
// both the surface and its painter without allocating.
func (win *Window) ensureBackBuffer(width, height int) bool {
	width, height = max(1, width), max(1, height)
	if win.backBuffer != nil && win.backWidth == width && win.backHeight == height {
		return false
	}
	win.releaseBackBuffer()
	win.backBuffer = paint.NewPixmap(width, height)
	win.backWidth, win.backHeight = width, height
	return true
}
