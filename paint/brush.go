package paint

import (
	"github.com/uk0/silk/cairo"
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

	// Lazily built cairo pattern. Stays nil until cairoPainter.applyBrush
	// hits this brush, then is reused for every subsequent fill. The
	// finalizer destroys whatever pattern is current when the
	// LinearGradient is GC'd.
	pat       *cairo.Pattern
	finalizer bool // true once SetFinalizer has been installed
}

func NewLinearGradient(x0, y0, x1, y1 float32) *LinearGradient {
	return &LinearGradient{X0: x0, Y0: y0, X1: x1, Y1: y1}
}

func (g *LinearGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
	// Stop list changed; drop any cached cairo pattern so the next
	// applyBrush rebuilds. Cheap because applyBrush only runs once
	// per fill, not per stop set.
	if g.pat != nil {
		g.pat.Destroy()
		g.pat = nil
	}
}

// cairoPattern returns the cached cairo pattern, building it on first
// access. Each call to AddStop invalidates the cache so a freshly-
// edited gradient picks up the new stops.
func (g *LinearGradient) cairoPattern() *cairo.Pattern {
	if g.pat != nil {
		return g.pat
	}
	p := cairo.NewLinearPattern(float64(g.X0), float64(g.Y0), float64(g.X1), float64(g.Y1))
	for _, s := range g.Stops {
		r, gg, b, a := s.Color.NRGBAf()
		p.AddColorStopRGBA(float64(s.Offset), r, gg, b, a)
	}
	g.pat = p
	cairoPatternCount++
	if !g.finalizer {
		g.finalizer = true
		runtime.SetFinalizer(g, func(lg *LinearGradient) {
			if lg.pat != nil {
				lg.pat.Destroy()
				cairoPatternCount--
			}
		})
	}
	return p
}

// RadialGradient mirrors LinearGradient but radiates from a centre.
type RadialGradient struct {
	Cx, Cy, R0, R1 float32
	Stops          []GradientStop

	pat       *cairo.Pattern
	finalizer bool
}

func NewRadialGradient(cx, cy, r0, r1 float32) *RadialGradient {
	return &RadialGradient{Cx: cx, Cy: cy, R0: r0, R1: r1}
}

func (g *RadialGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
	if g.pat != nil {
		g.pat.Destroy()
		g.pat = nil
	}
}

func (g *RadialGradient) cairoPattern() *cairo.Pattern {
	if g.pat != nil {
		return g.pat
	}
	// Cairo's radial pattern is two-circle: inner (Cx,Cy,R0) → outer
	// (Cx,Cy,R1). The single-centre case we expose collapses to using
	// the same centre for both circles.
	p := cairo.NewRadialPattern(
		float64(g.Cx), float64(g.Cy), float64(g.R0),
		float64(g.Cx), float64(g.Cy), float64(g.R1),
	)
	for _, s := range g.Stops {
		r, gg, b, a := s.Color.NRGBAf()
		p.AddColorStopRGBA(float64(s.Offset), r, gg, b, a)
	}
	g.pat = p
	cairoPatternCount++
	if !g.finalizer {
		g.finalizer = true
		runtime.SetFinalizer(g, func(rg *RadialGradient) {
			if rg.pat != nil {
				rg.pat.Destroy()
				cairoPatternCount--
			}
		})
	}
	return p
}
