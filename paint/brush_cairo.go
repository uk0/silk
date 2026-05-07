//go:build !silk_no_cairo

package paint

import (
	"runtime"

	"silk/cairo"
)

var cairoPatternCount = 0

// PixmapBrush in the Cairo build holds both the source pixmap (for
// non-Cairo backends like glui) and a cairo.Pattern wrapping the
// pixmap's underlying surface (for cairo_set_source). The two
// representations are kept in sync at SetExtend time.
type PixmapBrush struct {
	pat    *cairo.Pattern
	pixmap Pixmap
	extend Extend
}

func (this *PixmapBrush) setFinalizer() {
	cairoPatternCount++
	runtime.SetFinalizer(this, func(p *PixmapBrush) {
		p.pat.Destroy()
		cairoPatternCount--
	})
}

// NewPixmapBrush constructs a Cairo-backed pattern brush. Type-asserts
// the supplied Pixmap to *cairoSurface — every Pixmap created via
// paint.NewPixmap or paint.LoadPngFile is backed by cairoSurface in
// the Cairo build, so the assertion always succeeds. The pure-Go
// build has its own NewPixmapBrush in brush_pure.go.
func NewPixmapBrush(pixmap Pixmap) *PixmapBrush {
	s := pixmap.(*cairoSurface)
	p := cairo.NewPatternForSurface(s.Surface)
	br := new(PixmapBrush)
	br.pat = p
	br.pixmap = pixmap
	br.setFinalizer()
	return br
}

func (this *PixmapBrush) Extend() Extend {
	if this.pat != nil {
		return Extend(this.pat.Extend())
	}
	return this.extend
}

func (this *PixmapBrush) SetExtend(ext Extend) {
	this.extend = ext
	if this.pat != nil {
		this.pat.SetExtend(cairo.Extend(ext))
	}
}
