package glui

import (
	"math"
	"testing"
)

// TestPublicAPISmoke exercises Renderer draw methods within a single
// batch kind to catch regressions in vertex emission. Doesn't validate
// pixel output — just asserts no panic and that vertex buffers grow.
//
// Uses newTestRenderer (transform_test.go) which has no GL context. flush()
// drains buffers safely without ctx since the radial-gradient work, so
// cross-kind transitions no longer crash here — but the smoke test stays
// minimal for clarity, exercising one batch kind so the assertions read
// against a stable vertex layout. PushClip/PopClip and End() still need a
// real GL context (gl.Scissor / gl.Enable / gl.Disable); those paths are
// covered by the standalone glui_demo.
func TestPublicAPISmoke(t *testing.T) {
	r := newTestRenderer()

	// All within kindRect — no batch switches.
	r.FillRect(Rect{X: 10, Y: 10, W: 100, H: 50}, Color{1, 0, 0, 1})
	r.FillRoundedRect(Rect{X: 120, Y: 10, W: 100, H: 50}, 8, Color{0, 1, 0, 1})
	r.StrokeRect(Rect{X: 10, Y: 70, W: 100, H: 50}, 2, Color{0, 0, 1, 1})
	r.FillCircle(50, 200, 30, Color{1, 1, 0, 1})

	// Transform stack within the same kind.
	r.Save()
	r.Translate(400, 100)
	r.Rotate(0.5)
	r.Scale(2, 2)
	r.FillRect(Rect{X: 0, Y: 0, W: 30, H: 30}, Color{0.5, 0.5, 0.5, 1})
	r.Restore()

	if len(r.verts) == 0 {
		t.Error("no vertices recorded")
	}
	if len(r.indices) == 0 {
		t.Error("no indices recorded")
	}
}

// TestNoVertexCorruption verifies all vertex floats are finite. Catches
// uninitialised reads, divide-by-zero in projection, and similar bugs that
// silently produce NaN/Inf and corrupt the GPU buffer.
func TestNoVertexCorruption(t *testing.T) {
	r := newTestRenderer()
	r.FillRect(Rect{X: 100, Y: 100, W: 50, H: 50}, Color{1, 0.5, 0, 1})
	r.FillRoundedRect(Rect{X: 200, Y: 100, W: 50, H: 50}, 8, Color{1, 0.5, 0, 1})

	for i, v := range r.verts {
		floats := [12]float32{
			v.X, v.Y, v.U, v.V, v.R, v.G, v.B, v.A,
			v.CornerHX, v.CornerHY, v.CornerR, v.CornerAA,
		}
		for _, f := range floats {
			if isNaN(f) || isInf(f) {
				t.Errorf("vertex[%d] has non-finite float: %+v", i, v)
				break
			}
		}
	}
}

func isNaN(f float32) bool { return f != f }
func isInf(f float32) bool {
	// Use math.Inf to avoid the compile-time "division by zero" diagnostic.
	pos := float32(math.Inf(1))
	neg := float32(math.Inf(-1))
	return f == pos || f == neg
}

// TestGradientNoVertexCorruption mirrors TestNoVertexCorruption but for
// the FillGradientRect emit path, which appends vertices directly rather
// than through pushQuad / pushRectQuad. A fresh renderer is used so the
// initial kind-change check inside FillGradientRect short-circuits flush
// (curKind == kindNone, no indices pending → flush returns early without
// touching gl).
func TestGradientNoVertexCorruption(t *testing.T) {
	r := newTestRenderer()
	r.FillGradientRect(Rect{X: 50, Y: 50, W: 80, H: 40},
		Color{1, 0, 0, 1}, Color{0, 1, 0, 1}, false)
	r.FillGradientRect(Rect{X: 50, Y: 50, W: 80, H: 40},
		Color{1, 0, 0, 1}, Color{0, 1, 0, 1}, true)
	for i, v := range r.verts {
		floats := [12]float32{
			v.X, v.Y, v.U, v.V, v.R, v.G, v.B, v.A,
			v.CornerHX, v.CornerHY, v.CornerR, v.CornerAA,
		}
		for _, f := range floats {
			if isNaN(f) || isInf(f) {
				t.Errorf("gradient vertex[%d] non-finite: %+v", i, v)
				break
			}
		}
	}
}
