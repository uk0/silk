//go:build !silk_no_cairo

package bench

import (
	"testing"

	"silk/paint"
)

// Cairo benchmarks measure the full per-call work — the painter is
// backed by a real cairo image surface, so every Fill/Stroke/DrawText
// runs the C library's rasteriser into pixels. Comparing these numbers
// directly to the glui benchmarks captures the headline win: glui's
// CPU cost is "record into a vertex buffer", Cairo's is "draw the
// pixels right now". The CPU time difference is what frees up the UI
// thread on glui.
//
// Surface size is 1920×1080 to match glui's framebuffer; for scenarios
// that draw outside the surface bounds Cairo simply clips, same as glui's
// off-screen quad emission.

func newCairoPainter() (paint.Painter, paint.Pixmap) {
	pix := paint.NewPixmap(1920, 1080)
	g := pix.NewPainter()
	return g, pix
}

func BenchmarkCairoRectFill(b *testing.B) {
	g, _ := newCairoPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RectFill(g, 1000)
	}
}

func BenchmarkCairoRoundedRect(b *testing.B) {
	g, _ := newCairoPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RoundedRect(g, 500)
	}
}

func BenchmarkCairoLinearGradient(b *testing.B) {
	g, _ := newCairoPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		LinearGradient(g, 200)
	}
}

func BenchmarkCairoTextPaint(b *testing.B) {
	g, _ := newCairoPainter()
	// Warm-up: first DrawText resolves the default font + initial atlas.
	TextPaint(g, 1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TextPaint(g, 200)
	}
}

func BenchmarkCairoScrollingList(b *testing.B) {
	g, _ := newCairoPainter()
	ScrollingList(g, 1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ScrollingList(g, 1000)
	}
}

func BenchmarkCairoTypicalForm(b *testing.B) {
	g, _ := newCairoPainter()
	TypicalForm(g)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TypicalForm(g)
	}
}
