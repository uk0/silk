package glui

import (
	"silk/glui/shader"

	"github.com/go-gl/gl/v2.1/gl"
)

// Context owns all GPU state shared across frames: shader programs,
// vertex buffer objects, index buffers, and the texture atlas.
//
// One Context per OpenGL context (i.e. per Window). All draw operations
// must run on the goroutine that owns the GL context, typically the main
// thread.
type Context struct {
	// Shader programs for the seven core draw kinds.
	rectProg           *shader.Program // solid + bordered rectangles, rounded corners
	pathProg           *shader.Program // arbitrary paths via triangle fan
	imageProg          *shader.Program // textured quads
	glyphProg          *shader.Program // alpha glyph atlas blits (LUMINANCE)
	glyphLCDProg       *shader.Program // RGB-striped LCD subpixel glyph atlas
	gradientProg       *shader.Program // two-stop linear gradient quads (uniforms)
	gradientRampProg   *shader.Program // multi-stop gradient via 1-D ramp texture
	gradientRadialProg *shader.Program // radial gradient via ramp texture + per-pixel distance

	// Shared streaming VBO + EBO. All draw kinds append into this buffer
	// and flush on material change. 256KB initial size, grown geometrically.
	vbo uint32
	ebo uint32
	vao uint32 // 0 on GL 2.1; we set up state per-flush instead

	// Logical viewport in points (NOT framebuffer pixels).
	viewW, viewH float32

	// Framebuffer scale: physical pixels per point.
	scale float32

	initialized bool

	// msaaSamples reports the actual sample count negotiated with the
	// driver at Init time. 0 means MSAA is inactive (either the GLFW hint
	// requested 0 samples, or the driver couldn't honour the request).
	// Populated once and never mutated after Init.
	msaaSamples int

	// gradientRamps caches uploaded 256×1 colour-ramp textures keyed by a
	// hash of the stop list. Lives on Context (not Renderer) because the GL
	// texture must outlive a single Begin/End pair — most UI gradients
	// recur across frames, so the cache earns its keep on the second hit.
	//
	// Cache eviction is intentionally minimal: the typical app uses tens of
	// distinct gradients, well under any reasonable cap. Once a gradient
	// goes stale the host can call Context.Destroy and start fresh.
	gradientRamps map[uint64]*Texture
}

// NewContext allocates a Context. Call Init() once GL is current.
func NewContext() *Context {
	return &Context{scale: 1}
}

// Init compiles shaders and allocates GPU buffers. Call this exactly once
// after the GL context becomes current and before any rendering.
func (c *Context) Init() error {
	if c.initialized {
		return nil
	}

	// Compile core programs.
	prog, err := shader.Compile(rectVertSrc, rectFragSrc)
	if err != nil {
		return err
	}
	c.rectProg = prog

	prog, err = shader.Compile(pathVertSrc, pathFragSrc)
	if err != nil {
		return err
	}
	c.pathProg = prog

	prog, err = shader.Compile(imageVertSrc, imageFragSrc)
	if err != nil {
		return err
	}
	c.imageProg = prog

	prog, err = shader.Compile(glyphVertSrc, glyphFragSrc)
	if err != nil {
		return err
	}
	c.glyphProg = prog

	prog, err = shader.Compile(glyphLCDVertSrc, glyphLCDFragSrc)
	if err != nil {
		return err
	}
	c.glyphLCDProg = prog

	prog, err = shader.Compile(gradientVertSrc, gradientFragSrc)
	if err != nil {
		return err
	}
	c.gradientProg = prog

	prog, err = shader.Compile(gradientRampVertSrc, gradientRampFragSrc)
	if err != nil {
		return err
	}
	c.gradientRampProg = prog
	c.gradientRamps = make(map[uint64]*Texture)

	prog, err = shader.Compile(gradientRadialVertSrc, gradientRadialFragSrc)
	if err != nil {
		return err
	}
	c.gradientRadialProg = prog

	// Streaming buffers — reused every frame, GL_DYNAMIC_DRAW so the driver
	// can keep them in mappable memory.
	gl.GenBuffers(1, &c.vbo)
	gl.GenBuffers(1, &c.ebo)
	gl.BindBuffer(gl.ARRAY_BUFFER, c.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, 256*1024, nil, gl.DYNAMIC_DRAW)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, c.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, 64*1024, nil, gl.DYNAMIC_DRAW)

	// Common state for 2D UI work: alpha blending on, no depth test,
	// counter-clockwise faces (we always emit CCW triangles).
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)

	// Enable multisample rasterisation when the framebuffer was created
	// with sample buffers (GLFW Samples hint > 0). The driver treats
	// gl.Enable(MULTISAMPLE) as a no-op when no samples are available, so
	// it's safe to call unconditionally — but we still query the buffer
	// state to populate Context.MSAASamples for diagnostic / overlay use.
	gl.Enable(gl.MULTISAMPLE)
	var sampleBuffers, samples int32
	gl.GetIntegerv(gl.SAMPLE_BUFFERS, &sampleBuffers)
	gl.GetIntegerv(gl.SAMPLES, &samples)
	if sampleBuffers > 0 {
		c.msaaSamples = int(samples)
	}

	c.initialized = true
	return nil
}

// MSAASamples returns the actual multisample sample count negotiated at
// Init. 0 indicates MSAA is inactive — either disabled at the framebuffer
// level (SILK_GLUI_MSAA=0) or unsupported by the driver. Use this to
// surface the sample count in a diagnostic overlay or skip MSAA-only
// fast paths in test rigs.
func (c *Context) MSAASamples() int { return c.msaaSamples }

// Resize updates the logical viewport. width/height are in points,
// scale is physical-pixels-per-point (2.0 on Retina, 1.0 otherwise).
func (c *Context) Resize(width, height, scale float32) {
	c.viewW = width
	c.viewH = height
	c.scale = scale
	gl.Viewport(0, 0, int32(width*scale), int32(height*scale))
}

// Destroy releases all GPU resources. Call before tearing down the GL context.
func (c *Context) Destroy() {
	if !c.initialized {
		return
	}
	c.rectProg.Delete()
	c.pathProg.Delete()
	c.imageProg.Delete()
	c.glyphProg.Delete()
	if c.glyphLCDProg != nil {
		c.glyphLCDProg.Delete()
	}
	if c.gradientProg != nil {
		c.gradientProg.Delete()
	}
	if c.gradientRampProg != nil {
		c.gradientRampProg.Delete()
	}
	if c.gradientRadialProg != nil {
		c.gradientRadialProg.Delete()
	}
	for _, tex := range c.gradientRamps {
		tex.Free()
	}
	c.gradientRamps = nil
	gl.DeleteBuffers(1, &c.vbo)
	gl.DeleteBuffers(1, &c.ebo)
	c.initialized = false
}
