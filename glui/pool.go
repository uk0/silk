package glui

import (
	"sync"

	"silk/glui/shader"
)

// rendererPool reuses Renderer + its slices across frames.
var rendererPool = newPool()

type pool struct{ p sync.Pool }

func newPool() *pool {
	return &pool{p: sync.Pool{New: func() any {
		return &Renderer{
			verts:   make([]vertex, 0, 1024),
			indices: make([]uint16, 0, 1536),
		}
	}}}
}

func (p *pool) get() *Renderer { return p.p.Get().(*Renderer) }
func (p *pool) put(r *Renderer) {
	// Don't pool monstrously large buffers; let them GC.
	if cap(r.verts) > 16384 {
		return
	}
	p.p.Put(r)
}

// programFor returns the shader program matching a batch kind.
func (c *Context) programFor(k batchKind) *shader.Program {
	switch k {
	case kindRect:
		return c.rectProg
	case kindPath:
		return c.pathProg
	case kindImage:
		return c.imageProg
	case kindGlyph:
		return c.glyphProg
	case kindGradient:
		return c.gradientProg
	case kindGradientRamp:
		return c.gradientRampProg
	case kindGradientRadial:
		return c.gradientRadialProg
	}
	return nil
}
