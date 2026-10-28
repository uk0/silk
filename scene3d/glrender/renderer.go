//go:build cgo && !silk_pure_go

package glrender

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/uk0/silk/scene3d"
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
	info                          Info
	closed                        bool
	program                       uint32
	world, vp, material, objectID int32
	fbo, depth                    uint32
	textures                      [2]uint32
	width, height                 int
	maxSize                       int
	meshes                        map[*scene3d.Mesh]*meshBuffer
	resident                      int64
	epoch                         uint64
	frame                         Frame
	readback                      []byte
	key                           *scene3d.Snapshot
	camera                        scene3d.Camera
	bg                            color.RGBA
}

// NewCurrent changes GL state and needs a dedicated, current context. It never
// creates, terminates or migrates a native context. Close before its destruction.
func NewCurrent() (*Renderer, error) {
	if err := gl.Init(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	version := gl.GetString(gl.VERSION)
	if version == nil {
		return nil, fmt.Errorf("%w: no current context", ErrUnavailable)
	}
	ext := " " + gl.GoStr(gl.GetString(gl.EXTENSIONS)) + " "
	if !strings.Contains(ext, " GL_EXT_framebuffer_object ") {
		return nil, fmt.Errorf("%w: EXT_framebuffer_object required", ErrUnavailable)
	}
	var buffers, colors, size, renderbufferSize int32
	gl.GetIntegerv(gl.MAX_DRAW_BUFFERS, &buffers)
	gl.GetIntegerv(gl.MAX_COLOR_ATTACHMENTS_EXT, &colors)
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &size)
	gl.GetIntegerv(gl.MAX_RENDERBUFFER_SIZE_EXT, &renderbufferSize)
	if buffers < 2 || colors < 2 || size < 1 || renderbufferSize < 1 {
		return nil, fmt.Errorf("%w: insufficient framebuffer capabilities", ErrUnavailable)
	}
	r := &Renderer{info: Info{gl.GoStr(gl.GetString(gl.VENDOR)), gl.GoStr(gl.GetString(gl.RENDERER)), gl.GoStr(version)}, maxSize: int(min(size, renderbufferSize)), meshes: make(map[*scene3d.Mesh]*meshBuffer)}
	program, err := compileProgram()
	if err != nil {
		return nil, err
	}
	r.program = program
	r.world = gl.GetUniformLocation(program, gl.Str("world\x00"))
	r.vp = gl.GetUniformLocation(program, gl.Str("viewProjection\x00"))
	r.material = gl.GetUniformLocation(program, gl.Str("material\x00"))
	r.objectID = gl.GetUniformLocation(program, gl.Str("objectID\x00"))
	if r.world < 0 || r.vp < 0 || r.material < 0 || r.objectID < 0 {
		r.Close()
		return nil, fmt.Errorf("%w: missing shader uniforms", ErrUnavailable)
	}
	if e := gl.GetError(); e != gl.NO_ERROR {
		r.Close()
		return nil, fmt.Errorf("%w: init GL error %#x", ErrUnavailable, e)
	}
	return r, nil
}
func (r *Renderer) Info() Info { return r.info }
func compileProgram() (uint32, error) {
	var shaders []uint32
	defer func() {
		for _, s := range shaders {
			gl.DeleteShader(s)
		}
	}()
	for _, spec := range []struct {
		kind   uint32
		source string
	}{{gl.VERTEX_SHADER, vertexShader}, {gl.FRAGMENT_SHADER, fragmentShader}} {
		s := gl.CreateShader(spec.kind)
		if s == 0 {
			return 0, fmt.Errorf("%w: create shader", ErrUnavailable)
		}
		shaders = append(shaders, s)
		str, free := gl.Strs(spec.source + "\x00")
		gl.ShaderSource(s, 1, str, nil)
		free()
		gl.CompileShader(s)
		var ok int32
		gl.GetShaderiv(s, gl.COMPILE_STATUS, &ok)
		if ok == 0 {
			buf := make([]byte, 8192)
			gl.GetShaderInfoLog(s, int32(len(buf)), nil, &buf[0])
			return 0, fmt.Errorf("%w: shader: %s", ErrUnavailable, strings.TrimRight(string(buf), "\x00"))
		}
	}
	p := gl.CreateProgram()
	if p == 0 {
		return 0, fmt.Errorf("%w: create program", ErrUnavailable)
	}
	for _, s := range shaders {
		gl.AttachShader(p, s)
	}
	gl.BindAttribLocation(p, 0, gl.Str("position\x00"))
	gl.LinkProgram(p)
	var ok int32
	gl.GetProgramiv(p, gl.LINK_STATUS, &ok)
	if ok == 0 {
		buf := make([]byte, 8192)
		gl.GetProgramInfoLog(p, int32(len(buf)), nil, &buf[0])
		gl.DeleteProgram(p)
		return 0, fmt.Errorf("%w: link: %s", ErrUnavailable, strings.TrimRight(string(buf), "\x00"))
	}
	return p, nil
}
func (r *Renderer) releaseTarget() {
	if r.fbo != 0 {
		gl.DeleteFramebuffersEXT(1, &r.fbo)
		r.fbo = 0
	}
	if r.depth != 0 {
		gl.DeleteRenderbuffersEXT(1, &r.depth)
		r.depth = 0
	}
	gl.DeleteTextures(2, &r.textures[0])
	r.textures = [2]uint32{}
	r.width, r.height = 0, 0
}
func (r *Renderer) Close() {
	if r == nil || r.closed {
		return
	}
	r.closed = true
	r.releaseTarget()
	for mesh, b := range r.meshes {
		r.deleteMesh(mesh, b)
	}
	if r.program != 0 {
		gl.DeleteProgram(r.program)
		r.program = 0
	}
	r.frame = Frame{}
	r.readback = nil
	r.key = nil
}
func (r *Renderer) target(w, h int) error {
	if r.width == w && r.height == h {
		return nil
	}
	r.releaseTarget()
	r.key = nil
	gl.GenFramebuffersEXT(1, &r.fbo)
	gl.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, r.fbo)
	gl.GenTextures(2, &r.textures[0])
	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	for i, tex := range r.textures {
		gl.BindTexture(gl.TEXTURE_2D, tex)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		gl.FramebufferTexture2DEXT(gl.FRAMEBUFFER_EXT, gl.COLOR_ATTACHMENT0_EXT+uint32(i), gl.TEXTURE_2D, tex, 0)
	}
	gl.GenRenderbuffersEXT(1, &r.depth)
	gl.BindRenderbufferEXT(gl.RENDERBUFFER_EXT, r.depth)
	gl.RenderbufferStorageEXT(gl.RENDERBUFFER_EXT, gl.DEPTH_COMPONENT24, int32(w), int32(h))
	gl.FramebufferRenderbufferEXT(gl.FRAMEBUFFER_EXT, gl.DEPTH_ATTACHMENT_EXT, gl.RENDERBUFFER_EXT, r.depth)
	status := gl.CheckFramebufferStatusEXT(gl.FRAMEBUFFER_EXT)
	if e := gl.GetError(); e != gl.NO_ERROR || status != gl.FRAMEBUFFER_COMPLETE_EXT {
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
	gl.DeleteBuffers(1, &b.vertex)
	gl.DeleteBuffers(1, &b.index)
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
	gl.GenBuffers(1, &b.vertex)
	gl.BindBuffer(gl.ARRAY_BUFFER, b.vertex)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)
	gl.GenBuffers(1, &b.index)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.index)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)
	if e := gl.GetError(); e != gl.NO_ERROR {
		gl.DeleteBuffers(1, &b.vertex)
		gl.DeleteBuffers(1, &b.index)
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
	if r == nil || r.closed {
		return nil, ErrClosed
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
	gl.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, r.fbo)
	gl.Viewport(0, 0, int32(w), int32(h))
	gl.Disable(gl.BLEND)
	gl.Disable(gl.DITHER)
	gl.Disable(gl.SCISSOR_TEST)
	gl.Disable(gl.STENCIL_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.MULTISAMPLE)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthMask(true)
	gl.DepthFunc(gl.LESS)
	gl.ColorMask(true, true, true, true)
	// Clear IDs separately: the visible background is not an object ID.
	gl.DrawBuffer(gl.COLOR_ATTACHMENT1_EXT)
	gl.ClearColor(0, 0, 0, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.DrawBuffer(gl.COLOR_ATTACHMENT0_EXT)
	gl.ClearColor(float32(bg.R)/255, float32(bg.G)/255, float32(bg.B)/255, 1)
	gl.ClearDepth(1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	attachments := [2]uint32{gl.COLOR_ATTACHMENT0_EXT, gl.COLOR_ATTACHMENT1_EXT}
	gl.DrawBuffers(2, &attachments[0])
	gl.UseProgram(r.program)
	gl.UniformMatrix4fv(r.vp, 1, false, &vp32[0])
	gl.EnableVertexAttribArray(0)
	for i := 0; i < s.InstanceCount(); i++ {
		in, _ := s.Instance(i)
		if in.Mesh.TriangleCount() == 0 {
			continue
		}
		b, e := r.upload(in.Mesh)
		if e != nil {
			return nil, e
		}
		if b.count == 0 {
			continue
		}
		world, _ := matrix32(in.Transform)
		gl.UniformMatrix4fv(r.world, 1, false, &world[0])
		gl.Uniform4f(r.material, float32(in.Color[0])/255, float32(in.Color[1])/255, float32(in.Color[2])/255, 1)
		id := i + 1
		gl.Uniform3f(r.objectID, float32(id&255)/255, float32((id>>8)&255)/255, float32((id>>16)&255)/255)
		gl.BindBuffer(gl.ARRAY_BUFFER, b.vertex)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.index)
		gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
		gl.DrawElementsWithOffset(gl.TRIANGLES, b.count, gl.UNSIGNED_INT, 0)
		r.frame.Stats.DrawCalls++
		r.frame.Stats.Triangles += int(b.count) / 3
	}
	gl.DisableVertexAttribArray(0)
	gl.UseProgram(0)
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	gl.PixelStorei(gl.PACK_ALIGNMENT, 4)
	gl.PixelStorei(gl.PACK_ROW_LENGTH, 0)
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0_EXT)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(r.readback))
	gl.ReadBuffer(gl.COLOR_ATTACHMENT1_EXT)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(r.frame.pickPixels))
	if e := gl.GetError(); e != gl.NO_ERROR {
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
	r.frame.Stats.ReadbackBytes = int64(w) * int64(h) * 8
	r.key, r.camera, r.bg = s, c, bg
	return &r.frame, nil
}
