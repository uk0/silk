package bench

import (
	"testing"

	"silk/glui"
)

// glui benchmarks measure CPU-side draw command recording into the
// renderer's vertex/index buffers. The actual GPU flush is excluded:
// flush() requires a real GL context, and what we want to compare
// against Cairo's per-call rasterisation cost is the time the CPU
// spends *between* the user calling p.Fill() and the work landing on
// the GPU command queue. That gap is what blocks the UI thread.
//
// Construction note: newAdapterTestRenderer is a glui-internal helper
// for off-GL tests. We can't reach it from outside the glui package, so
// we use the public newBenchRenderer-equivalent: NewCairoCompat takes
// any Renderer, and Renderer can be constructed from a Context. We
// don't have a real Context, so we assemble a minimal one that supports
// recording — the same pattern the glui benchmarks themselves use.

func newGluiPainter() (*glui.CairoCompat, func()) {
	r := glui.NewBenchRenderer(1920, 1080)
	c := glui.NewCairoCompat(r)
	reset := func() { glui.ResetBenchRenderer(r) }
	return c, reset
}

func BenchmarkGluiRectFill(b *testing.B) {
	g, reset := newGluiPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RectFill(g, 1000)
		reset()
	}
}

func BenchmarkGluiRoundedRect(b *testing.B) {
	g, reset := newGluiPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		RoundedRect(g, 500)
		reset()
	}
}

func BenchmarkGluiLinearGradient(b *testing.B) {
	g, reset := newGluiPainter()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		LinearGradient(g, 200)
		reset()
	}
}

func BenchmarkGluiTextPaint(b *testing.B) {
	g, reset := newGluiPainter()
	// Warm-up call: amortise the one-time font load (system CJK fallback
	// fonts are ~25MB each on macOS) before the timer starts.
	TextPaint(g, 1)
	reset()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TextPaint(g, 200)
		reset()
	}
}

func BenchmarkGluiScrollingList(b *testing.B) {
	g, reset := newGluiPainter()
	ScrollingList(g, 1) // warmup font + initial slice growth
	reset()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ScrollingList(g, 1000)
		reset()
	}
}

func BenchmarkGluiTypicalForm(b *testing.B) {
	g, reset := newGluiPainter()
	TypicalForm(g) // warmup font
	reset()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TypicalForm(g)
		reset()
	}
}
