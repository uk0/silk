package glui

import "github.com/go-gl/gl/v2.1/gl"

// clipKind discriminates the two clip mechanisms — rectangular
// scissor (the historic path) and path-shaped stencil (added for
// rotated containers, ROADMAP §3.2.1). The kind drives PopClip's
// teardown and Save/Restore's matching pop selection.
type clipKind uint8

const (
	clipKindNone         clipKind = iota // no clip / cleared
	clipKindScissor                      // rectangular scissor
	clipKindStencil                      // path-shaped stencil with ref = depth
	clipKindStencilEmpty                 // degenerate stencil push (path < 3 pts)
)

// clipState describes a GL scissor rect in framebuffer pixels (NOT logical
// points). The (x, y) origin is bottom-left, matching gl.Scissor.
//
// For stencil clips (kind = clipKindStencil), x/y/w/h are unused; the
// active polygon mask lives in the GL stencil buffer and depth holds
// the stencil ref the eventual color draws compare against.
type clipState struct {
	x, y, w, h int32
	enabled    bool
	kind       clipKind
	depth      uint8 // stencil ref for clipKindStencil
}

// PushClip intersects the current clip with rc (specified in logical
// points, top-left origin) and installs the result as the active GL
// scissor. PushClip MUST be matched by a PopClip at the same nesting
// level, exactly like Save/Restore.
//
// Any pending batch is flushed before the scissor changes — otherwise the
// previously-queued geometry would be clipped against the wrong rect.
func (r *Renderer) PushClip(rc Rect) {
	r.flush()

	// Save the previous state so PopClip can restore it. We track the
	// stack regardless of GL context — off-GL test renderers still
	// observe nesting depth via len(r.clipStack) and the curClip update
	// below, even though the gl.Scissor call is skipped.
	r.clipStack = append(r.clipStack, r.curClip)

	// Convert logical rect to integer framebuffer pixels.
	scale := float32(1)
	if r.ctx != nil && r.ctx.scale > 0 {
		scale = r.ctx.scale
	}
	px := int32(rc.X * scale)
	py := int32(rc.Y * scale)
	pw := int32(rc.W * scale)
	ph := int32(rc.H * scale)

	// Intersect with the current clip if one is active.
	if r.curClip.enabled {
		// Convert the existing scissor (which is in GL bottom-left coords)
		// back to top-left to do the intersection in a single coord space.
		fbH := int32(r.frameH * scale)
		curTopY := fbH - (r.curClip.y + r.curClip.h)
		px, py, pw, ph = intersectRect(
			px, py, pw, ph,
			r.curClip.x, curTopY, r.curClip.w, r.curClip.h,
		)
	}

	if pw < 0 {
		pw = 0
	}
	if ph < 0 {
		ph = 0
	}

	// Convert to GL bottom-left origin: scissor_y = fbH - (top_y + h).
	fbH := int32(r.frameH * scale)
	glY := fbH - (py + ph)

	r.curClip = clipState{x: px, y: glY, w: pw, h: ph, enabled: true, kind: clipKindScissor}
	if r.ctx == nil {
		// Off-GL test renderer: stack state already updated. Skip the
		// driver calls so unit tests that exercise Clip / Save / Restore
		// don't need a real OpenGL context. Real-window paint paths
		// always have ctx set.
		return
	}
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(px, glY, pw, ph)
}

// PopClip restores the previous clip state (or disables scissoring if the
// stack is empty). Any pending batch is flushed first.
func (r *Renderer) PopClip() {
	r.flush()

	n := len(r.clipStack)
	if n == 0 {
		// Defensive: unbalanced PopClip just disables scissoring.
		r.curClip = clipState{}
		if r.ctx != nil {
			gl.Disable(gl.SCISSOR_TEST)
		}
		return
	}
	prev := r.clipStack[n-1]
	r.clipStack = r.clipStack[:n-1]
	r.curClip = prev

	if r.ctx == nil {
		return
	}
	if prev.enabled {
		gl.Enable(gl.SCISSOR_TEST)
		gl.Scissor(prev.x, prev.y, prev.w, prev.h)
	} else {
		gl.Disable(gl.SCISSOR_TEST)
	}
}

// ClipRect is a one-shot clip helper: it pushes the rect, runs fn, then
// pops. Matches the typical "draw inside this region" usage.
func (r *Renderer) ClipRect(rc Rect, fn func()) {
	r.PushClip(rc)
	fn()
	r.PopClip()
}

// intersectRect returns the intersection of two top-left-origin rects in
// (x, y, w, h) form. If they do not overlap the returned size is zero.
func intersectRect(ax, ay, aw, ah, bx, by, bw, bh int32) (x, y, w, h int32) {
	x1 := ax
	if bx > x1 {
		x1 = bx
	}
	y1 := ay
	if by > y1 {
		y1 = by
	}
	x2 := ax + aw
	if bx+bw < x2 {
		x2 = bx + bw
	}
	y2 := ay + ah
	if by+bh < y2 {
		y2 = by + bh
	}
	w = x2 - x1
	h = y2 - y1
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return x1, y1, w, h
}
