package glui

// DrawText renders text starting at (x, y) — where y is the baseline — in
// the given font and color. Each glyph is positioned using the bearing
// values returned by font.Face.Glyph so descenders ('g', 'p', 'j') and
// short letters ('a', '.') sit correctly relative to the baseline.
//
// Two positioning strategies, picked by Font.SubpixelEnabled():
//
//   - Default (subpixel off): the quad lands at the exact float pen position
//     pen + offX. Texture LINEAR sampling smooths fractional offsets — text
//     looks slightly soft when the pen sits between pixels but stays visually
//     coherent and matches the historical baseline.
//   - Subpixel on: pen X is split into floor + fraction. The fraction is
//     quantised to numSubpixelBuckets buckets and the matching pre-rasterised
//     mask is fetched. The quad snaps to integer X so texture sampling
//     stays pixel-aligned — sharp text at any pen position. The rasteriser
//     does the subpixel work, not the texture filter.
func (r *Renderer) DrawText(f *Font, text string, x, y float32, col Color) {
	if f == nil || text == "" {
		return
	}
	if f.SubpixelLCDEnabled() {
		r.drawTextLCD(f, text, x, y, col)
		return
	}

	subpixel := f.SubpixelEnabled()

	// Pre-rasterise every glyph so the atlas texture is consistent for the
	// whole batch. If glyphs were rasterised inside the loop and the atlas
	// resized mid-flight, the texture upload would invalidate vertices we
	// already pushed. With subpixel on we rasterise the bucket-specific
	// variant the glyph will actually draw against — pre-pass and draw
	// pass must request the same key.
	{
		pen := x
		for _, ch := range text {
			frac := pen - float32(int32(pen))
			if frac < 0 {
				frac += 1
			}
			g := f.GlyphAt(ch, frac)
			pen += g.advance
		}
	}

	// Off-GL test mode: r.ctx == nil → skip the texture upload (which would
	// segfault on the gl.GenTextures call) and pass tex=0 to setBatch. The
	// downstream flush also short-circuits when ctx is nil, so the test still
	// observes vertex emission via r.verts without ever talking to GL.
	var tex uint32
	if r.ctx != nil {
		tex = f.Texture()
	}
	r.setBatch(kindGlyph, tex)

	atlasW := float32(f.atlasW)
	atlasH := float32(f.atlasH)
	pen := x

	for _, ch := range text {
		frac := pen - float32(int32(pen))
		if frac < 0 {
			frac += 1
		}
		g := f.GlyphAt(ch, frac)
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
		var gx float32
		if subpixel {
			// Snap to integer; the bucket-specific mask carries the
			// subpixel shift internally so texture sampling stays aligned.
			gx = float32(int32(pen)) + g.offX
			if pen < 0 && pen != float32(int32(pen)) {
				gx -= 1 // floor for negative non-integer pen
			}
		} else {
			gx = pen + g.offX
		}
		gy := y + g.offY
		gw := float32(g.region.W)
		gh := float32(g.region.H)

		r.pushQuad(gx, gy, gw, gh, u0, v0, u1, v1, col)
		pen += g.advance
	}
}

// drawTextLCD is the LCD-subpixel rendering path. The pen always snaps to
// integer X — sub-position is encoded in the per-pixel R/G/B channels of
// the glyph mask, not in a fractional quad position. Sub-bucket indexing
// is bypassed (the LCD cache is keyed by rune alone).
//
// The caller (DrawText) has already bypass-checked f for nil and text for
// empty so we can dive straight into the rasterise+emit loop.
func (r *Renderer) drawTextLCD(f *Font, text string, x, y float32, col Color) {
	// Pre-rasterise so the atlas texture is consistent for the whole batch
	// — same precaution as the alpha path. Glyph(ch) routes through
	// glyphForKey which dispatches to lcdRasteriseAndCache when LCD is on.
	for _, ch := range text {
		f.Glyph(ch)
	}

	var tex uint32
	if r.ctx != nil {
		tex = f.Texture()
	}
	r.setBatch(kindGlyphLCD, tex)

	atlasW := float32(f.atlasW)
	atlasH := float32(f.atlasH)
	pen := x

	for _, ch := range text {
		g := f.Glyph(ch)
		if g.region.W == 0 || g.region.H == 0 {
			pen += g.advance
			continue
		}
		u0 := float32(g.region.X) / atlasW
		v0 := float32(g.region.Y) / atlasH
		u1 := float32(g.region.X+g.region.W) / atlasW
		v1 := float32(g.region.Y+g.region.H) / atlasH

		// Integer pen X snap. The R/G/B channel masks already carry the
		// sub-pixel shift; sliding the quad on a fractional offset would
		// blur the stripes back into a soft grayscale edge.
		gx := float32(int32(pen)) + g.offX
		if pen < 0 && pen != float32(int32(pen)) {
			gx -= 1
		}
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
