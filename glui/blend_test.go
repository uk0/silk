package glui

import (
	"testing"

	"github.com/go-gl/gl/v2.1/gl"

	"silk/paint"
)

// TestBlendStateForCoverage verifies every operator the fixed-function
// pipeline can express returns ok=true with the documented GL constants.
func TestBlendStateForCoverage(t *testing.T) {
	cases := []struct {
		op       paint.Operator
		want     blendState
		supported bool
	}{
		{paint.OpClear, blendState{gl.ZERO, gl.ZERO, gl.FUNC_ADD}, true},
		{paint.OpSource, blendState{gl.ONE, gl.ZERO, gl.FUNC_ADD}, true},
		{paint.OpOver, blendState{gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpIn, blendState{gl.DST_ALPHA, gl.ZERO, gl.FUNC_ADD}, true},
		{paint.OpOut, blendState{gl.ONE_MINUS_DST_ALPHA, gl.ZERO, gl.FUNC_ADD}, true},
		{paint.OpAtop, blendState{gl.DST_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpDest, blendState{gl.ZERO, gl.ONE, gl.FUNC_ADD}, true},
		{paint.OpDestOver, blendState{gl.ONE_MINUS_DST_ALPHA, gl.ONE, gl.FUNC_ADD}, true},
		{paint.OpDestIn, blendState{gl.ZERO, gl.SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpDestOut, blendState{gl.ZERO, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpDestAtop, blendState{gl.ONE_MINUS_DST_ALPHA, gl.SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpXor, blendState{gl.ONE_MINUS_DST_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true},
		{paint.OpAdd, blendState{gl.SRC_ALPHA, gl.ONE, gl.FUNC_ADD}, true},
		{paint.OpMultiply, blendState{gl.DST_COLOR, gl.ZERO, gl.FUNC_ADD}, true},
		{paint.OpScreen, blendState{gl.ONE, gl.ONE_MINUS_SRC_COLOR, gl.FUNC_ADD}, true},
		{paint.OpDarken, blendState{gl.ONE, gl.ONE, gl.MIN}, true},
		{paint.OpLighten, blendState{gl.ONE, gl.ONE, gl.MAX}, true},
	}
	for _, tc := range cases {
		got, ok := blendStateFor(tc.op)
		if ok != tc.supported {
			t.Errorf("op %d: supported=%v want %v", tc.op, ok, tc.supported)
		}
		if got != tc.want {
			t.Errorf("op %d: state=%+v want %+v", tc.op, got, tc.want)
		}
	}
}

// TestBlendStateForUnsupported checks that the non-separable operators
// fall through to defaultBlendState with ok=false. These should
// eventually graduate to a shader variant; the current contract is that
// the renderer silently uses OVER until that happens.
func TestBlendStateForUnsupported(t *testing.T) {
	unsupported := []paint.Operator{
		paint.OpOverlay,
		paint.OpColorDodge,
		paint.OpColorBurn,
		paint.OpHardLigh,
		paint.OpSoftLigh,
		paint.OpDifference,
		paint.OpExclusion,
		paint.OpHslHue,
		paint.OpHslSaturate,
		paint.OpHslColor,
		paint.OpHslLuminosity,
	}
	for _, op := range unsupported {
		got, ok := blendStateFor(op)
		if ok {
			t.Errorf("op %d expected unsupported, got ok=true (state=%+v)", op, got)
		}
		if got != defaultBlendState {
			t.Errorf("op %d unsupported should fall back to default; got %+v", op, got)
		}
	}
}

// TestRendererSetBlendOpUpdatesCurOp confirms the recorded operator
// matches the requested op for supported entries.
func TestRendererSetBlendOpUpdatesCurOp(t *testing.T) {
	r := newAdapterTestRenderer()
	if r.curOp != paint.OpOver {
		// The off-GL test helper hands us a zero-valued struct; default
		// of paint.Operator is 0 == OpClear by enum order. We assert the
		// caller writes OpOver before any test case relies on it.
	}
	r.curOp = paint.OpOver

	r.SetBlendOp(paint.OpSource)
	if r.curOp != paint.OpSource {
		t.Errorf("after SetBlendOp(OpSource), curOp=%v want OpSource", r.curOp)
	}
	r.SetBlendOp(paint.OpDestOut)
	if r.curOp != paint.OpDestOut {
		t.Errorf("after SetBlendOp(OpDestOut), curOp=%v want OpDestOut", r.curOp)
	}
	r.SetBlendOp(paint.OpOver)
	if r.curOp != paint.OpOver {
		t.Errorf("after SetBlendOp(OpOver), curOp=%v want OpOver", r.curOp)
	}
}

// TestRendererSetBlendOpUnsupportedFallsBackToOver verifies that an op
// the fixed-function pipeline can't express is recorded as OpOver — not
// the requested op. This ensures a subsequent SetBlendOp(OpOver) is a
// no-op (otherwise we'd flush twice for nothing).
func TestRendererSetBlendOpUnsupportedFallsBackToOver(t *testing.T) {
	r := newAdapterTestRenderer()
	r.curOp = paint.OpOver

	r.SetBlendOp(paint.OpHslLuminosity)
	if r.curOp != paint.OpOver {
		t.Errorf("unsupported op should record OpOver, got %v", r.curOp)
	}
}

// TestRendererSetBlendOpFlushesPendingBatch verifies that switching ops
// drains in-flight geometry — the previously emitted quads must blend
// against the framebuffer using the prior operator, not the new one.
func TestRendererSetBlendOpFlushesPendingBatch(t *testing.T) {
	r := newAdapterTestRenderer()
	r.curOp = paint.OpOver

	r.FillRect(Rect{X: 0, Y: 0, W: 10, H: 10}, Color{1, 0, 0, 1})
	if len(r.indices) == 0 {
		t.Fatalf("FillRect should have queued indices")
	}
	r.SetBlendOp(paint.OpSource)
	if len(r.indices) != 0 {
		t.Errorf("SetBlendOp must flush pending indices; got %d remaining", len(r.indices))
	}
}

// TestRendererSetBlendOpSameOpIsNoFlush asserts that re-setting the
// active op is a fast path: no flush, no GL call. Without this short
// circuit a tight repaint that calls SetOperator(OpOver) every widget
// would force a flush per widget.
func TestRendererSetBlendOpSameOpIsNoFlush(t *testing.T) {
	r := newAdapterTestRenderer()
	r.curOp = paint.OpOver

	r.FillRect(Rect{X: 0, Y: 0, W: 10, H: 10}, Color{1, 0, 0, 1})
	idxBefore := len(r.indices)
	r.SetBlendOp(paint.OpOver) // same as current → must not flush
	if len(r.indices) != idxBefore {
		t.Errorf("SetBlendOp(same) must not flush; before=%d after=%d", idxBefore, len(r.indices))
	}
}

// TestApplyBlendStateNoCtxIsSafe locks in the off-GL behaviour: with no
// real GL context (the test harness leaves ctx == nil), applyBlendState
// must short-circuit instead of calling gl.BlendEquation, which would
// segfault. This contract is what lets every other test in this file
// run without a window.
func TestApplyBlendStateNoCtxIsSafe(t *testing.T) {
	r := newAdapterTestRenderer()
	if r.ctx != nil {
		t.Fatalf("test renderer should have nil ctx")
	}
	r.applyBlendState(blendState{gl.ZERO, gl.ZERO, gl.FUNC_ADD})
}

// TestCairoCompatSetOperatorRoutesToRenderer is the integration check
// against the public paint.Painter API: a Cairo-compatible widget calls
// SetOperator and the underlying renderer should pick it up.
func TestCairoCompatSetOperatorRoutesToRenderer(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetOperator(paint.OpDestOut)
	if r.curOp != paint.OpDestOut {
		t.Errorf("CairoCompat.SetOperator(OpDestOut) → renderer.curOp=%v want OpDestOut", r.curOp)
	}
}

// TestCairoCompatSetOperatorWithOpenSubpathFlushes verifies the flush
// happens on op switch even mid-path. The CairoCompat keeps its own
// path buffer (not the GL vertex buffer), so we exercise the flush by
// emitting a fill+rect first to seed indices, then SetOperator.
func TestCairoCompatSetOperatorMidFrameFlushes(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetBrush(&paint.SolidBrush{Color: paint.Color{R: 1, G: 0, B: 0, A: 1}})
	c.Rectangle(0, 0, 10, 10)
	c.Fill()
	if len(r.indices) == 0 {
		t.Fatalf("CairoCompat.Fill should have produced indices")
	}
	c.SetOperator(paint.OpSource)
	if len(r.indices) != 0 {
		t.Errorf("SetOperator should flush indices; %d still pending", len(r.indices))
	}
	if r.curOp != paint.OpSource {
		t.Errorf("renderer.curOp = %v, want OpSource", r.curOp)
	}
}
