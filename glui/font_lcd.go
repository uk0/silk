package glui

import (
	"image"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// snapshotAlpha takes a copy of the alpha samples in mask covering
// dr.Sub(dr.Min)-shifted coordinates starting at maskp. opentype.Face
// reuses one internal rasterisation buffer across successive Glyph
// calls — the mask returned by call N is invalidated by call N+1 — so
// the LCD path must snapshot each sub-position's mask immediately
// before invoking Glyph for the next sub-position. Without the
// snapshot all three "different" masks turn out byte-identical
// because they all alias the most recent rasterisation.
func snapshotAlpha(mask image.Image, maskp image.Point, dr image.Rectangle) *image.Alpha {
	w := dr.Dx()
	h := dr.Dy()
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	src := image.Rectangle{
		Min: maskp,
		Max: image.Point{X: maskp.X + w, Y: maskp.Y + h},
	}
	draw.Draw(out, out.Bounds(), mask, src.Min, draw.Src)
	return out
}

// LCD subpixel rasterisation packs three horizontally-shifted alpha masks
// into the R, G, B channels of an RGBA buffer. Each LCD pixel on a typical
// desktop display is composed of three vertical subpixels (red, green,
// blue, left-to-right), so a glyph rasterised at three sub-positions and
// distributed across those subpixels appears ~3× sharper than a single
// grayscale mask filtered by GL_LINEAR.
//
// The technique is the same one FreeType ships under FT_LCD_FILTER_DEFAULT.
// We deliberately do NOT apply FreeType's three-tap blur filter here —
// keeping the raw RGB stripes pins down the byte-level test surface (a
// single channel mismatch between R and B fails the test) while leaving
// room for a follow-up blur pass once visual baselines are tuned.
//
// rasteriseLCD walks the supplied face chain and uses the first face that
// produces a glyph for r at all three sub-positions. CJK fallback faces
// therefore work uniformly with LCD mode — the chain semantics match the
// grayscale path. Returns ok=false when no face renders the rune.
func rasteriseLCD(faces []font.Face, r rune) (rgba []byte, w, h int, offX, offY, advance float32, ok bool) {
	if len(faces) == 0 {
		return
	}

	// Sub-position dots in 26.6 fixed-point: 0, 1/3 px, 2/3 px. The opentype
	// rasteriser uses the fractional dot as the rasterisation offset, baking
	// the shift directly into the produced mask. Three rasterisations land at
	// effectively the same integer pixel grid (the dots stay strictly inside
	// one pixel) but with mass shifted right by 0, 21/64 px, 43/64 px.
	dots := [3]fixed.Int26_6{0, 64 / 3, (64 * 2) / 3}

	var (
		dr      image.Rectangle
		drA     image.Rectangle
		drB     image.Rectangle
		drC     image.Rectangle
		maskA   *image.Alpha
		maskB   *image.Alpha
		maskC   *image.Alpha
		adv     fixed.Int26_6
		matched bool
	)

	for _, fc := range faces {
		gdrA, gmA, gpA, gaA, hitA := fc.Glyph(fixed.Point26_6{X: dots[0]}, r)
		if !hitA {
			continue
		}
		// Snapshot before the next Glyph call to avoid aliasing — see
		// snapshotAlpha doc.
		snapA := snapshotAlpha(gmA, gpA, gdrA)

		gdrB, gmB, gpB, _, hitB := fc.Glyph(fixed.Point26_6{X: dots[1]}, r)
		if !hitB {
			continue
		}
		snapB := snapshotAlpha(gmB, gpB, gdrB)

		gdrC, gmC, gpC, _, hitC := fc.Glyph(fixed.Point26_6{X: dots[2]}, r)
		if !hitC {
			continue
		}
		snapC := snapshotAlpha(gmC, gpC, gdrC)

		drA, drB, drC = gdrA, gdrB, gdrC
		maskA, maskB, maskC = snapA, snapB, snapC
		adv = gaA
		matched = true
		break
	}
	if !matched {
		return
	}

	// Take the union of the three pixel rects so every channel contributes
	// over the same logical region. With sub-pixel shifts staying strictly
	// inside one pixel, the rects normally agree; we still take the union
	// defensively in case opentype rounds dr boundaries differently.
	dr = drA
	if drB.Min.X < dr.Min.X {
		dr.Min.X = drB.Min.X
	}
	if drB.Min.Y < dr.Min.Y {
		dr.Min.Y = drB.Min.Y
	}
	if drB.Max.X > dr.Max.X {
		dr.Max.X = drB.Max.X
	}
	if drB.Max.Y > dr.Max.Y {
		dr.Max.Y = drB.Max.Y
	}
	if drC.Min.X < dr.Min.X {
		dr.Min.X = drC.Min.X
	}
	if drC.Min.Y < dr.Min.Y {
		dr.Min.Y = drC.Min.Y
	}
	if drC.Max.X > dr.Max.X {
		dr.Max.X = drC.Max.X
	}
	if drC.Max.Y > dr.Max.Y {
		dr.Max.Y = drC.Max.Y
	}

	w = dr.Dx()
	h = dr.Dy()
	offX = float32(dr.Min.X)
	offY = float32(dr.Min.Y)
	advance = float32(adv) / 64
	if w == 0 || h == 0 {
		// Whitespace glyph — pen advances but no mask to pack.
		return nil, 0, 0, offX, offY, advance, true
	}

	// snapshotAlpha placed each per-channel mask at origin (0,0). The
	// snapshot's Pix index for column 0 of mask A corresponds to dr X
	// position drA.Min.X — so the union's column k is read from snapshot
	// column (k - shiftA) where shiftA = dr.Min.X - drA.Min.X. The same
	// holds for the Y axis. Out-of-bounds reads (column < 0 or ≥ snap
	// width) return zero coverage by image.Alpha's default behaviour.
	shiftA := image.Point{X: drA.Min.X - dr.Min.X, Y: drA.Min.Y - dr.Min.Y}
	shiftB := image.Point{X: drB.Min.X - dr.Min.X, Y: drB.Min.Y - dr.Min.Y}
	shiftC := image.Point{X: drC.Min.X - dr.Min.X, Y: drC.Min.Y - dr.Min.Y}

	rgba = make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ar, _, _, _ := maskA.At(x-shiftA.X, y-shiftA.Y).RGBA()
			br, _, _, _ := maskB.At(x-shiftB.X, y-shiftB.Y).RGBA()
			cr, _, _, _ := maskC.At(x-shiftC.X, y-shiftC.Y).RGBA()
			i := (y*w + x) * 4
			rgba[i+0] = byte(ar >> 8) // R: 0 px shift
			rgba[i+1] = byte(br >> 8) // G: 1/3 px shift
			rgba[i+2] = byte(cr >> 8) // B: 2/3 px shift
			// Alpha: average coverage. Used as the blend factor when GL
			// can't do dual-source per-channel blend (every desktop driver
			// can; GL 2.1 baseline cannot, hence the avg). drawTextLCD
			// installs a per-batch glBlendFunc(ONE, ONE_MINUS_SRC_COLOR)
			// so per-channel coverage drives the destination factor while
			// alpha controls overall opacity.
			rgba[i+3] = byte((ar + br + cr) / 3 >> 8)
		}
	}
	return rgba, w, h, offX, offY, advance, true
}

// SubpixelLCDEnabled reports whether the font rasterises glyphs through the
// LCD subpixel path. Tests, debug overlays, and the renderer use this to
// route batches to the kindGlyphLCD program — which expects an RGBA atlas
// and a per-channel blend func — instead of the standard alpha glyph path.
func (f *Font) SubpixelLCDEnabled() bool { return f.subpixelLCD }

// SetSubpixelLCD turns LCD subpixel rendering on or off and resets the glyph
// cache so the new buffer format takes effect on the next draw. Toggling at
// runtime is supported but expensive — the entire atlas is rebuilt the
// first time each rune is needed in the new mode.
//
// Setting LCD on automatically disables grayscale subpixel buckets: LCD
// stripes already encode subpixel position in three channels, so a bucket
// per fractional pen X would multiply atlas slot count without visible gain.
// The Subpixel flag itself is preserved on the Font so callers that re-disable
// LCD (SetSubpixelLCD(false)) get their previous bucket setting back.
func (f *Font) SetSubpixelLCD(on bool) {
	if f.subpixelLCD == on {
		return
	}
	f.subpixelLCD = on
	f.resetAtlas()
	f.dirty = true
}
