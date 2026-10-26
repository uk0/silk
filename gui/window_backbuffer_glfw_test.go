//go:build !windows

package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

func TestGLFWBackBufferResizeRetiresOwnedResources(t *testing.T) {
	surface := paint.NewPixmap(16, 16)
	painter := surface.NewPainter()
	painter.Save()
	win := &Window{backBuffer: surface, backPainter: painter, backWidth: 16, backHeight: 16}
	defer func() {
		paint.ClosePainter(win.backPainter)
		paint.CloseSurface(win.backBuffer)
		paint.ClosePainter(painter)
		paint.CloseSurface(surface)
	}()
	got := win.getBackBuffer(32, 24)
	if surface.Surface != nil {
		t.Error("resize kept the old native surface alive until GC")
	}
	if painter.CurrentState() != 0 {
		t.Error("resize did not retire the painter and its saved state")
	}
	if win.backPainter != nil {
		t.Error("resize kept a painter bound to the previous surface")
	}
	if got == nil || got.Width() != 32 || got.Height() != 24 {
		t.Fatal("resize did not create the exact framebuffer")
	}
}

func TestGLFWBackBufferDestroyReleasesWithoutGC(t *testing.T) {
	surface := paint.NewPixmap(16, 16)
	painter := surface.NewPainter()
	painter.Save()
	defer paint.CloseSurface(surface)
	defer paint.ClosePainter(painter)
	win := &Window{backBuffer: surface, backPainter: painter, backWidth: 16, backHeight: 16}
	win.destroy()
	if surface.Surface != nil || painter.CurrentState() != 0 {
		t.Error("destroy only dropped Go references instead of closing native resources")
	}
	if win.backBuffer != nil || win.backPainter != nil || win.backWidth != 0 || win.backHeight != 0 {
		t.Error("destroy left stale buffer ownership or dimensions")
	}
	win.destroy() // idempotent cleanup
}

func TestGLFWBackBufferRecreatesMissingSurface(t *testing.T) {
	win := &Window{backWidth: 16, backHeight: 16}
	got := win.getBackBuffer(16, 16)
	defer paint.CloseSurface(got)
	if got == nil {
		t.Fatal("matching cached dimensions masked a missing surface")
	}
}
