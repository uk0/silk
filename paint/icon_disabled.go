package paint

import (
	"image"
	"runtime"

	"github.com/uk0/silk/cairo"
)

func (s *subIcon) sourcePattern() *cairo.Pattern {
	if s.pat == nil {
		s.pat = cairo.NewPatternForSurface(s.img.Surface)
		runtime.SetFinalizer(s, func(sub *subIcon) { sub.pat.Destroy() })
	}
	return s.pat
}

func (s *subIcon) disabledPresentation() *subIcon {
	if !s.disabledMaskReady {
		s.disabledMaskReady = true
		if mask := tonalDisabledIconMask(s.img); mask != nil {
			s.disabledMask = &subIcon{size: s.size, img: mask}
		}
	}
	if s.disabledMask != nil {
		return s.disabledMask
	}
	return s
}

// tonalDisabledIconMask keeps the contrast of a filled raster glyph in its
// alpha channel. Tint is still applied at draw time; a theme change does not
// decode, resample or pre-composite the sprite against a fixed background.
// A monochrome icon already has the right silhouette and needs no new mask.
func tonalDisabledIconMask(src *cairoSurface) *cairoSurface {
	const maxDimension = 4096
	w, h := src.Width(), src.Height()
	if w <= 0 || h <= 0 || w > maxDimension || h > maxDimension {
		return nil
	}
	im, err := src.Image()
	rgba, ok := im.(*image.RGBA)
	if err != nil || !ok {
		return nil
	}

	// Derive contrast from the most-covered pixels. Ignore antialiased
	// edges and shadows when finding tonal limits. The deepest ink keeps
	// its original coverage, including fully opaque pixels.
	peak, lo, hi := byte(0), 255, 0
	for y := range h {
		row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+4*w]
		for i := 0; i < len(row); i += 4 {
			if a := row[i+3]; a == 0 || a < peak {
				continue
			} else if a > peak {
				peak, lo, hi = a, 255, 0
			}
			y := straightIconLuma(row[i], row[i+1], row[i+2], row[i+3])
			lo, hi = min(lo, y), max(hi, y)
		}
	}
	span := hi - lo
	if span < 24 {
		return nil
	}

	mask := &cairoSurface{Surface: cairo.NewImageSurface(cairo.FORMAT_A8, w, h)}
	mask.setFinalizer()
	stride := mask.Stride()
	if stride < w {
		CloseSurface(mask)
		return nil
	}
	coverage := make([]byte, stride*h)
	for y := range h {
		row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+4*w]
		for x := range w {
			i := 4 * x
			a := row[i+3]
			if a == 0 {
				continue
			}
			depth := min(span, max(0, straightIconLuma(row[i], row[i+1], row[i+2], a)-lo))
			// Keep about 55% of original coverage in bright details.
			gain := 255 - (116*depth+span/2)/span
			coverage[y*stride+x] = byte((int(a)*gain + 127) / 255)
		}
	}
	if err := mask.Surface.SetData(coverage); err != nil {
		CloseSurface(mask)
		return nil
	}
	return mask
}

// Cairo's RGBA readback is premultiplied. Evaluate sRGB luma after undoing
// premultiplication so low coverage is not mistaken for dark colored ink.
func straightIconLuma(r, g, b, a byte) int {
	weighted := 54*int(r) + 183*int(g) + 19*int(b)
	return min(255, (weighted*255+128*int(a))/(256*int(a)))
}
