package glui

import (
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"

	"silk/paint"
)

// Renderer records draw commands per frame and flushes them in batches.
//
// Usage:
//
//	r := ctx.Begin(framebufferW, framebufferH)
//	r.FillRect(rect, color)
//	r.FillRoundedRect(rect, radius, color)
//	r.End()  // flushes everything to the GPU
//
// Renderer is NOT safe for concurrent use. One instance per frame per
// Context. Reuse the same Renderer across frames to avoid allocations.
type Renderer struct {
	ctx *Context

	// Batched vertex data. Cleared at Begin, uploaded at End.
	verts   []vertex
	indices []uint16

	// Current shader+texture key. When this changes we flush the current
	// batch before starting a new one.
	curKind batchKind
	curTex  uint32

	frameW, frameH float32

	// Modelview transform stack. xform is the current top of stack and is
	// applied inside project() before clip-space conversion.
	xform  matrix3
	xstack []matrix3

	// Clip stack. curClip is the active GL scissor; clipStack holds the
	// previous states pushed by PushClip().
	curClip   clipState
	clipStack []clipState

	// curStencilRef is the stencil ref value for the topmost active
	// path-shaped clip (clipKindStencil). 0 means no stencil clip in
	// effect. Each PushClipPath bumps + writes; PopClipPath rewinds.
	// Bounded at 255 — well beyond realistic UI clip nesting.
	curStencilRef uint8

	// Active two-stop linear-gradient colours. Bound as the u_color0 /
	// u_color1 uniforms on the gradient shader at flush time. A batch can
	// contain only ONE pair of stops because uniforms are per-program global,
	// so any change to either colour forces a flush before the next quad is
	// emitted (FillGradientRect handles that comparison directly).
	gradStart Color
	gradEnd   Color

	// Active radial-gradient inner/outer radii in logical points. Bound as
	// the u_radii vec2 on gradientRadialProg at flush time. Like the linear
	// gradient pair above these are uniform-globals — a change in either
	// component forces a flush before the next quad joins the batch.
	radR0 float32
	radR1 float32

	// curOp is the active Cairo-style blend operator. Begin() resets this
	// to OpOver so cross-frame state can't leak. SetBlendOp flushes the
	// batch and reprograms gl.BlendFunc / gl.BlendEquation when the op
	// changes — same flush semantics as gradient stops or texture binds.
	curOp paint.Operator
}

type batchKind uint8

const (
	kindNone           batchKind = iota
	kindRect                     // solid + rounded rectangles, AA via SDF
	kindPath                     // arbitrary triangulated paths
	kindImage                    // textured quad
	kindGlyph                    // text from alpha atlas
	kindGlyphLCD                 // text from RGB-striped LCD subpixel atlas
	kindGradient                 // two-stop linear gradient quads
	kindGradientRamp             // multi-stop gradient via 1-D ramp texture
	kindGradientRadial           // radial multi-stop gradient
)

// Begin starts a new frame. fbW/fbH are in points (logical units).
// Clears vertex buffers but keeps GPU buffers allocated.
func (c *Context) Begin(fbW, fbH float32) *Renderer {
	r := rendererPool.get()
	r.ctx = c
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone
	r.curTex = 0
	r.frameW = fbW
	r.frameH = fbH
	r.radR0 = 0
	r.radR1 = 0

	// Reset transform stack to identity for this frame.
	r.xform = identityMatrix3()
	r.xstack = r.xstack[:0]

	// Reset clip stack — prior-frame scissor state must not leak.
	r.curClip = clipState{}
	r.clipStack = r.clipStack[:0]
	r.curStencilRef = 0
	gl.Disable(gl.SCISSOR_TEST)

	// Reset gradient stop cache so the first FillGradientRect of the frame
	// always primes uniforms — comparing against a stale prior-frame value
	// would let a colour-equal-but-batch-stale gradient skip the flush.
	r.gradStart = Color{}
	r.gradEnd = Color{}

	// Reset blend op to OVER. We don't trust whatever op the previous
	// frame left set — apply the GL state explicitly so Begin() lands in
	// a known configuration.
	r.curOp = paint.OpOver
	r.applyBlendState(defaultBlendState)

	return r
}

// End flushes any pending batch and uploads to the GPU.
func (r *Renderer) End() {
	r.flush()
	// Make sure scissor is off for whatever runs after us.
	gl.Disable(gl.SCISSOR_TEST)
	// Restore the default OVER blend state so external GL code (or the next
	// Begin) doesn't inherit whatever the last widget left set.
	if r.curOp != paint.OpOver {
		r.applyBlendState(defaultBlendState)
		r.curOp = paint.OpOver
	}
	rendererPool.put(r)
}

// project converts a point in logical (top-left origin, Y-down) coordinates
// to clip space [-1, 1] with Y-up. The current modelview transform is
// applied first.
func (r *Renderer) project(x, y float32) (cx, cy float32) {
	// Apply modelview transform (column-major affine, last row implicit).
	tx := r.xform[0]*x + r.xform[3]*y + r.xform[6]
	ty := r.xform[1]*x + r.xform[4]*y + r.xform[7]
	// Project to clip space.
	cx = (tx/r.frameW)*2 - 1
	cy = 1 - (ty/r.frameH)*2
	return
}

// pushQuad emits 4 vertices + 6 indices forming a quad with the given
// shared color. Each corner is projected through the current transform
// independently so the quad survives rotation/skew correctly.
//
// Corner-SDF data is zeroed; this is the right format for path/glyph/image
// quads, which never read the trailing a_corner attribute. Rect-kind quads
// must use pushRectQuad so the shader has well-defined SDF parameters.
func (r *Renderer) pushQuad(x, y, w, h float32, u0, v0, u1, v1 float32, col Color) {
	base := uint16(len(r.verts))
	x0, y0 := r.project(x, y)
	x1, y1 := r.project(x+w, y)
	x2, y2 := r.project(x+w, y+h)
	x3, y3 := r.project(x, y+h)

	r.verts = append(r.verts,
		vertex{x0, y0, u0, v0, col.R, col.G, col.B, col.A, 0, 0, 0, 0},
		vertex{x1, y1, u1, v0, col.R, col.G, col.B, col.A, 0, 0, 0, 0},
		vertex{x2, y2, u1, v1, col.R, col.G, col.B, col.A, 0, 0, 0, 0},
		vertex{x3, y3, u0, v1, col.R, col.G, col.B, col.A, 0, 0, 0, 0},
	)
	r.indices = append(r.indices,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}

// pushRectQuad emits a quad whose four vertices carry per-vertex SDF corner
// data. (halfW, halfH) is the rect's half-size in points, radius is the
// corner radius (0 for a sharp rect), and aaWidth is the anti-aliasing
// half-width — typically 1 point on the framebuffer.
//
// All four vertices receive identical corner data so the interpolated
// varying is constant across the quad — this keeps the SDF computation
// per-fragment exact while still letting different batched rects carry
// different sizes/radii in the same draw call.
func (r *Renderer) pushRectQuad(x, y, w, h, halfW, halfH, radius, aaWidth float32, col Color) {
	base := uint16(len(r.verts))
	x0, y0 := r.project(x, y)
	x1, y1 := r.project(x+w, y)
	x2, y2 := r.project(x+w, y+h)
	x3, y3 := r.project(x, y+h)

	// UVs are in *points* centered on the rect midpoint — exactly what the
	// SDF in the rect fragment shader consumes.
	u0, v0 := -halfW, -halfH
	u1, v1 := halfW, halfH

	r.verts = append(r.verts,
		vertex{x0, y0, u0, v0, col.R, col.G, col.B, col.A, halfW, halfH, radius, aaWidth},
		vertex{x1, y1, u1, v0, col.R, col.G, col.B, col.A, halfW, halfH, radius, aaWidth},
		vertex{x2, y2, u1, v1, col.R, col.G, col.B, col.A, halfW, halfH, radius, aaWidth},
		vertex{x3, y3, u0, v1, col.R, col.G, col.B, col.A, halfW, halfH, radius, aaWidth},
	)
	r.indices = append(r.indices,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}

// FillRect paints a solid axis-aligned rectangle. The rect shader's SDF
// reduces to the rectangle's natural edge when radius=0.
func (r *Renderer) FillRect(rc Rect, col Color) {
	r.setBatch(kindRect, 0)
	hw, hh := rc.W*0.5, rc.H*0.5
	r.pushRectQuad(rc.X, rc.Y, rc.W, rc.H, hw, hh, 0, 1, col)
}

// FillGradientRect paints rc with a two-stop linear gradient from start to
// end. When vertical is true the axis runs top-to-bottom; otherwise it runs
// left-to-right.
//
// The gradient shader takes its stop colours from per-program uniforms
// (u_color0, u_color1), which are global to a draw call — so a batch can
// only carry one pair of stops. Any change to either colour forces a flush
// before the new quad is emitted, ensuring the uniforms bound at the next
// flush match the colours the quad expects.
//
// Multi-stop gradients are not supported; callers route their start and end
// stops here and lose intermediates. paint.LinearGradient documents the
// limitation; CairoCompat collapses to the first/last stop pair.
func (r *Renderer) FillGradientRect(rc Rect, start, end Color, vertical bool) {
	// A different gradient — kind change, OR same kind but different stops —
	// must flush before we add the new quad. setBatch handles the kind/tex
	// comparison; the colour comparison is independent because uniforms are
	// not part of the batch key.
	if r.curKind != kindGradient || r.gradStart != start || r.gradEnd != end {
		r.flush()
		r.curKind = kindGradient
		r.curTex = 0
		r.gradStart = start
		r.gradEnd = end
	}

	base := uint16(len(r.verts))
	x0, y0 := r.project(rc.X, rc.Y)
	x1, y1 := r.project(rc.X+rc.W, rc.Y)
	x2, y2 := r.project(rc.X+rc.W, rc.Y+rc.H)
	x3, y3 := r.project(rc.X, rc.Y+rc.H)

	// Pack the gradient parameter t into v_uv.x. The fragment shader reads
	// v_uv.x and clamps to [0,1] — see gradientFragSrc. v_color is white so
	// the mix(start, end, t) is unmodulated; if a future feature wants a
	// per-vertex tint (alpha modulation from a Save/Restore stack) it can
	// pass a colour here.
	if vertical {
		// t = 0 at top, 1 at bottom.
		r.verts = append(r.verts,
			vertex{x0, y0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x1, y1, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x2, y2, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x3, y3, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
		)
	} else {
		// t = 0 at left, 1 at right.
		r.verts = append(r.verts,
			vertex{x0, y0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x1, y1, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x2, y2, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0},
			vertex{x3, y3, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0},
		)
	}
	r.indices = append(r.indices,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}

// setBatch flushes the current batch if the new kind/texture differs.
func (r *Renderer) setBatch(kind batchKind, tex uint32) {
	if r.curKind == kind && r.curTex == tex {
		return
	}
	r.flush()
	r.curKind = kind
	r.curTex = tex
}

// flush uploads the accumulated vertices/indices and issues a draw call.
//
// Off-GL test renderers (newAdapterTestRenderer / newTestRenderer) leave
// ctx == nil. We treat that as a "drain only" mode: the vertex/index
// buffers are cleared so the next batch starts fresh, but no GL calls
// fire. This keeps batch-transition tests honest — they can observe that
// the buffer drained between calls without needing a real GL context.
func (r *Renderer) flush() {
	if r.curKind == kindNone || len(r.indices) == 0 {
		return
	}
	if r.ctx == nil {
		r.verts = r.verts[:0]
		r.indices = r.indices[:0]
		return
	}

	// Upload vertices.
	gl.BindBuffer(gl.ARRAY_BUFFER, r.ctx.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(r.verts)*vertexSize, gl.Ptr(r.verts), gl.DYNAMIC_DRAW)

	// Upload indices.
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.ctx.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(r.indices)*2, gl.Ptr(r.indices), gl.DYNAMIC_DRAW)

	// Bind program for this batch kind.
	prog := r.ctx.programFor(r.curKind)
	prog.Use()

	// Wire up the attribute layout. With GL 2.1 + no VAO we set up pointers
	// each flush; this is cheap and avoids global state bugs.
	//
	// All four programs share the same 48-byte stride. The trailing
	// a_corner attribute is only present in the rect shader; for the other
	// kinds Attrib() returns -1 and we skip the enable.
	posLoc := uint32(prog.Attrib("a_pos"))
	uvLoc := uint32(prog.Attrib("a_uv"))
	colLoc := uint32(prog.Attrib("a_color"))

	gl.EnableVertexAttribArray(posLoc)
	gl.VertexAttribPointer(posLoc, 2, gl.FLOAT, false, vertexSize, unsafe.Pointer(uintptr(0)))
	gl.EnableVertexAttribArray(uvLoc)
	gl.VertexAttribPointer(uvLoc, 2, gl.FLOAT, false, vertexSize, unsafe.Pointer(uintptr(8)))
	gl.EnableVertexAttribArray(colLoc)
	gl.VertexAttribPointer(colLoc, 4, gl.FLOAT, false, vertexSize, unsafe.Pointer(uintptr(16)))

	cornerLoc := int32(-1)
	if r.curKind == kindRect {
		cornerLoc = prog.Attrib("a_corner")
		if cornerLoc >= 0 {
			loc := uint32(cornerLoc)
			gl.EnableVertexAttribArray(loc)
			gl.VertexAttribPointer(loc, 4, gl.FLOAT, false, vertexSize, unsafe.Pointer(uintptr(32)))
		}
	}

	if r.curTex != 0 {
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, r.curTex)
		prog.Set1i("u_tex", 0)
	}

	// Gradient uniforms must be bound after Use() and before the draw call —
	// Set4f calls glUniform4f which targets the currently bound program.
	if r.curKind == kindGradient {
		prog.Set4f("u_color0", r.gradStart.R, r.gradStart.G, r.gradStart.B, r.gradStart.A)
		prog.Set4f("u_color1", r.gradEnd.R, r.gradEnd.G, r.gradEnd.B, r.gradEnd.A)
	}
	// Radial gradient: pass (R0, R1) so the fragment shader can map per-
	// pixel distance to a 0..1 ramp index.
	if r.curKind == kindGradientRadial {
		prog.Set2f("u_radii", r.radR0, r.radR1)
	}

	// LCD glyph batches need per-channel destination weighting:
	//
	//   out_R = src_R * 1 + dst_R * (1 - src_R)
	//
	// The shader writes pre-multiplied (textRGB * cov, avgCov * textA), so
	// glBlendFunc(GL_ONE, GL_ONE_MINUS_SRC_COLOR) gives each destination
	// channel its own coverage factor — the GL 2.1 stand-in for dual-source
	// blending. After the draw we restore whatever curOp's blend state was
	// so subsequent batches don't inherit the LCD-specific factors.
	lcdBlendActive := false
	if r.curKind == kindGlyphLCD {
		gl.BlendFunc(gl.ONE, gl.ONE_MINUS_SRC_COLOR)
		lcdBlendActive = true
	}

	gl.DrawElements(gl.TRIANGLES, int32(len(r.indices)), gl.UNSIGNED_SHORT, unsafe.Pointer(uintptr(0)))

	if lcdBlendActive {
		state, _ := blendStateFor(r.curOp)
		gl.BlendFunc(state.srcFactor, state.dstFactor)
	}

	gl.DisableVertexAttribArray(posLoc)
	gl.DisableVertexAttribArray(uvLoc)
	gl.DisableVertexAttribArray(colLoc)
	if cornerLoc >= 0 {
		gl.DisableVertexAttribArray(uint32(cornerLoc))
	}

	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
}
