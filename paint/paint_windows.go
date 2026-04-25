package paint

import (
	"silk/cairo"
)

func NewWin32Surface(dc uintptr) *cairoSurface {
	s := cairo.NewWin32Surface(dc)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}

func NewWin32PrintingSurface(dc uintptr) *cairoSurface {
	s := cairo.NewWin32PrintingSurface(dc)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}
