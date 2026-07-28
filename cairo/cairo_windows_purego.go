//go:build silk_pure_go

package cairo

// Win32 surface constructors for the pure-Go build.
//
// paint/paint_windows.go is selected by filename on every Windows build and
// calls these unconditionally, but the real implementations
// (cairo/cairo_windows.go) are cgo-only — they wrap cairo_win32_surface_create
// from cairo-win32.h. Without these stubs the pure-Go configuration simply does
// not compile on Windows.
//
// The pure-Go rasteriser has no HDC-backed surface: it renders into its own
// image buffers, and the Windows backend blits those. Drawing straight onto a
// device context is therefore unsupported here and both constructors return
// nil; callers that genuinely need a Win32 surface must use the default cgo
// build.

// NewWin32Surface is unsupported in the pure-Go build and returns nil.
func NewWin32Surface(dc uintptr) *Surface { return nil }

// NewWin32PrintingSurface is unsupported in the pure-Go build and returns nil.
func NewWin32PrintingSurface(dc uintptr) *Surface { return nil }
