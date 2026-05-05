package glui

// Soft drop-shadow primitive.
//
// Cards, dialogs, tooltips, and popups in the Cairo path render box
// shadows by blurring an offset rectangle. On the GL path we approximate
// the same effect with the rect SDF: outset the quad by the blur radius
// and let the rect shader's smoothstep AA do the falloff. The shader
// receives the original rect's half-size as the SDF's "inside" extent,
// while the AA-width parameter is enlarged to span the blur. The
// fragment that lies `blur` points outside the rect lands at SDF distance
// `blur` and the smoothstep clamps it to zero alpha — exactly the soft
// drop-shadow shape we want.
//
// Callers should draw the actual rectangle ON TOP of the shadow, never
// beneath. The shadow primitive itself does not punch out the centre
// region (doing so cleanly requires a stencil pass); for opaque shapes
// the rect-on-top render order makes the inner overdraw invisible. For
// translucent shapes, callers either tolerate the small overdraw or
// switch to a dedicated stencil-cut primitive.

// FillBoxShadow renders a soft drop shadow under a rectangle. The shadow
// occupies the area (rc expanded by blur) with falloff governed by blur
// (in points) and radius (rounded-corner radius matching the underlying
// shape). The host code should draw the actual rectangle ON TOP of the
// shadow, not beneath.
//
// Parameters:
//   rc     — rectangle whose silhouette the shadow follows
//   radius — corner radius of the rectangle (matches the shape on top)
//   blur   — feather distance in points; total spread is blur on every side
//   col    — shadow colour (typically black with reduced alpha)
//
// The blur parameter doubles as the shader's AA width, so larger blurs
// produce softer falloff while smaller blurs give a tight halo.
func (r *Renderer) FillBoxShadow(rc Rect, radius, blur float32, col Color) {
	// Degenerate inputs short-circuit cleanly: a zero-blur shadow has no
	// soft edge to render, and a zero-area rect carries no silhouette.
	if blur <= 0 || rc.W <= 0 || rc.H <= 0 {
		return
	}

	r.setBatch(kindRect, 0)

	// Outset the quad by blur on every side so the smoothstep ramp has
	// room to cover. The SDF half-size stays at the original rect's
	// half-extent — that fixes the contour where the smoothstep
	// transitions from full alpha (inside) to zero (outside the blur).
	inflate := blur
	rx := rc.X - inflate
	ry := rc.Y - inflate
	rw := rc.W + inflate*2
	rh := rc.H + inflate*2

	hw := rc.W * 0.5
	hh := rc.H * 0.5

	// Use the rect shader's AA-width parameter as the shadow blur. The
	// math: inside the original rect the SDF returns negative, the
	// smoothstep clamps to alpha=1; outside, the smoothstep ramp covers
	// `blur` points before reaching zero.
	r.pushRectQuad(rx, ry, rw, rh, hw, hh, radius, blur, col)
}
