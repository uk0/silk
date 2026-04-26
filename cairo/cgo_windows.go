// Windows cgo flags for Cairo.
//
// On Windows, pkg-config is often unavailable or unreliable. We default to
// MSYS2 / MinGW-w64 layout (C:\msys64\mingw64) and additionally honor
// whatever CGO_CFLAGS / CGO_LDFLAGS the developer has already set.
//
// If you installed Cairo to a different prefix, set:
//
//	set CGO_CFLAGS=-IC:/your/cairo/include
//	set CGO_LDFLAGS=-LC:/your/cairo/lib -lcairo

package cairo

// #cgo CFLAGS: -IC:/msys64/mingw64/include -IC:/msys64/mingw64/include/cairo
// #cgo LDFLAGS: -LC:/msys64/mingw64/lib -lcairo
import "C"
