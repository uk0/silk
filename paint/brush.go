package paint

import (
	"silk/cairo"
	"runtime"
)

var cairoPatternCount = 0

type Brush interface {
}

type SolidBrush struct {
	Color Color
}

func NewSolidBrush(cr Color) *SolidBrush {
	return &SolidBrush{cr}
}

type PixmapBrush struct {
	pat *cairo.Pattern
}

func (this *PixmapBrush) setFinalizer() {
	cairoPatternCount++
	runtime.SetFinalizer(this, func(p *PixmapBrush) {
		p.pat.Destroy()
		cairoPatternCount--
	})
}

func NewPixmapBrush(pixmap Pixmap) *PixmapBrush {
	s := pixmap.(*cairoSurface)
	p := cairo.NewPatternForSurface(s.Surface)
	br := new(PixmapBrush)
	br.pat = p
	br.setFinalizer()
	return br
}

func (this *PixmapBrush) Extend() Extend {
	return Extend(this.pat.Extend())
}

func (this *PixmapBrush) SetExtend(ext Extend) {
	this.pat.SetExtend(cairo.Extend(ext))
}
