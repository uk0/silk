package paint

import (
	"github.com/uk0/silk/cairo"
	"math"
)

// Crisp vector axes share the scene manipulator's semantic XYZ colors and
// remain resolution-independent at fractional desktop scales.
func drawTransformMove(size int, cc *cairo.Context) {
	s := float64(size)
	cc.SetLineWidth(s * .085)
	for _, axis := range []struct{ x, y, r, g, b float64 }{
		{.84, .72, 245.0 / 255, 91.0 / 255, 97.0 / 255},
		{.48, .12, 85.0 / 255, 207.0 / 255, 141.0 / 255},
		{.14, .76, 84.0 / 255, 160.0 / 255, 1},
	} {
		cc.SetSourceRGB(axis.r, axis.g, axis.b)
		cc.MoveTo(s*.48, s*.56)
		cc.LineTo(s*axis.x, s*axis.y)
		cc.Stroke()
		cc.Rectangle(s*axis.x-s*.065, s*axis.y-s*.065, s*.13, s*.13)
		cc.Fill()
	}
}

func drawTransformRotate(size int, cc *cairo.Context) {
	s := float64(size)
	cc.SetLineWidth(s * .09)
	cc.SetSourceRGB(.33, .63, 1)
	cc.Arc(s*.5, s*.5, s*.31, -.8, 4.7)
	cc.Stroke()
	cc.SetSourceRGB(.33, .81, .55)
	cc.MoveTo(s*.51, s*.06)
	cc.LineTo(s*.68, s*.20)
	cc.LineTo(s*.46, s*.30)
	cc.ClosePath()
	cc.Fill()
	cc.SetSourceRGB(.96, .36, .38)
	cc.Arc(s*.5, s*.5, s*.08, 0, 2*math.Pi)
	cc.Fill()
}
func drawTransformScale(size int, cc *cairo.Context) {
	s := float64(size)
	cc.SetLineWidth(s * .075)
	cc.SetSourceRGB(.33, .63, 1)
	cc.Rectangle(s*.13, s*.55, s*.30, s*.30)
	cc.Stroke()
	cc.SetSourceRGB(.33, .81, .55)
	cc.Rectangle(s*.57, s*.13, s*.30, s*.30)
	cc.Stroke()
	cc.SetSourceRGB(.96, .36, .38)
	cc.MoveTo(s*.40, s*.60)
	cc.LineTo(s*.60, s*.40)
	cc.Stroke()
}
