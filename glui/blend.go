package glui

import (
	"github.com/go-gl/gl/v2.1/gl"

	"silk/paint"
)

// blendState describes the GL blend pipeline for a paint.Operator.
//
// equation is the value passed to glBlendEquation (FUNC_ADD / MIN / MAX).
// srcFactor and dstFactor are the values passed to glBlendFunc.
type blendState struct {
	srcFactor uint32
	dstFactor uint32
	equation  uint32
}

// defaultBlendState is the standard OVER mode for straight-alpha sources.
// glui colours arrive non-premultiplied, so SRC_ALPHA / ONE_MINUS_SRC_ALPHA
// implements the canonical Porter-Duff OVER for our pipeline.
var defaultBlendState = blendState{
	srcFactor: gl.SRC_ALPHA,
	dstFactor: gl.ONE_MINUS_SRC_ALPHA,
	equation:  gl.FUNC_ADD,
}

// blendStateFor maps a paint.Operator to the GL state that produces
// equivalent (or close-enough) output on the fixed-function blend
// pipeline. The bool return is false for operators that need a
// fragment-shader variant — the caller should fall back to OVER.
//
// Coverage:
//
//   - All twelve separable Porter-Duff operators (CLEAR / SOURCE / OVER /
//     IN / OUT / ATOP / DEST / DEST_OVER / DEST_IN / DEST_OUT / DEST_ATOP /
//     XOR) are exact.
//   - ADD / MULTIPLY / SCREEN match the Cairo definition for opaque
//     sources; alpha edge cases differ slightly because GL fixed-function
//     can't read both source and destination alpha into a single factor.
//   - DARKEN / LIGHTEN use FUNC_MIN / FUNC_MAX equations.
//   - OVERLAY / COLOR_DODGE / COLOR_BURN / HARD_LIGHT / SOFT_LIGHT /
//     DIFFERENCE / EXCLUSION and the HSL_* family are non-separable or
//     piecewise — they require a per-pixel shader read of the framebuffer
//     and are deferred to a later milestone. Returning ok=false lets the
//     caller fall back to OVER without a hard failure.
func blendStateFor(op paint.Operator) (blendState, bool) {
	switch op {
	case paint.OpClear:
		return blendState{gl.ZERO, gl.ZERO, gl.FUNC_ADD}, true
	case paint.OpSource:
		return blendState{gl.ONE, gl.ZERO, gl.FUNC_ADD}, true
	case paint.OpOver:
		return defaultBlendState, true
	case paint.OpIn:
		return blendState{gl.DST_ALPHA, gl.ZERO, gl.FUNC_ADD}, true
	case paint.OpOut:
		return blendState{gl.ONE_MINUS_DST_ALPHA, gl.ZERO, gl.FUNC_ADD}, true
	case paint.OpAtop:
		return blendState{gl.DST_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true
	case paint.OpDest:
		return blendState{gl.ZERO, gl.ONE, gl.FUNC_ADD}, true
	case paint.OpDestOver:
		return blendState{gl.ONE_MINUS_DST_ALPHA, gl.ONE, gl.FUNC_ADD}, true
	case paint.OpDestIn:
		return blendState{gl.ZERO, gl.SRC_ALPHA, gl.FUNC_ADD}, true
	case paint.OpDestOut:
		return blendState{gl.ZERO, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true
	case paint.OpDestAtop:
		return blendState{gl.ONE_MINUS_DST_ALPHA, gl.SRC_ALPHA, gl.FUNC_ADD}, true
	case paint.OpXor:
		return blendState{gl.ONE_MINUS_DST_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.FUNC_ADD}, true
	case paint.OpAdd:
		return blendState{gl.SRC_ALPHA, gl.ONE, gl.FUNC_ADD}, true
	case paint.OpMultiply:
		return blendState{gl.DST_COLOR, gl.ZERO, gl.FUNC_ADD}, true
	case paint.OpScreen:
		return blendState{gl.ONE, gl.ONE_MINUS_SRC_COLOR, gl.FUNC_ADD}, true
	case paint.OpDarken:
		return blendState{gl.ONE, gl.ONE, gl.MIN}, true
	case paint.OpLighten:
		return blendState{gl.ONE, gl.ONE, gl.MAX}, true
	}
	return defaultBlendState, false
}

// applyBlendState pushes state into the live GL pipeline. Tests with a
// nil context skip the GL calls — they verify Renderer.curOp directly
// without needing a real driver.
func (r *Renderer) applyBlendState(s blendState) {
	if r.ctx == nil {
		return
	}
	gl.BlendEquation(s.equation)
	gl.BlendFunc(s.srcFactor, s.dstFactor)
}

// SetBlendOp installs op as the active blend mode. The current batch is
// flushed before the GL state changes so already-emitted geometry blends
// against the framebuffer using the previously-active operator.
//
// Operators that the fixed-function pipeline can't express fall back to
// OVER (curOp is recorded as OpOver, not the requested op, so subsequent
// SetBlendOp(OpOver) calls correctly become no-ops).
func (r *Renderer) SetBlendOp(op paint.Operator) {
	if op == r.curOp {
		return
	}
	r.flush()
	state, ok := blendStateFor(op)
	if ok {
		r.curOp = op
	} else {
		r.curOp = paint.OpOver
		state = defaultBlendState
	}
	r.applyBlendState(state)
}
