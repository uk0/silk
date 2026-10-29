//go:build cgo && !silk_pure_go

package glrender

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"strings"

	"github.com/uk0/silk/scene3d"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
)

const maxResidentBytes = 64 << 20
const vertexShader = `#version 120
attribute vec3 position;
uniform mat4 world;
uniform mat4 viewProjection;
varying vec3 worldPosition;
void main() {
    vec4 p = world * vec4(position, 1.0);
    worldPosition = p.xyz;
    gl_Position = viewProjection * p;
}`
const fragmentShader = `#version 120
uniform vec4 material;
uniform vec3 objectID;
varying vec3 worldPosition;
void main() {
    vec3 n = cross(dFdx(worldPosition), dFdy(worldPosition));
    float magnitude = length(n);
    if (magnitude == 0.0) discard;
    float diffuse = 0.25 + 0.75 * abs(dot(n / magnitude, normalize(vec3(0.35, 0.8, 0.45))));
    gl_FragData[0] = vec4(material.rgb * diffuse, 1.0);
    gl_FragData[1] = vec4(objectID, 1.0);
}`

type meshBuffer struct {
	vertex, index uint32
	count         int32
	bytes         int64
	seen          uint64
}
type Renderer struct {
	api                                          *gl.API
	info                                         Info
	closed                                       bool
	program                                      uint32
	world, vp, material, objectID                int32
	fbo, depth                                   uint32
	textures                                     [2]uint32
	width, height                                int
	maxSize                                      int
	meshes                                       map[*scene3d.Mesh]*meshBuffer
	resident                                     int64
	epoch                                        uint64
	frame                                        Frame
	readback                                     []byte
	key                                          *scene3d.Snapshot
	camera                                       scene3d.Camera
	bg                                           color.RGBA
	capabilities                                 Capabilities
	instanceProgram, instanceBuffer, drawProgram uint32
	instanceVP                                   int32
	instanceData                                 []float32
	instanceBufferBytes                          int64
}

// NewCurrent changes GL state and needs a dedicated, current context. It never
// creates, terminates or migrates a native context. Close before its destruction.
func NewCurrent() (*Renderer, error) { return NewCurrentWithOptions(Options{}) }

// NewCurrentWithOptions additionally controls ordered hardware draw instancing.
func NewCurrentWithOptions(options Options) (*Renderer, error) {
	if !options.valid() {
		return nil, ErrOptions
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	api, err := gl.OpenCurrent()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	owned := false
	defer func() {
		if !owned {
			api.Close()
		}
	}()
	version := api.GetString(gl.VERSION)
	if version == "" {
		return nil, fmt.Errorf("%w: no current context", ErrUnavailable)
	}
	ext := " " + api.GetString(gl.EXTENSIONS) + " "
	if !strings.Contains(ext, " GL_EXT_framebuffer_object ") {
		return nil, fmt.Errorf("%w: EXT_framebuffer_object required", ErrUnavailable)
	}
	var buffers, colors, size, renderbufferSize int32
	api.GetIntegerv(gl.MAX_DRAW_BUFFERS, &buffers)
	api.GetIntegerv(gl.MAX_COLOR_ATTACHMENTS_EXT, &colors)
	api.GetIntegerv(gl.MAX_TEXTURE_SIZE, &size)
	api.GetIntegerv(gl.MAX_RENDERBUFFER_SIZE_EXT, &renderbufferSize)
	if buffers < 2 || colors < 2 || size < 1 || renderbufferSize < 1 {
		return nil, fmt.Errorf("%w: insufficient framebuffer capabilities", ErrUnavailable)
	}
	r := &Renderer{api: api, info: Info{api.GetString(gl.VENDOR), api.GetString(gl.RENDERER), version}, maxSize: int(min(size, renderbufferSize)), meshes: make(map[*scene3d.Mesh]*meshBuffer)}
	program, err := compileProgram(api, vertexShader, fragmentShader, []string{"position"})
	if err != nil {
		return nil, err
	}
	r.program = program
	r.world = api.GetUniformLocation(program, "world")
	r.vp = api.GetUniformLocation(program, "viewProjection")
	r.material = api.GetUniformLocation(program, "material")
	r.objectID = api.GetUniformLocation(program, "objectID")
	if r.world < 0 || r.vp < 0 || r.material < 0 || r.objectID < 0 {
		r.Close()
		return nil, fmt.Errorf("%w: missing shader uniforms", ErrUnavailable)
	}
	if err := r.initInstancing(options.Instancing); err != nil {
		r.Close()
		return nil, err
	}
	if e := api.GetError(); e != gl.NO_ERROR {
		r.Close()
		return nil, fmt.Errorf("%w: init GL error %#x", ErrUnavailable, e)
	}
	owned = true
	return r, nil
}
func (r *Renderer) Info() Info { return r.info }
func compileProgram(api *gl.API, vertex, fragment string, attributes []string) (uint32, error) {
	var shaders []uint32
	defer func() {
		for _, s := range shaders {
			api.DeleteShader(s)
		}
	}()
	for _, spec := range []struct {
		kind   uint32
		source string
	}{{gl.VERTEX_SHADER, vertex}, {gl.FRAGMENT_SHADER, fragment}} {
		s := api.CreateShader(spec.kind)
		if s == 0 {
			return 0, fmt.Errorf("%w: create shader", ErrUnavailable)
		}
		shaders = append(shaders, s)
		api.ShaderSource(s, spec.source)
		api.CompileShader(s)
		var ok int32
		api.GetShaderiv(s, gl.COMPILE_STATUS, &ok)
		if ok == 0 {
			buf := make([]byte, 8192)
			api.GetShaderInfoLog(s, int32(len(buf)), nil, &buf[0])
			return 0, fmt.Errorf("%w: shader: %s", ErrUnavailable, strings.TrimRight(string(buf), "\x00"))
		}
	}
	p := api.CreateProgram()
	if p == 0 {
		return 0, fmt.Errorf("%w: create program", ErrUnavailable)
	}
	for _, s := range shaders {
		api.AttachShader(p, s)
	}
	for index, name := range attributes {
		api.BindAttribLocation(p, uint32(index), name)
	}
	api.LinkProgram(p)
	var ok int32
	api.GetProgramiv(p, gl.LINK_STATUS, &ok)
	if ok == 0 {
		buf := make([]byte, 8192)
		api.GetProgramInfoLog(p, int32(len(buf)), nil, &buf[0])
		api.DeleteProgram(p)
		return 0, fmt.Errorf("%w: link: %s", ErrUnavailable, strings.TrimRight(string(buf), "\x00"))
	}
	return p, nil
}
func (r *Renderer) releaseTarget() {
	if r.fbo != 0 {
		r.api.DeleteFramebuffersEXT(1, &r.fbo)
		r.fbo = 0
	}
	if r.depth != 0 {
		r.api.DeleteRenderbuffersEXT(1, &r.depth)
		r.depth = 0
	}
	r.api.DeleteTextures(2, &r.textures[0])
	r.textures = [2]uint32{}
	r.width, r.height = 0, 0
}

// Close is an idempotent convenience wrapper. Use CloseCurrent to detect a
// wrong thread/context; neither method deletes resources from another context.
func (r *Renderer) Close() { _ = r.CloseCurrent() }

// CloseCurrent deletes resources only on the owning thread and context. A
// rejected call leaves the renderer intact so its owner can retry cleanup.
func (r *Renderer) CloseCurrent() error {
	if r == nil || r.api == nil {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !r.api.OnThread() {
		return ErrContext
	}
	if r.closed {
		return nil
	}
	if !r.api.Current() {
		return ErrContext
	}
	r.closed = true
	r.releaseTarget()
	for mesh, b := range r.meshes {
		r.deleteMesh(mesh, b)
	}
	if r.program != 0 {
		r.api.DeleteProgram(r.program)
		r.program = 0
	}
	if r.instanceProgram != 0 {
		r.api.DeleteProgram(r.instanceProgram)
		r.instanceProgram = 0
	}
	if r.instanceBuffer != 0 {
		r.api.DeleteBuffers(1, &r.instanceBuffer)
		r.instanceBuffer = 0
	}
	r.instanceData = nil
	r.instanceBufferBytes = 0
	r.frame = Frame{}
	r.readback = nil
	r.key = nil
	r.api.Close()
	return nil
}
func (r *Renderer) target(w, h int) error {
	if r.width == w && r.height == h {
		return nil
	}
	r.releaseTarget()
	r.key = nil
	r.api.GenFramebuffersEXT(1, &r.fbo)
	r.api.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, r.fbo)
	r.api.GenTextures(2, &r.textures[0])
	r.api.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	for i, tex := range r.textures {
		r.api.BindTexture(gl.TEXTURE_2D, tex)
		r.api.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		r.api.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		r.api.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		r.api.FramebufferTexture2DEXT(gl.FRAMEBUFFER_EXT, gl.COLOR_ATTACHMENT0_EXT+uint32(i), gl.TEXTURE_2D, tex, 0)
	}
	r.api.GenRenderbuffersEXT(1, &r.depth)
	r.api.BindRenderbufferEXT(gl.RENDERBUFFER_EXT, r.depth)
	r.api.RenderbufferStorageEXT(gl.RENDERBUFFER_EXT, gl.DEPTH_COMPONENT24, int32(w), int32(h))
	r.api.FramebufferRenderbufferEXT(gl.FRAMEBUFFER_EXT, gl.DEPTH_ATTACHMENT_EXT, gl.RENDERBUFFER_EXT, r.depth)
	status := r.api.CheckFramebufferStatusEXT(gl.FRAMEBUFFER_EXT)
	if e := r.api.GetError(); e != gl.NO_ERROR || status != gl.FRAMEBUFFER_COMPLETE_EXT {
		r.releaseTarget()
		return fmt.Errorf("%w: framebuffer status %#x, GL error %#x", ErrUnavailable, status, e)
	}
	r.width, r.height = w, h
	r.frame.Image = image.NewRGBA(image.Rect(0, 0, w, h))
	r.frame.pickPixels = make([]byte, w*h*4)
	r.readback = make([]byte, w*h*4)
	return nil
}
func (r *Renderer) deleteMesh(mesh *scene3d.Mesh, b *meshBuffer) {
	r.api.DeleteBuffers(1, &b.vertex)
	r.api.DeleteBuffers(1, &b.index)
	r.resident -= b.bytes
	delete(r.meshes, mesh)
}
func (r *Renderer) upload(mesh *scene3d.Mesh) (*meshBuffer, error) {
	if b := r.meshes[mesh]; b != nil {
		return b, nil
	}
	bytes := int64(mesh.VertexCount())*12 + int64(mesh.TriangleCount())*12
	if bytes > maxResidentBytes-r.resident {
		return nil, scene3d.ErrLimit
	}
	vertices := make([]float32, mesh.VertexCount()*3)
	for i := 0; i < mesh.VertexCount(); i++ {
		v, _ := mesh.Vertex(i)
		vertices[i*3], vertices[i*3+1], vertices[i*3+2] = float32(v.X), float32(v.Y), float32(v.Z)
	}
	indices := mesh.CopyIndices(nil)
	b := &meshBuffer{count: int32(len(indices)), bytes: bytes, seen: r.epoch}
	r.api.GenBuffers(1, &b.vertex)
	r.api.BindBuffer(gl.ARRAY_BUFFER, b.vertex)
	r.api.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Pointer(vertices), gl.STATIC_DRAW)
	r.api.GenBuffers(1, &b.index)
	r.api.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.index)
	r.api.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Pointer(indices), gl.STATIC_DRAW)
	if e := r.api.GetError(); e != gl.NO_ERROR {
		r.api.DeleteBuffers(1, &b.vertex)
		r.api.DeleteBuffers(1, &b.index)
		return nil, fmt.Errorf("%w: mesh upload %#x", ErrUnavailable, e)
	}
	r.meshes[mesh] = b
	r.resident += bytes
	r.frame.Stats.MeshUploads++
	return b, nil
}
func matrix32(m scene3d.Mat4) ([16]float32, error) {
	var out [16]float32
	for i, x := range m {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > math.MaxFloat32 {
			return out, scene3d.ErrGeometry
		}
		out[i] = float32(x)
	}
	return out, nil
}
func (r *Renderer) Render(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA) (*Frame, error) {
	if r == nil || r.api == nil {
		return nil, ErrClosed
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !r.api.OnThread() {
		return nil, ErrContext
	}
	if r.closed {
		return nil, ErrClosed
	}
	if !r.api.Current() {
		return nil, ErrContext
	}
	if s == nil {
		return nil, scene3d.ErrNode
	}
	if w <= 0 || h <= 0 || w > min(r.maxSize, scene3d.MaxDimension) || h > min(r.maxSize, scene3d.MaxDimension) || w > scene3d.MaxPixels/h {
		return nil, scene3d.ErrLimit
	}
	bg.A = 255
	if r.key == s && r.camera == c && r.bg == bg && r.width == w && r.height == h {
		r.frame.Stats.Cached = true
		return &r.frame, nil
	}
	vp, e := c.ViewProjection(float64(w) / float64(h))
	if e != nil {
		return nil, e
	}
	vp32, e := matrix32(vp)
	if e != nil {
		return nil, e
	}
	// Validate before touching the previous frame and mark the complete resident
	// set before eviction, so a new first mesh cannot evict a later shared mesh.
	r.epoch++
	var total int64
	for i := 0; i < s.InstanceCount(); i++ {
		in, _ := s.Instance(i)
		if _, e = matrix32(in.Transform); e != nil {
			return nil, e
		}
		if b := r.meshes[in.Mesh]; b != nil {
			b.seen = r.epoch
		}
		total += int64(in.Mesh.VertexCount())*12 + int64(in.Mesh.TriangleCount())*12
	}
	// Conservative per-instance upper bound also bounds cold-cache uploads.
	if total > maxResidentBytes {
		return nil, scene3d.ErrLimit
	}
	for mesh, b := range r.meshes {
		if b.seen != r.epoch {
			r.deleteMesh(mesh, b)
		}
	}
	r.key = nil
	r.frame.Stats = Stats{}
	if e = r.target(w, h); e != nil {
		return nil, e
	}
	r.api.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, r.fbo)
	r.api.Viewport(0, 0, int32(w), int32(h))
	r.api.Disable(gl.BLEND)
	r.api.Disable(gl.DITHER)
	r.api.Disable(gl.SCISSOR_TEST)
	r.api.Disable(gl.STENCIL_TEST)
	r.api.Disable(gl.CULL_FACE)
	r.api.Disable(gl.MULTISAMPLE)
	r.api.Enable(gl.DEPTH_TEST)
	r.api.DepthMask(true)
	r.api.DepthFunc(gl.LESS)
	r.api.ColorMask(true, true, true, true)
	// Clear IDs separately: the visible background is not an object ID.
	r.api.DrawBuffer(gl.COLOR_ATTACHMENT1_EXT)
	r.api.ClearColor(0, 0, 0, 0)
	r.api.Clear(gl.COLOR_BUFFER_BIT)
	r.api.DrawBuffer(gl.COLOR_ATTACHMENT0_EXT)
	r.api.ClearColor(float32(bg.R)/255, float32(bg.G)/255, float32(bg.B)/255, 1)
	r.api.ClearDepth(1)
	r.api.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	attachments := [2]uint32{gl.COLOR_ATTACHMENT0_EXT, gl.COLOR_ATTACHMENT1_EXT}
	r.api.DrawBuffers(2, &attachments[0])
	r.api.EnableVertexAttribArray(0)
	defer r.finishDraw()
	if e = r.drawInstances(s, vp, &vp32); e != nil {
		return nil, e
	}
	r.finishDraw()
	r.api.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	r.api.PixelStorei(gl.PACK_ALIGNMENT, 4)
	r.api.PixelStorei(gl.PACK_ROW_LENGTH, 0)
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT0_EXT)
	r.api.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(r.readback))
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT1_EXT)
	r.api.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(r.frame.pickPixels))
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return nil, fmt.Errorf("%w: render/readback %#x", ErrUnavailable, e)
	}
	stride := w * 4
	for y := 0; y < h; y++ {
		copy(r.frame.Image.Pix[y*stride:(y+1)*stride], r.readback[(h-y-1)*stride:(h-y)*stride])
	}
	r.frame.snapshot = s
	r.frame.Sequence++
	r.frame.Stats.ResidentMeshes = len(r.meshes)
	r.frame.Stats.ResidentBytes = r.resident
	r.frame.Stats.InstanceBufferBytes = r.instanceBufferBytes
	r.frame.Stats.ReadbackBytes = int64(w) * int64(h) * 8
	r.key, r.camera, r.bg = s, c, bg
	return &r.frame, nil
}
