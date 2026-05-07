package glui

import (
	"github.com/go-gl/gl/v2.1/gl"

	"silk/glui/path"
)

// stencilDepth tracks how many PushClipPath calls are currently on
// the renderer's clip stack. Each push writes its triangulated path
// into stencil with INCR_WRAP and bumps depth; PopClipPath rewinds
// with DECR_WRAP. While depth > 0, color draws use stencil_func
// EQUAL with ref = depth so only fragments inside every nested clip
// pass.
//
// 8-bit stencil + INCR_WRAP wrap-around at 256 means clip nesting
// depth is bounded at 255 — well beyond any realistic UI tree. Each
// PushClipPath also increments curStencilRef; PopClipPath decrements.

// PushClipPath installs a polygon clip from points (top-left origin,
// logical units). The path is triangulated and its triangles written
// into the stencil buffer via INCR_WRAP, then color draws are gated
// on stencil ref equal to the new depth.
//
// Triangulation reuses the ear-clip path used for ordinary fills.
// The caller must close the polygon's contour (last point need not
// equal first; both are tolerated).
//
// PushClipPath MUST be matched by PopClipPath at the same nesting
// level. Mixed PushClip / PushClipPath calls are allowed: each push
// writes its kind onto the unified clip stack.
func (r *Renderer) PushClipPath(points [][2]float32) {
	if len(points) < 3 {
		// Degenerate path — push an "empty clip" so PopClipPath still
		// has something to pop.
		r.flush()
		r.clipStack = append(r.clipStack, r.curClip)
		r.curClip = clipState{enabled: false, kind: clipKindStencilEmpty, depth: r.curStencilRef}
		return
	}

	r.flush()

	// Triangulate via ear-clip. A single contour is the common case;
	// callers needing holes / multi-sub-paths should call PushClipPath
	// once per loop and accept the additional stencil cost.
	indices := path.Triangulate(points)

	prev := r.curClip
	r.clipStack = append(r.clipStack, prev)

	// Bump nesting depth + capture the stencil ref the eventual draws
	// must match against. Stencil is 8-bit so we cap at 255 and emit a
	// trace at the boundary; production scenes are well under that.
	if r.curStencilRef < 255 {
		r.curStencilRef++
	}
	depth := r.curStencilRef

	r.curClip = clipState{
		enabled: true,
		kind:    clipKindStencil,
		depth:   depth,
	}

	if r.ctx == nil {
		// Off-GL test renderer: state captured, skip GL writes. Tests
		// observe stack depth + curClip.kind without a live context.
		return
	}

	// First push initialises the stencil buffer. We clear once when
	// transitioning from depth 0 to depth 1 so prior frames' stencil
	// values don't leak. Subsequent pushes accumulate.
	gl.Enable(gl.STENCIL_TEST)
	if depth == 1 {
		gl.ClearStencil(0)
		gl.Clear(gl.STENCIL_BUFFER_BIT)
	}

	// Stencil-write pass: turn off color writes, increment stencil
	// inside the polygon's coverage. INCR_WRAP wraps at 256; we never
	// approach that bound in practice.
	gl.ColorMask(false, false, false, false)
	gl.StencilFunc(gl.ALWAYS, 0, 0xff)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.INCR_WRAP)

	// Render the triangles with the path program. We emit a temporary
	// batch and flush immediately so the stencil writes complete
	// before color draws resume.
	r.setBatch(kindPath, 0)
	base := uint16(len(r.verts))
	col := Color{1, 1, 1, 1}
	for _, p := range points {
		x, y := r.project(p[0], p[1])
		r.verts = append(r.verts, vertex{x, y, 0, 0, col.R, col.G, col.B, col.A, 0, 0, 0, 0})
	}
	for i := 0; i+2 < len(indices); i += 3 {
		r.indices = append(r.indices,
			base+uint16(indices[i]),
			base+uint16(indices[i+1]),
			base+uint16(indices[i+2]),
		)
	}
	r.flush()

	// Color-write resume: restore color mask, stencil func gates draws
	// on ref == current depth. Any nested PushClipPath replaces this
	// state with a higher ref.
	gl.ColorMask(true, true, true, true)
	gl.StencilFunc(gl.EQUAL, int32(depth), 0xff)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.KEEP)
}

// PopClipPath rewinds the most recent PushClipPath. Stencil is
// decremented over the same triangles via DECR_WRAP, then either
// (a) the previous clip is restored, or (b) stencil is disabled
// when the stack is empty.
//
// Mixed Push/Pop kinds: PopClipPath always pops the top of the
// stack; calling it after a scissor PushClip would corrupt state.
// Hosts should match the pop operation to the push kind. CairoCompat
// already routes uniformly so this is invisible at the public API.
func (r *Renderer) PopClipPath(points [][2]float32) {
	r.flush()

	n := len(r.clipStack)
	if n == 0 {
		// Defensive: nothing to pop — clear stencil and return.
		r.curClip = clipState{}
		if r.curStencilRef > 0 {
			r.curStencilRef--
		}
		if r.ctx != nil {
			gl.Disable(gl.STENCIL_TEST)
		}
		return
	}

	prev := r.clipStack[n-1]
	r.clipStack = r.clipStack[:n-1]

	if r.curStencilRef > 0 {
		r.curStencilRef--
	}
	r.curClip = prev

	if r.ctx == nil {
		return
	}

	// Decrement-write pass: clear the stencil bumps we made on push.
	if len(points) >= 3 {
		indices := path.Triangulate(points)
		gl.Enable(gl.STENCIL_TEST)
		gl.ColorMask(false, false, false, false)
		gl.StencilFunc(gl.ALWAYS, 0, 0xff)
		gl.StencilOp(gl.KEEP, gl.KEEP, gl.DECR_WRAP)

		r.setBatch(kindPath, 0)
		base := uint16(len(r.verts))
		col := Color{1, 1, 1, 1}
		for _, p := range points {
			x, y := r.project(p[0], p[1])
			r.verts = append(r.verts, vertex{x, y, 0, 0, col.R, col.G, col.B, col.A, 0, 0, 0, 0})
		}
		for i := 0; i+2 < len(indices); i += 3 {
			r.indices = append(r.indices,
				base+uint16(indices[i]),
				base+uint16(indices[i+1]),
				base+uint16(indices[i+2]),
			)
		}
		r.flush()
		gl.ColorMask(true, true, true, true)
	}

	// Restore parent state: stencil func to whatever the parent depth
	// requires, or disable stencil when we drop to 0.
	if r.curStencilRef > 0 && prev.kind == clipKindStencil {
		gl.StencilFunc(gl.EQUAL, int32(r.curStencilRef), 0xff)
	} else if r.curStencilRef == 0 {
		gl.Disable(gl.STENCIL_TEST)
	}

	// Restore parent scissor when the parent was a rectangular clip.
	if prev.enabled && prev.kind != clipKindStencil {
		gl.Enable(gl.SCISSOR_TEST)
		gl.Scissor(prev.x, prev.y, prev.w, prev.h)
	} else {
		gl.Disable(gl.SCISSOR_TEST)
	}
}
