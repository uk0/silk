package purecairo

import (
	"image"
	"image/color"
	"math"

	"silk/geom"
)

// Gradient implementations — linear and radial.
//
// Both linearGradient and radialGradient implement image.Image and
// return per-device-pixel colours. Endpoints / centres are pre-
// transformed to device space at SetSource time, so the per-pixel
// inner loop is only arithmetic — no matrix multiply per call.
//
// Reference: cairo/src/cairo-pattern.c. cairo's linear gradient
// projects the sample point onto the gradient axis; we do the same
// using a cached inverse-dot-product. cairo's two-circle radial is
// parametric in (1-t)*r0 + t*r1; we approximate with the simpler
// "distance-from-centre / outer-radius" form, accurate for the
// common silk avatar / card cases where r0 = 0.

// linearGradient maps device (x, y) → colour by projecting onto the
// transformed axis (p0 → p1) and looking up the stop list at the
// resulting parametric t.
type linearGradient struct {
	dx, dy   float64 // device delta from p0 to p1
	dot      float64 // dx*dx + dy*dy (cached for the early-out)
	invDot   float64 // 1 / dot, cached
	p0x, p0y float64 // device coords of the start endpoint
	stops    []gradStop
}

func newLinearGradient(ctm *geom.Mat3x2, p *Pattern) *linearGradient {
	x0, y0 := ctm.Transform(p.x0, p.y0)
	x1, y1 := ctm.Transform(p.x1, p.y1)
	dx := x1 - x0
	dy := y1 - y0
	dot := dx*dx + dy*dy
	invDot := 0.0
	if dot > 1e-12 {
		invDot = 1 / dot
	}
	return &linearGradient{
		dx: dx, dy: dy, dot: dot, invDot: invDot,
		p0x: x0, p0y: y0,
		stops: append([]gradStop(nil), p.stops...),
	}
}

func (g *linearGradient) ColorModel() color.Model { return color.RGBAModel }
func (g *linearGradient) Bounds() image.Rectangle {
	return image.Rect(-1<<30, -1<<30, 1<<30, 1<<30)
}
func (g *linearGradient) At(x, y int) color.Color { return g.RGBA64At(x, y) }
func (g *linearGradient) RGBA64At(x, y int) color.RGBA64 {
	if g.dot == 0 {
		return colorAtOffset(g.stops, 0)
	}
	fx, fy := float64(x), float64(y)
	t := ((fx-g.p0x)*g.dx + (fy-g.p0y)*g.dy) * g.invDot
	return colorAtOffset(g.stops, t)
}

// radialGradient maps device (x, y) → colour by distance from the
// outer-circle centre, normalised against (rOuter - rInner). cairo's
// full two-circle parametrisation is the obvious follow-up; the
// simplified version handles every gradient silk currently emits.
type radialGradient struct {
	cx, cy float64 // outer circle centre, device coords
	rOuter float64 // outer radius, device units
	rInner float64 // inner radius, device units
	stops  []gradStop
}

func newRadialGradient(ctm *geom.Mat3x2, p *Pattern) *radialGradient {
	cx, cy := ctm.Transform(p.cx1, p.cy1)
	// Rough device-radius using CTM linear scale.
	scale := math.Hypot(ctm.Xx, ctm.Yx)
	if scale == 0 {
		scale = 1
	}
	return &radialGradient{
		cx:     cx,
		cy:     cy,
		rOuter: p.r1 * scale,
		rInner: p.r0 * scale,
		stops:  append([]gradStop(nil), p.stops...),
	}
}

func (g *radialGradient) ColorModel() color.Model { return color.RGBAModel }
func (g *radialGradient) Bounds() image.Rectangle {
	return image.Rect(-1<<30, -1<<30, 1<<30, 1<<30)
}
func (g *radialGradient) At(x, y int) color.Color { return g.RGBA64At(x, y) }
func (g *radialGradient) RGBA64At(x, y int) color.RGBA64 {
	span := g.rOuter - g.rInner
	if span <= 0 {
		return colorAtOffset(g.stops, 0)
	}
	dx := float64(x) - g.cx
	dy := float64(y) - g.cy
	d := math.Sqrt(dx*dx + dy*dy)
	t := (d - g.rInner) / span
	return colorAtOffset(g.stops, t)
}

// colorAtOffset interpolates the stop list at parametric position t.
// Out-of-range t clamps to the nearest stop colour. Empty stop lists
// return transparent black so a forgotten AddColorStop doesn't paint
// uninitialised garbage.
func colorAtOffset(stops []gradStop, t float64) color.RGBA64 {
	if len(stops) == 0 {
		return color.RGBA64{}
	}
	if t <= stops[0].offset {
		return rgba8To64(stops[0].col)
	}
	if t >= stops[len(stops)-1].offset {
		return rgba8To64(stops[len(stops)-1].col)
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].offset {
			prev := stops[i-1]
			curr := stops[i]
			span := curr.offset - prev.offset
			if span < 1e-9 {
				return rgba8To64(curr.col)
			}
			f := (t - prev.offset) / span
			return blendRGBA(prev.col, curr.col, f)
		}
	}
	return rgba8To64(stops[len(stops)-1].col)
}

// blendRGBA linearly interpolates between two non-premultiplied RGBA
// colours and returns the result in 16-bit RGBA64. A more accurate
// blend would convert to linear-light first, but every silk caller
// passes opaque-or-fully-transparent stops, where the difference is
// invisible.
func blendRGBA(a, b color.RGBA, t float64) color.RGBA64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	one := 1 - t
	r := one*float64(a.R) + t*float64(b.R)
	g := one*float64(a.G) + t*float64(b.G)
	bb := one*float64(a.B) + t*float64(b.B)
	aa := one*float64(a.A) + t*float64(b.A)
	return color.RGBA64{
		R: uint16(r * 257),
		G: uint16(g * 257),
		B: uint16(bb * 257),
		A: uint16(aa * 257),
	}
}

func rgba8To64(c color.RGBA) color.RGBA64 {
	return color.RGBA64{
		R: uint16(c.R) * 257,
		G: uint16(c.G) * 257,
		B: uint16(c.B) * 257,
		A: uint16(c.A) * 257,
	}
}
