//go:build !silk_no_cairo

package paint

import (
	"runtime"

	"silk/cairo"
	"silk/core"
)

// cairoSurface is the Cairo-backed Surface / Pixmap implementation.
// Lives in this Cairo-tagged file so a silk_no_cairo build skips it
// entirely; the silk_no_cairo build provides imagePixmap (in
// pixmap_pure.go) which also satisfies Surface + Pixmap.
//
// Held externally only via the public interfaces — no caller outside
// paint references *cairoSurface directly. NewPixmap / LoadPngFile /
// the Win32 helpers all return Pixmap interface values.

var cairoSurfaceCount = 0

type cairoSurface struct {
	*cairo.Surface
}

func (this *cairoSurface) setFinalizer() {
	cairoSurfaceCount++
	if cairoSurfaceCount > 2000 && cairoSurfaceCount%100 == 0 {
		core.Warn("seems cairo surface leaks, count = ", cairoSurfaceCount)
	}
	runtime.SetFinalizer(this, func(p *cairoSurface) {
		p.Surface.Destroy()
		cairoSurfaceCount--
	})
}

func (this *cairoSurface) SurfaceType() int {
	return int(this.Surface.Type())
}

func (this *cairoSurface) NewPainter() Painter {
	painter := new(cairoPainter)
	painter.setFinalizer()
	painter.cairo = this.Surface.NewContext()
	painter.SetPen(NewPen(Color{0, 0, 0, 255}, 1))
	return painter
}
