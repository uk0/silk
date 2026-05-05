package glui

import (
	"testing"
)

// These benchmarks measure CPU-side draw command recording only.
//
// We can't call ctx.Begin() / r.End() in the timing loop because:
//   - Begin() calls gl.Disable(GL_SCISSOR_TEST), and
//   - End() -> flush() calls gl.BufferData / gl.DrawElements.
// Both panic without a real GL context. Instead we construct a *Renderer
// directly (same pattern as transform_test.go's newTestRenderer), record
// shapes, and reset the slices manually.
//
// What we actually measure: project() + slice growth + the inner geometry
// fast paths used by FillRect / FillRoundedRect / Save+Translate+Rotate.
// This is the work that grows linearly with widget count, so it is the
// number worth optimising against the Cairo Painter cost.

func newBenchRenderer(w, h float32) *Renderer {
	return &Renderer{
		verts:   make([]vertex, 0, 8192),
		indices: make([]uint16, 0, 12288),
		frameW:  w,
		frameH:  h,
		xform:   identityMatrix3(),
	}
}

func resetBenchRenderer(r *Renderer) {
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone
	r.curTex = 0
	r.xform = identityMatrix3()
	r.xstack = r.xstack[:0]
}

func BenchmarkRecord1000Rects(b *testing.B) {
	r := newBenchRenderer(1920, 1080)
	col := Color{0.5, 0.7, 1.0, 1.0}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			r.FillRect(Rect{X: float32(j%40) * 48, Y: float32(j/40) * 48, W: 40, H: 40}, col)
		}
		resetBenchRenderer(r)
	}
}

func BenchmarkRecord1000RoundedRects(b *testing.B) {
	r := newBenchRenderer(1920, 1080)
	col := Color{0.5, 0.7, 1.0, 1.0}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			r.FillRoundedRect(Rect{X: float32(j%40) * 48, Y: float32(j/40) * 48, W: 40, H: 40}, 8, col)
		}
		resetBenchRenderer(r)
	}
}

func BenchmarkRecordTransformStack(b *testing.B) {
	r := newBenchRenderer(800, 600)
	col := Color{1, 0, 0, 1}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			r.Save()
			r.Translate(float32(j*8), float32(j*4))
			r.Rotate(0.1)
			r.FillRect(Rect{0, 0, 20, 20}, col)
			r.Restore()
		}
		resetBenchRenderer(r)
	}
}

func BenchmarkProject(b *testing.B) {
	r := newBenchRenderer(1920, 1080)
	var sx, sy float32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sx, sy = r.project(float32(i&1023), float32((i>>10)&1023))
	}
	_, _ = sx, sy
}
