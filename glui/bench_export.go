package glui

// NewBenchRenderer constructs an off-GL Renderer suitable for CPU-only
// benchmarks. The returned Renderer has no Context and will never call
// gl.* — flush() short-circuits when ctx == nil. Tests outside the glui
// package use this entry point to drive batches without a window.
//
// w, h are the logical framebuffer size used by project(). They affect
// clip-space output but not allocation behaviour.
func NewBenchRenderer(w, h float32) *Renderer {
	return &Renderer{
		verts:   make([]vertex, 0, 8192),
		indices: make([]uint16, 0, 12288),
		frameW:  w,
		frameH:  h,
		xform:   identityMatrix3(),
	}
}

// ResetBenchRenderer drains the renderer's CPU buffers between bench
// iterations. Re-using a Renderer is much cheaper than allocating a new
// one because the underlying slice capacity is preserved — the inner
// loop only touches lengths.
func ResetBenchRenderer(r *Renderer) {
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone
	r.curTex = 0
	r.xform = identityMatrix3()
	r.xstack = r.xstack[:0]
}
