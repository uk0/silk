package paint

// DrawShadowRect renders a soft drop shadow under a (possibly rounded)
// rectangle by stacking N offset rounded-rect fills with linearly
// falling alpha. Mirrors glui's `Renderer.FillBoxShadow` shape so the
// two backends present the same shadow API to host code:
//
//	glui:   r.FillBoxShadow(rc, radius, blur, color)
//	cairo:  paint.DrawShadowRect(g, x, y, w, h, radius, blur, color)
//
// The host should draw the actual rectangle ON TOP of the shadow,
// not beneath. The shadow does not punch out the centre — for
// opaque shapes the rect-on-top render order makes the inner
// overdraw invisible; for translucent shapes, callers either
// tolerate the overdraw or switch to a stencil-cut primitive (not
// available on the cairo path).
//
// Algorithm: blur is rounded to N=ceil(blur). We render N+1 layers
// outset by 0..N points, with alpha falling linearly from color.A at
// the innermost layer down to color.A/(N+1) at the outermost. The
// linear ramp approximates a Gaussian closely enough that designers
// don't notice the difference at typical card-elevation blur values
// (2–8 points). For a sharper drop shadow pass blur ≤ 2.
//
// Degenerate inputs short-circuit cleanly:
//   - blur ≤ 0    → no shadow drawn
//   - w/h ≤ 0     → no silhouette
//   - color.A = 0 → invisible shadow, skip
func DrawShadowRect(g Painter, x, y, w, h, radius, blur float64, color Color) {
	if blur <= 0 || w <= 0 || h <= 0 || color.A == 0 {
		return
	}
	layers := int(blur + 0.5)
	if layers < 1 {
		layers = 1
	}
	denom := float64(layers + 1)
	for i := 0; i <= layers; i++ {
		outset := float64(i)
		alpha := float64(color.A) * (float64(layers+1) - float64(i)) / denom / denom
		if alpha < 1 {
			continue
		}
		layerColor := Color{color.R, color.G, color.B, uint8(alpha)}
		drawRoundedRectPath(g, x-outset, y-outset, w+outset*2, h+outset*2, radius+outset)
		g.SetBrush1(layerColor)
		g.Fill()
	}
}

// drawRoundedRectPath emits a rounded-rect sub-path on g without
// committing it (no Fill/Stroke). Pulled out so DrawShadowRect can
// reuse the same arc-corner shape host widgets use.
func drawRoundedRectPath(g Painter, x, y, w, h, r float64) {
	if r <= 0 {
		g.Rectangle(x, y, w, h)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	const piHalf = 1.5707963267948966
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -piHalf, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, piHalf)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, piHalf, 2*piHalf)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, 2*piHalf, 3*piHalf)
}
