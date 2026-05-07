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

// GradientStop is a single colour stop along a gradient axis.
type GradientStop struct {
	Offset float32
	Color  Color
}

// LinearGradient is a brush that interpolates colour along a straight axis.
type LinearGradient struct {
	X0, Y0, X1, Y1 float32
	Stops          []GradientStop
}

func NewLinearGradient(x0, y0, x1, y1 float32) *LinearGradient {
	return &LinearGradient{X0: x0, Y0: y0, X1: x1, Y1: y1}
}

func (g *LinearGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
}

// RadialGradient mirrors LinearGradient but radiates from a centre.
type RadialGradient struct {
	Cx, Cy, R0, R1 float32
	Stops          []GradientStop
}

func NewRadialGradient(cx, cy, r0, r1 float32) *RadialGradient {
	return &RadialGradient{Cx: cx, Cy: cy, R0: r0, R1: r1}
}

func (g *RadialGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
}
