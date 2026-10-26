package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

func TestWindowBackBufferReusesSurfacePainterAndPixels(t *testing.T) {
	win := &Window{}
	defer win.releaseBackBuffer()
	if !win.ensureBackBuffer(19, 23) {
		t.Fatal("first allocation must invalidate previous pixels")
	}
	surface := win.backBuffer
	painter := surface.NewPainter()
	win.backPainter = painter
	painter.SetBrush1(paint.Color{R: 12, G: 34, B: 56, A: 255})
	painter.Rectangle(0, 0, 19, 23)
	painter.Fill()
	for range 100 {
		if win.ensureBackBuffer(19, 23) || win.backBuffer != surface || win.backPainter != painter {
			t.Fatal("unchanged dimensions replaced the retained resources")
		}
	}
	img, err := surface.Image()
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(18, 22).RGBA()
	if r != 12*257 || g != 34*257 || b != 56*257 || a != 65535 {
		t.Fatal("reuse lost pixels needed by a partial repaint")
	}
}

func TestWindowBackBufferReleaseIsImmediateAndIdempotent(t *testing.T) {
	surface := paint.NewPixmap(31, 17)
	painter := surface.NewPainter()
	painter.Save()
	win := &Window{backBuffer: surface, backPainter: painter, backWidth: 31, backHeight: 17}
	defer paint.CloseSurface(surface)
	defer paint.ClosePainter(painter)
	win.releaseBackBuffer()
	if surface.Surface != nil || painter.CurrentState() != 0 {
		t.Fatal("native resources still live while their Go wrappers are retained")
	}
	if win.backBuffer != nil || win.backPainter != nil || win.backWidth != 0 || win.backHeight != 0 {
		t.Fatal("release did not clear all owned state")
	}
	win.releaseBackBuffer()
	if !win.ensureBackBuffer(31, 17) {
		t.Fatal("released dimensions suppressed recreation")
	}
	win.releaseBackBuffer()
}

func TestWindowBackBufferNormalizesEmptyDimensions(t *testing.T) {
	win := &Window{}
	defer win.releaseBackBuffer()
	if !win.ensureBackBuffer(0, -7) || win.backWidth != 1 || win.backHeight != 1 {
		t.Fatal("invalid dimensions were not normalized")
	}
	if win.ensureBackBuffer(-9, 0) {
		t.Fatal("equivalent normalized size caused an allocation")
	}
}

func BenchmarkWindowBackBufferReuse(b *testing.B) {
	win := &Window{}
	win.ensureBackBuffer(800, 600)
	win.backPainter = win.backBuffer.NewPainter()
	defer win.releaseBackBuffer()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		win.ensureBackBuffer(800, 600)
	}
}
