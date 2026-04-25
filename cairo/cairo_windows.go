package cairo

// #include <cairo/cairo-win32.h>
import "C"

import (
	"unsafe"
)

func NewWin32Surface(dc uintptr) *Surface {
	s := C.cairo_win32_surface_create((*_Ctype_struct_HDC__)(unsafe.Pointer(dc)))
	return (*Surface)(s)
}

func NewWin32PrintingSurface(dc uintptr) *Surface {
	s := C.cairo_win32_printing_surface_create((*_Ctype_struct_HDC__)(unsafe.Pointer(dc)))
	return (*Surface)(s)
}

func (this *Surface) HDC() uintptr {
	h := C.cairo_win32_surface_get_dc(surface_t(this))
	return uintptr(unsafe.Pointer(h))
}

func NewWin32DibSurface(format Format, width, height int) *Surface {
	s := C.cairo_win32_surface_create_with_dib(C.cairo_format_t(format), C.int(width), C.int(height))
	return (*Surface)(s)
}
