// Package shader provides GLSL program compilation and uniform management
// for the glui renderer.
package shader

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-gl/gl/v2.1/gl"
)

// Program wraps a compiled+linked GLSL program.
type Program struct {
	id       uint32
	uniforms map[string]int32
	attribs  map[string]int32
}

// Compile compiles a vertex+fragment program. Both sources MUST already
// contain the GLSL version pragma the runtime needs.
func Compile(vertSrc, fragSrc string) (*Program, error) {
	vert, err := compileStage(gl.VERTEX_SHADER, vertSrc)
	if err != nil {
		return nil, fmt.Errorf("vertex: %w", err)
	}
	defer gl.DeleteShader(vert)

	frag, err := compileStage(gl.FRAGMENT_SHADER, fragSrc)
	if err != nil {
		return nil, fmt.Errorf("fragment: %w", err)
	}
	defer gl.DeleteShader(frag)

	id := gl.CreateProgram()
	gl.AttachShader(id, vert)
	gl.AttachShader(id, frag)
	gl.LinkProgram(id)

	var status int32
	gl.GetProgramiv(id, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(id, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(id, logLen, nil, gl.Str(log))
		gl.DeleteProgram(id)
		return nil, errors.New("link failed: " + log)
	}

	return &Program{
		id:       id,
		uniforms: make(map[string]int32),
		attribs:  make(map[string]int32),
	}, nil
}

func compileStage(kind uint32, src string) (uint32, error) {
	id := gl.CreateShader(kind)
	csrc, free := gl.Strs(src + "\x00")
	gl.ShaderSource(id, 1, csrc, nil)
	free()
	gl.CompileShader(id)

	var status int32
	gl.GetShaderiv(id, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(id, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(id, logLen, nil, gl.Str(log))
		gl.DeleteShader(id)
		return 0, errors.New(log)
	}
	return id, nil
}

// Use binds the program for subsequent draw calls.
func (p *Program) Use() { gl.UseProgram(p.id) }

// ID returns the underlying GL program object.
func (p *Program) ID() uint32 { return p.id }

// Uniform looks up (and caches) the location of a uniform by name.
// Returns -1 if the uniform was optimised out, which is harmless for
// most subsequent calls.
func (p *Program) Uniform(name string) int32 {
	if loc, ok := p.uniforms[name]; ok {
		return loc
	}
	cname, free := gl.Strs(name + "\x00")
	loc := gl.GetUniformLocation(p.id, *cname)
	free()
	p.uniforms[name] = loc
	return loc
}

// Attrib looks up a vertex attribute location.
func (p *Program) Attrib(name string) int32 {
	if loc, ok := p.attribs[name]; ok {
		return loc
	}
	cname, free := gl.Strs(name + "\x00")
	loc := gl.GetAttribLocation(p.id, *cname)
	free()
	p.attribs[name] = loc
	return loc
}

// Delete frees the program. Subsequent calls are no-ops.
func (p *Program) Delete() {
	if p.id != 0 {
		gl.DeleteProgram(p.id)
		p.id = 0
	}
}

// Set2f sets a vec2 uniform.
func (p *Program) Set2f(name string, x, y float32) {
	gl.Uniform2f(p.Uniform(name), x, y)
}

// Set4f sets a vec4 uniform.
func (p *Program) Set4f(name string, x, y, z, w float32) {
	gl.Uniform4f(p.Uniform(name), x, y, z, w)
}

// Set1i sets an int/sampler uniform.
func (p *Program) Set1i(name string, v int32) {
	gl.Uniform1i(p.Uniform(name), v)
}

// SetMat3 sets a mat3 uniform from a 9-float column-major slice.
func (p *Program) SetMat3(name string, m [9]float32) {
	gl.UniformMatrix3fv(p.Uniform(name), 1, false, &m[0])
}
