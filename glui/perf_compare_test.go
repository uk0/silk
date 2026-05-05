package glui

import (
	"testing"

	"silk/paint"
)

// Side-by-side benchmark suite for measuring CairoCompat's CPU-side cost
// vs the bare Renderer fast paths. Running these against the real Cairo
// painter would require linking the cairo cgo bindings into the test
// binary; we deliberately keep the comparison in-package and CPU-only so
// the benchmark stays driver-free.
//
// Important: like bench_test.go we cannot exercise End() / flush() — both
// paths call gl.* and panic without a real GL context. We measure only
// the work that grows linearly with widget count: vertex emission, path
// triangulation, polyline construction, batch transitions. The renderer
// slices are reset between iterations, mirroring what flush() does at
// frame end.

// drawScene executes a representative widget-tree paint pattern on the
// given paint.Painter. Used for both the Cairo-style (CairoCompat) and
// any future native paint.Painter benchmarks.
//
// Text is intentionally omitted: CairoCompat.DrawText hits f.Texture()
// which calls gl.GenTextures on first sight — a panic in a benchmark
// without GL. A separate harness with a real window is the right place
// to measure text. Everything else (rect path, fill triangulation,
// stroke join emission, save/restore, transforms) is the bulk of UI
// frame work and runs fine here.
func drawScene(p paint.Painter, w, h float64) {
	// Background fill via Rectangle + Fill (typical "paint backdrop"
	// pattern from Form.Draw).
	p.SetBrush1(paint.Color{R: 240, G: 240, B: 245, A: 255})
	p.Rectangle(0, 0, w, h)
	p.Fill()

	// 50 buttons in a 10x5 grid: filled rect background + 1-px stroked
	// border. Mirrors what Button.Draw emits per frame.
	for i := 0; i < 50; i++ {
		x := float64((i % 10) * 60)
		y := float64((i / 10) * 30)
		p.SetBrush1(paint.Color{R: 100, G: 149, B: 237, A: 255})
		p.Rectangle(x+4, y+4, 52, 22)
		p.Fill()
		p.SetPen1(paint.Color{R: 70, G: 120, B: 210, A: 255}, 1)
		p.Rectangle(x+4, y+4, 52, 22)
		p.Stroke()
	}

	// 30 small rectangles to give a higher per-frame draw call count without
	// pulling in the GL-bound text path.
	for i := 0; i < 30; i++ {
		x := float64((i % 15) * 60)
		y := float64(200 + (i/15)*20)
		p.SetBrush1(paint.Color{R: 33, G: 37, B: 41, A: 255})
		p.Rectangle(x+10, y, 4, 12)
		p.Fill()
	}
}

// resetCompatRenderer mirrors flush() bookkeeping without the GL calls,
// so the benchmark loop can iterate without re-allocating the painter.
func resetCompatRenderer(r *Renderer) {
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone
	r.curTex = 0
	r.xform = identityMatrix3()
	r.xstack = r.xstack[:0]
	r.curClip = clipState{}
	r.clipStack = r.clipStack[:0]
}

// BenchmarkSceneViaCairoCompat measures the per-frame CPU cost of routing
// a representative widget paint pattern through the CairoCompat facade —
// the same code path the legacy widget set uses under SILK_GLUI=1.
//
// What's actually measured: the paint.Painter API surface (state pushes,
// Rectangle path emission, Fill triangulation, Polyline stroke join
// emission) plus the glyph atlas's CPU-side metric calls. UploadTexture,
// gl.BufferData, and gl.DrawElements are excluded — they require a GL
// context and would be measured by a separate end-to-end harness.
func BenchmarkSceneViaCairoCompat(b *testing.B) {
	r := newBenchRenderer(1920, 1080)
	c := NewCairoCompat(r)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		drawScene(c, 1920, 1080)
		resetCompatRenderer(r)
	}
}

// BenchmarkSceneViaNativeAdapter measures the same scene through the
// native PainterAdapter (no Cairo/paint imports). Used as a baseline:
// the gap between this and BenchmarkSceneViaCairoCompat is the cost of
// the paint.Painter compatibility layer itself.
func BenchmarkSceneViaNativeAdapter(b *testing.B) {
	r := newBenchRenderer(1920, 1080)
	p := NewPainterAdapter(r)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		drawSceneNative(p, 1920, 1080)
		resetCompatRenderer(r)
	}
}

// drawSceneNative emits the same shapes as drawScene but through the
// native PainterAdapter API (no paint.Painter indirection). Note this
// skips text — PainterAdapter's text path differs from CairoCompat's
// (FontCache lookup vs. Cairo metrics) and a fair comparison would need
// a separate text-only micro-benchmark.
func drawSceneNative(p *PainterAdapter, w, h float64) {
	p.SetBrush1(Color{0.94, 0.94, 0.96, 1})
	p.Rectangle(0, 0, w, h)
	p.Fill()

	for i := 0; i < 50; i++ {
		x := float64((i % 10) * 60)
		y := float64((i / 10) * 30)
		p.SetBrush1(Color{0.39, 0.58, 0.93, 1})
		p.Rectangle(x+4, y+4, 52, 22)
		p.Fill()
		p.SetPen1(Color{0.27, 0.47, 0.82, 1}, 1)
		p.Rectangle(x+4, y+4, 52, 22)
		p.Stroke()
	}
}
