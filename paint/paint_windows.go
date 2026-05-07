//go:build !silk_no_cairo

package paint

import (
	"silk/cairo"
)

// NewWin32Surface wraps a Win32 device context as a Cairo surface.
// Windows-specific bridge for the Cairo build; the silk_no_cairo
// build uses GLFW + glui throughout, so these helpers are absent.
func NewWin32Surface(dc uintptr) Pixmap {
	s := cairo.NewWin32Surface(dc)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}

func NewWin32PrintingSurface(dc uintptr) Pixmap {
	s := cairo.NewWin32PrintingSurface(dc)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}
