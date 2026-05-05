package glui

import (
	"unsafe"

	"github.com/go-gl/gl/v2.1/gl"
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
}

type batchKind uint8

const (
	kindNone  batchKind = iota
	kindRect            // solid + rounded rectangles, AA via SDF
	kindPath            // arbitrary triangulated paths
	kindImage           // textured quad
	kindGlyph           // text from atlas
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

	// Reset transform stack to identity for this frame.
	r.xform = identityMatrix3()
	r.xstack = r.xstack[:0]

	// Reset clip stack — prior-frame scissor state must not leak.
	r.curClip = clipState{}
	r.clipStack = r.clipStack[:0]
	gl.Disable(gl.SCISSOR_TEST)

	return r
}

// End flushes any pending batch and uploads to the GPU.
func (r *Renderer) End() {
	r.flush()
	// Make sure scissor is off for whatever runs after us.
	gl.Disable(gl.SCISSOR_TEST)
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
func (r *Renderer) flush() {
	if r.curKind == kindNone || len(r.indices) == 0 {
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

	gl.DrawElements(gl.TRIANGLES, int32(len(r.indices)), gl.UNSIGNED_SHORT, unsafe.Pointer(uintptr(0)))

	gl.DisableVertexAttribArray(posLoc)
	gl.DisableVertexAttribArray(uvLoc)
	gl.DisableVertexAttribArray(colLoc)
	if cornerLoc >= 0 {
		gl.DisableVertexAttribArray(uint32(cornerLoc))
	}

	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
}
