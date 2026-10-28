//go:build ignore

// This small generator is deliberately independent of go-gl/glow. Its input is
// the OpenGL 2.1/EXT ABI subset used by Silk's scene renderer, not a full registry.
package main

import (
	"fmt"
	"go/format"
	"os"
	"strings"
)

type abiType struct{ c, goType, cast string }

var types = map[string]abiType{
	"void": {"void", "", ""}, "u32": {"uint32_t", "uint32", "C.uint32_t(%s)"},
	"i32": {"int32_t", "int32", "C.int32_t(%s)"}, "f32": {"float", "float32", "C.float(%s)"},
	"f64": {"double", "float64", "C.double(%s)"}, "bool": {"uint8_t", "bool", "C.uint8_t(boolByte(%s))"},
	"size":  {"ptrdiff_t", "int", "C.ptrdiff_t(%s)"},
	"cu32p": {"const uint32_t *", "*uint32", "(*C.uint32_t)(unsafe.Pointer(%s))"},
	"ci32p": {"const int32_t *", "*int32", "(*C.int32_t)(unsafe.Pointer(%s))"},
	"u32p":  {"uint32_t *", "*uint32", "(*C.uint32_t)(unsafe.Pointer(%s))"},
	"i32p":  {"int32_t *", "*int32", "(*C.int32_t)(unsafe.Pointer(%s))"},
	"f32p":  {"const float *", "*float32", "(*C.float)(unsafe.Pointer(%s))"},
	"bytep": {"char *", "*byte", "(*C.char)(unsafe.Pointer(%s))"},
	"ptr":   {"void *", "unsafe.Pointer", "%s"}, "cptr": {"const void *", "unsafe.Pointer", "%s"},
	"offset": {"const void *", "uintptr", "C.uintptr_t(%s)"},
	"str":    {"const char *", "string", "c_%s"}, "strs": {"const char *const *", "", ""},
	"bytestr": {"const uint8_t *", "string", ""},
}

// Parameter names avoid C/Go keywords. Offset parameters are passed as integers
// across cgo and cast inside C, never forged into Go pointers under checkptr.
const spec = `
GetString bytestr name:u32
GetIntegerv void pname:u32 data:i32p
GetError u32
CreateShader u32 kind:u32
ShaderSource void shader:u32 count:i32 sources:strs lengths:ci32p
CompileShader void shader:u32
GetShaderiv void shader:u32 pname:u32 params:i32p
GetShaderInfoLog void shader:u32 maxLength:i32 length:i32p info:bytep
DeleteShader void shader:u32
CreateProgram u32
AttachShader void program:u32 shader:u32
BindAttribLocation void program:u32 index:u32 name:str
LinkProgram void program:u32
GetProgramiv void program:u32 pname:u32 params:i32p
GetProgramInfoLog void program:u32 maxLength:i32 length:i32p info:bytep
DeleteProgram void program:u32
GetUniformLocation i32 program:u32 name:str
GenFramebuffersEXT void n:i32 framebuffers:u32p
BindFramebufferEXT void target:u32 framebuffer:u32
DeleteFramebuffersEXT void n:i32 framebuffers:cu32p
GenRenderbuffersEXT void n:i32 renderbuffers:u32p
BindRenderbufferEXT void target:u32 renderbuffer:u32
DeleteRenderbuffersEXT void n:i32 renderbuffers:cu32p
RenderbufferStorageEXT void target:u32 internalformat:u32 width:i32 height:i32
FramebufferRenderbufferEXT void target:u32 attachment:u32 renderbufferTarget:u32 renderbuffer:u32
FramebufferTexture2DEXT void target:u32 attachment:u32 textureTarget:u32 texture:u32 level:i32
CheckFramebufferStatusEXT u32 target:u32
GenTextures void n:i32 textures:u32p
BindTexture void target:u32 texture:u32
DeleteTextures void n:i32 textures:cu32p
TexParameteri void target:u32 pname:u32 param:i32
TexImage2D void target:u32 level:i32 internalformat:i32 width:i32 height:i32 border:i32 format:u32 kind:u32 pixels:cptr
GenBuffers void n:i32 buffers:u32p
BindBuffer void target:u32 buffer:u32
BufferData void target:u32 size:size data:cptr usage:u32
DeleteBuffers void n:i32 buffers:cu32p
Viewport void x:i32 y:i32 width:i32 height:i32
Disable void cap:u32
Enable void cap:u32
DepthMask void flag:bool
DepthFunc void fn:u32
ColorMask void red:bool green:bool blue:bool alpha:bool
DrawBuffer void buf:u32
DrawBuffers void n:i32 bufs:cu32p
ClearColor void red:f32 green:f32 blue:f32 alpha:f32
ClearDepth void depth:f64
Clear void mask:u32
UseProgram void program:u32
UniformMatrix4fv void location:i32 count:i32 transpose:bool value:f32p
Uniform4f void location:i32 x:f32 y:f32 z:f32 w:f32
Uniform3f void location:i32 x:f32 y:f32 z:f32
EnableVertexAttribArray void index:u32
DisableVertexAttribArray void index:u32
VertexAttribPointerWithOffset void index:u32 size:i32 kind:u32 normalized:bool stride:i32 offset:offset
DrawElementsWithOffset void mode:u32 count:i32 kind:u32 offset:offset
PixelStorei void pname:u32 param:i32
ReadBuffer void src:u32
ReadPixels void x:i32 y:i32 width:i32 height:i32 format:u32 kind:u32 pixels:ptr
`

func main() {
	var fields, wrappers, loaders, methods strings.Builder
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(spec), "\n") {
		f := strings.Fields(line)
		name, ret := f[0], types[f[1]]
		native := strings.TrimSuffix(name, "WithOffset")
		var cParams, wrapperParams, callArgs, goParams, goArgs, setup []string
		for _, p := range f[2:] {
			pair := strings.Split(p, ":")
			n, k := pair[0], pair[1]
			typ, ok := types[k]
			if !ok {
				panic(k)
			}
			cParams = append(cParams, typ.c+" "+n)
			if k == "offset" {
				wrapperParams = append(wrapperParams, "uintptr_t "+n)
				callArgs = append(callArgs, "(const void *)"+n)
			} else {
				wrapperParams = append(wrapperParams, typ.c+" "+n)
				callArgs = append(callArgs, n)
			}
			goParams = append(goParams, n+" "+typ.goType)
			goArgs = append(goArgs, fmt.Sprintf(typ.cast, n))
			if k == "str" {
				setup = append(setup, fmt.Sprintf("c_%s := C.CString(%s); defer C.free(unsafe.Pointer(c_%s))", n, n, n))
			}
		}
		cp := strings.Join(cParams, ", ")
		if cp == "" {
			cp = "void"
		}
		fmt.Fprintf(&fields, "    %s (SILK_GL_CALL *%s)(%s);\n", ret.c, name, cp)
		fmt.Fprintf(&loaders, "    SILK_LOAD(%s, \"gl%s\");\n", name, native)
		count++
		if name == "ShaderSource" {
			continue
		}
		args := append([]string{"silk_gl_api *api"}, wrapperParams...)
		retWord := ""
		if f[1] != "void" {
			retWord = "return "
		}
		fmt.Fprintf(&wrappers, "static inline %s silk_%s(%s) { %sapi->%s(%s); }\n", ret.c, name, strings.Join(args, ", "), retWord, name, strings.Join(callArgs, ", "))
		fmt.Fprintf(&methods, "func (api *API) %s(%s) %s {\n", name, strings.Join(goParams, ", "), ret.goType)
		for _, s := range setup {
			fmt.Fprintln(&methods, s)
		}
		call := fmt.Sprintf("C.silk_%s(%s)", name, strings.Join(append([]string{"api.table"}, goArgs...), ", "))
		switch f[1] {
		case "void":
			fmt.Fprintln(&methods, call)
		case "bytestr":
			fmt.Fprintf(&methods, "return C.GoString((*C.char)(unsafe.Pointer(%s)))\n", call)
		default:
			fmt.Fprintf(&methods, "return %s(%s)\n", ret.goType, call)
		}
		fmt.Fprintln(&methods, "}")
	}
	header := fmt.Sprintf(`// Code generated by go run generate.go; DO NOT EDIT.
#ifndef SILK_GL21_BINDINGS_H
#define SILK_GL21_BINDINGS_H
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#if defined(_WIN32)
#define SILK_GL_CALL __stdcall
#else
#define SILK_GL_CALL
#endif
typedef void *(SILK_GL_CALL *silk_current_fn)(void);
typedef void *(SILK_GL_CALL *silk_lookup_fn)(const char *);
typedef struct silk_gl_api {
    void *library;
    void *context;
    uintptr_t thread;
    silk_current_fn getCurrent;
    silk_lookup_fn getProc;
%s} silk_gl_api;
silk_gl_api *silk_gl_open(char *error, size_t capacity);
void silk_gl_close(silk_gl_api *api);
int silk_gl_current(silk_gl_api *api);
uintptr_t silk_gl_thread(void);
void silk_gl_shader_source(silk_gl_api *api, uint32_t shader, const char *source, int32_t length);
%s
#endif
`, fields.String(), wrappers.String())
	write("bindings.h", []byte(header))
	write("load.inc", []byte("// Code generated by go run generate.go; DO NOT EDIT.\n"+loaders.String()))
	src := fmt.Sprintf(`//go:build cgo && !silk_pure_go

// Code generated by go run generate.go; DO NOT EDIT.
package gl21
/*
#include "bindings.h"
*/
import "C"
import "unsafe"
const EntryCount = %d
%s`, count, methods.String())
	formatted, e := format.Source([]byte(src))
	if e != nil {
		panic(e)
	}
	write("bindings.go", formatted)
}
func write(name string, b []byte) {
	if e := os.WriteFile(name, b, 0644); e != nil {
		panic(e)
	}
}
