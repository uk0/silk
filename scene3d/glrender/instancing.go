//go:build cgo && !silk_pure_go

package glrender

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/scene3d"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
)

// Bounded staging avoids a per-frame allocation proportional to the scene. Runs
// are split, never regrouped by mesh: equal-depth winners and original IDs stay
// in snapshot order, including across different mesh objects and culled holes.
const maxInstanceBatch = 1024
const instanceFloats = 24
const instanceStride = instanceFloats * 4

const instancedVertexShader = `#version 120
attribute vec3 position;
attribute vec4 world0;
attribute vec4 world1;
attribute vec4 world2;
attribute vec4 world3;
attribute vec4 instanceMaterial;
attribute vec3 instanceID;
uniform mat4 viewProjection;
varying vec3 worldPosition;
varying vec4 material;
varying vec3 objectID;
void main() {
    mat4 world = mat4(world0, world1, world2, world3);
    vec4 p = world * vec4(position, 1.0);
    worldPosition = p.xyz;
    material = instanceMaterial;
    objectID = instanceID;
    gl_Position = viewProjection * p;
}`

func (r *Renderer) initInstancing(mode InstancingMode) error {
	if mode == InstancingDisabled {
		return nil
	}
	if !r.api.SupportsInstancing() {
		if mode == InstancingRequired {
			return fmt.Errorf("%w: instanced arrays/draws, flat varying support and 7 attributes required", ErrUnavailable)
		}
		return nil
	}
	vertex, frag := instancingShaderSources(strings.Contains(" "+r.api.GetString(gl.EXTENSIONS)+" ", " GL_EXT_gpu_shader4 "))
	p, err := compileProgram(r.api, vertex, frag, []string{"position", "world0", "world1", "world2", "world3", "instanceMaterial", "instanceID"})
	if err != nil {
		return err
	}
	r.instanceProgram = p
	r.instanceVP = r.api.GetUniformLocation(p, "viewProjection")
	if r.instanceVP < 0 {
		return fmt.Errorf("%w: instanced viewProjection uniform", ErrUnavailable)
	}
	r.capabilities.InstancedDrawing = true
	return nil
}

func instancingShaderSources(useExtension bool) (string, string) {
	frag := strings.ReplaceAll(strings.ReplaceAll(fragmentShader, "uniform vec4 material;", "varying vec4 material;"), "uniform vec3 objectID;", "varying vec3 objectID;")
	vertex := instancedVertexShader
	// Never interpolate instance-constant values: Mesa exposed one-LSB color
	// changes with ordinary varyings even when all three vertices agreed.
	if useExtension {
		vertex = strings.Replace(vertex, "#version 120", "#version 120\n#extension GL_EXT_gpu_shader4 : require", 1)
		frag = strings.Replace(frag, "#version 120", "#version 120\n#extension GL_EXT_gpu_shader4 : require", 1)
		vertex = strings.ReplaceAll(strings.ReplaceAll(vertex, "varying vec4 material;", "flat varying vec4 material;"), "varying vec3 objectID;", "flat varying vec3 objectID;")
		frag = strings.ReplaceAll(strings.ReplaceAll(frag, "varying vec4 material;", "flat varying vec4 material;"), "varying vec3 objectID;", "flat varying vec3 objectID;")
	} else {
		// Core GL 3.x need not advertise the older extension string.
		vertex = strings.Replace(vertex, "#version 120", "#version 130", 1)
		frag = strings.Replace(frag, "#version 120", "#version 130", 1)
		vertex = strings.ReplaceAll(strings.ReplaceAll(vertex, "attribute ", "in "), "varying ", "out ")
		frag = strings.ReplaceAll(frag, "varying ", "in ")
		vertex = strings.ReplaceAll(strings.ReplaceAll(vertex, "out vec4 material;", "flat out vec4 material;"), "out vec3 objectID;", "flat out vec3 objectID;")
		frag = strings.ReplaceAll(strings.ReplaceAll(frag, "in vec4 material;", "flat in vec4 material;"), "in vec3 objectID;", "flat in vec3 objectID;")
	}
	return vertex, frag
}

func (r *Renderer) Capabilities() Capabilities { return r.capabilities }

func (r *Renderer) useDrawProgram(instanced bool, vp *[16]float32) {
	program, location := r.program, r.vp
	if instanced {
		program, location = r.instanceProgram, r.instanceVP
	}
	if r.drawProgram == program {
		return
	}
	if r.drawProgram == r.instanceProgram && r.instanceProgram != 0 {
		for i := uint32(1); i <= 6; i++ {
			r.api.DisableVertexAttribArray(i)
			r.api.VertexAttribDivisor(i, 0)
		}
	}
	if instanced {
		for i := uint32(1); i <= 6; i++ {
			r.api.EnableVertexAttribArray(i)
			r.api.VertexAttribDivisor(i, 1)
		}
	}
	r.api.UseProgram(program)
	r.api.UniformMatrix4fv(location, 1, false, &vp[0])
	r.drawProgram = program
}

func (r *Renderer) finishDraw() {
	if r.drawProgram == r.instanceProgram && r.instanceProgram != 0 {
		for i := uint32(1); i <= 6; i++ {
			r.api.DisableVertexAttribArray(i)
			r.api.VertexAttribDivisor(i, 0)
		}
	}
	r.api.DisableVertexAttribArray(0)
	r.api.UseProgram(0)
	r.drawProgram = 0
}

func appendInstance(data []float32, in scene3d.RenderInstance, id int) []float32 {
	world, _ := matrix32(in.Transform) // validated before any previous-frame mutation
	data = append(data, world[:]...)
	data = append(data, float32(in.Color[0])/255, float32(in.Color[1])/255, float32(in.Color[2])/255, 1)
	return append(data, float32(id&255)/255, float32((id>>8)&255)/255, float32((id>>16)&255)/255, 0)
}

func (r *Renderer) drawSingle(in scene3d.RenderInstance, id int, vp *[16]float32) error {
	b, err := r.upload(in.Mesh)
	if err != nil {
		return err
	}
	r.useDrawProgram(false, vp)
	world, _ := matrix32(in.Transform)
	r.api.UniformMatrix4fv(r.world, 1, false, &world[0])
	r.api.Uniform4f(r.material, float32(in.Color[0])/255, float32(in.Color[1])/255, float32(in.Color[2])/255, 1)
	r.api.Uniform3f(r.objectID, float32(id&255)/255, float32((id>>8)&255)/255, float32((id>>16)&255)/255)
	r.api.BindBuffer(gl.ARRAY_BUFFER, b.vertex)
	r.api.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.index)
	r.api.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
	r.api.DrawElementsWithOffset(gl.TRIANGLES, b.count, gl.UNSIGNED_INT, 0)
	r.frame.Stats.DrawCalls++
	r.frame.Stats.Instances++
	r.frame.Stats.Triangles += int(b.count) / 3
	return nil
}

func (r *Renderer) drawBatch(mesh *scene3d.Mesh, count int, vp *[16]float32) error {
	b, err := r.upload(mesh)
	if err != nil {
		return err
	}
	if r.instanceBuffer == 0 {
		r.api.GenBuffers(1, &r.instanceBuffer)
	}
	r.useDrawProgram(true, vp)
	r.api.BindBuffer(gl.ARRAY_BUFFER, r.instanceBuffer)
	size := len(r.instanceData) * 4
	r.api.BufferData(gl.ARRAY_BUFFER, size, gl.Pointer(r.instanceData), gl.STREAM_DRAW)
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return fmt.Errorf("%w: instance upload %#x", ErrUnavailable, e)
	}
	r.instanceBufferBytes = int64(size)
	for i := uint32(1); i <= 6; i++ {
		r.api.VertexAttribPointerWithOffset(i, 4, gl.FLOAT, false, instanceStride, uintptr(i-1)*16)
	}
	r.api.BindBuffer(gl.ARRAY_BUFFER, b.vertex)
	r.api.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.index)
	r.api.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
	r.api.DrawElementsInstancedWithOffset(gl.TRIANGLES, b.count, gl.UNSIGNED_INT, 0, int32(count))
	r.frame.Stats.DrawCalls++
	r.frame.Stats.InstancedDrawCalls++
	r.frame.Stats.InstancedInstances += count
	r.frame.Stats.Instances += count
	r.frame.Stats.Triangles += int(b.count) / 3 * count
	r.frame.Stats.InstanceUploadBytes += int64(size)
	return nil
}

func (r *Renderer) drawInstances(s *scene3d.Snapshot, vp scene3d.Mat4, vp32 *[16]float32) error {
	r.visible, r.frame.Stats.Culling = r.culler.Cull(r.visible, s, scene3d.NewFrustum(vp))
	r.frame.Stats.CulledInstances = r.frame.Stats.Culling.CulledInstances
	r.frame.Stats.CulledTriangles = r.frame.Stats.Culling.CulledTriangles
	var first scene3d.RenderInstance
	firstID, count := 0, 0
	r.instanceData = r.instanceData[:0]
	flush := func() error {
		if count == 0 {
			return nil
		}
		var err error
		if count == 1 {
			err = r.drawSingle(first, firstID, vp32)
		} else {
			err = r.drawBatch(first.Mesh, count, vp32)
		}
		count = 0
		r.instanceData = r.instanceData[:0]
		return err
	}
	for _, i := range r.visible {
		in, _ := s.Instance(i)
		if in.Mesh.TriangleCount() == 0 {
			continue
		}
		if r.instanceProgram == 0 {
			if err := r.drawSingle(in, i+1, vp32); err != nil {
				return err
			}
			continue
		}
		if count > 0 && (in.Mesh != first.Mesh || count == maxInstanceBatch) {
			if err := flush(); err != nil {
				return err
			}
		}
		if count == 0 {
			first = in
			firstID = i + 1
			count = 1
			continue
		}
		if count == 1 {
			if cap(r.instanceData) < maxInstanceBatch*instanceFloats {
				r.instanceData = make([]float32, 0, maxInstanceBatch*instanceFloats)
			}
			r.instanceData = appendInstance(r.instanceData, first, firstID)
		}
		r.instanceData = appendInstance(r.instanceData, in, i+1)
		count++
	}
	return flush()
}
