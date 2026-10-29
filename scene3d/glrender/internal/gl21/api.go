//go:build cgo && !silk_pure_go

// Package gl21 owns the deliberately small OpenGL ABI surface used by glrender.
// Each table belongs to one native context and thread; there is no mutable global
// GL entry table. Native context creation remains the caller's responsibility.
package gl21

/*
#cgo linux LDFLAGS: -ldl -lpthread
#cgo freebsd netbsd openbsd LDFLAGS: -lpthread
#include "bindings.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

//go:generate go run generate.go

type API struct {
	table  *C.silk_gl_api
	thread uintptr
}

func OpenCurrent() (*API, error) {
	var message [512]byte
	table := C.silk_gl_open((*C.char)(unsafe.Pointer(&message[0])), C.size_t(len(message)))
	if table == nil {
		return nil, errors.New(C.GoString((*C.char)(unsafe.Pointer(&message[0]))))
	}
	return &API{table: table, thread: uintptr(table.thread)}, nil
}

// OnThread reads immutable ownership data before touching the native table.
// A rejected worker cannot race a concurrent owner-thread Close/free.
func (api *API) OnThread() bool { return api != nil && api.thread == uintptr(C.silk_gl_thread()) }
func (api *API) Current() bool {
	return api.OnThread() && api.table != nil && C.silk_gl_current(api.table) != 0
}

// Close releases only the loader/table, not the GL resources or native context.
// The renderer must delete its GL objects before disposing this table.
func (api *API) Close() {
	if api.OnThread() && api.table != nil {
		C.silk_gl_close(api.table)
		api.table = nil
	}
}
func (api *API) ShaderSource(shader uint32, source string) {
	text := C.CString(source)
	defer C.free(unsafe.Pointer(text))
	C.silk_gl_shader_source(api.table, C.uint32_t(shader), text, C.int32_t(len(source)))
}

// SupportsInstancing is queried on the owning context; addresses alone do not
// prove support (GLX can return dispatch stubs for unsupported entrypoints).
func (api *API) SupportsInstancing() bool {
	if !api.Current() {
		return false
	}
	var attributes int32
	api.GetIntegerv(MAX_VERTEX_ATTRIBS, &attributes)
	return supportsInstancing(api.GetString(VERSION), api.GetString(EXTENSIONS), attributes, C.silk_gl_instancing_entries(api.table) != 0)
}
func boolByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
func Pointer[T ~byte | ~uint32 | ~float32](values []T) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(values))
}
