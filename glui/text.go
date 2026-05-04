package glui

// DrawText renders text starting at (x, y) — where y is the baseline — in
// the given font and color. Each glyph is positioned using the bearing
// values returned by font.Face.Glyph so descenders ('g', 'p', 'j') and
// short letters ('a', '.') sit correctly relative to the baseline.
func (r *Renderer) DrawText(f *Font, text string, x, y float32, col Color) {
	if f == nil || text == "" {
		return
	}

	// Pre-rasterise every glyph so the atlas texture is consistent for the
	// whole batch. If glyphs were rasterised inside the loop and the atlas
	// resized mid-flight, the texture upload would invalidate vertices we
	// already pushed.
	for _, ch := range text {
		f.Glyph(ch)
	}

	tex := f.Texture()
	r.setBatch(kindGlyph, tex)

	atlasW := float32(f.atlasW)
	atlasH := float32(f.atlasH)
	pen := x

	for _, ch := range text {
		g := f.Glyph(ch)
		if g.region.W == 0 || g.region.H == 0 {
			pen += g.advance
			continue
		}
		// UV in [0,1] over the atlas.
		u0 := float32(g.region.X) / atlasW
		v0 := float32(g.region.Y) / atlasH
		u1 := float32(g.region.X+g.region.W) / atlasW
		v1 := float32(g.region.Y+g.region.H) / atlasH

		// Position: face.Glyph returns dr.Min relative to the dot
		// (baseline pen). offY is negative for ascending glyphs and small
		// for short ones, so adding it lands the quad's top-left edge in
		// the right spot.
		gx := pen + g.offX
		gy := y + g.offY
		gw := float32(g.region.W)
		gh := float32(g.region.H)

		r.pushQuad(gx, gy, gw, gh, u0, v0, u1, v1, col)
		pen += g.advance
	}
}

// MeasureText returns the advance width of text in the given font. Equivalent
// to font.MeasureText(text); kept on Renderer for parity with DrawText.
func (r *Renderer) MeasureText(f *Font, text string) float32 {
	if f == nil {
		return 0
	}
	return f.MeasureText(text)
}
