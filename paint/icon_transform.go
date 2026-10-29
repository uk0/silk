package paint

import "github.com/uk0/silk/cairo"

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
