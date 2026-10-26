package gui

import "github.com/uk0/silk/paint"

// drawPopupSurface paints a full inset border. Transparent surfaces retain
// rounded antialiased edges; opaque surfaces have a fully covered rectangle.
// No shadow is painted outside the exact native window bounds.
func drawPopupSurface(g paint.Painter, w, h float64, bg, border paint.Color, radius float64) {
	if w <= 0 || h <= 0 {
		return
	}
	if radius <= 0 {
		bg.A = 255 // an opaque native target must never inherit stale backing pixels
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(bg)
		g.Fill()
		if w < 1 || h < 1 {
			return
		}
		g.Rectangle(.5, .5, w-1, h-1)
	} else {
		if w < 1 || h < 1 {
			return
		}
		roundedRect(g, .5, .5, w-1, h-1, radius)
		g.SetBrush1(bg)
		g.FillPreserve()
	}
	g.SetPen1(border, 1)
	g.Stroke()
}
