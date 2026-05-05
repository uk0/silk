package glui

import "math"

// Signed-distance-field text generation.
//
// The default font path uses opentype to rasterise glyphs into 8-bit alpha
// masks. That works well at integer point sizes but loses crispness at
// extreme zoom — the designer canvas can scale text 4-8× and the linear
// upsampling smears edges.
//
// A signed distance field encodes, for each pixel, the distance to the
// nearest contour. Sampling an SDF with bilinear interpolation and a
// smoothstep gives sharp edges at any zoom factor: the GPU shader already
// does this (see glyphFragSrc — `texture2D(...).r` is treated as coverage,
// which doubles as a half-coverage SDF when the input is normalised so
// 0.5 marks the contour).
//
// generateSDF performs the real two-pass 8-point sequential Euclidean
// distance transform (8SED, Danielsson 1980). For glyph-sized inputs
// (~50×50 px) it runs in well under a millisecond on a single CPU core,
// so it can run inline at glyph rasterisation time.

// generateSDF converts a binary alpha mask to a signed distance field
// using the 8-point sequential Euclidean distance transform (Danielsson
// 1980). It maintains two distance fields — one for inside pixels, one
// for outside — sweeps each forward and backward, then combines them
// into a single signed output.
//
// Input: mask of size w*h, single byte per pixel. >=128 means "inside".
// Output: same dimensions; 0 = far outside, 128 = on contour, 255 = far
// inside. spread (in pixels) sets the maximum distance considered before
// saturating to 0 or 255.
//
// For each output pixel the algorithm reads the OPPOSITE-state field:
// an inside pixel reports its distance to the nearest outside pixel
// (negative d → high byte), and an outside pixel reports its distance
// to the nearest inside pixel (positive d → low byte). This is the
// standard signed-distance convention.
func generateSDF(mask []byte, w, h int, spread float32) []byte {
	if w <= 0 || h <= 0 || spread <= 0 || len(mask) < w*h {
		return mask
	}

	// Two distance fields: one for the inside, one for the outside.
	// Each cell stores (dx, dy) — vector to nearest seed pixel of the
	// SAME state. After sweeping, an inside pixel's outside-field entry
	// holds the vector to the nearest outside pixel, and vice versa.
	type vec struct{ dx, dy int16 }
	const FAR = int16(32000)

	inside := make([]vec, w*h)
	outside := make([]vec, w*h)

	for i := 0; i < w*h; i++ {
		if mask[i] >= 128 {
			inside[i] = vec{0, 0}
			outside[i] = vec{FAR, FAR}
		} else {
			inside[i] = vec{FAR, FAR}
			outside[i] = vec{0, 0}
		}
	}

	// Forward and backward sweep helpers. The 8SED kernel uses 4
	// neighbours per direction: forward sweep reaches up-left, up,
	// up-right, and left; backward sweep reaches down-right, down,
	// down-left, and right.
	sweep := func(field []vec, fwd bool) {
		cmp := func(idx, dx, dy int) {
			x := idx % w
			y := idx / w
			nx := x + dx
			ny := y + dy
			if nx < 0 || nx >= w || ny < 0 || ny >= h {
				return
			}
			n := field[ny*w+nx]
			if n.dx == FAR {
				return
			}
			cd := vec{n.dx + int16(dx), n.dy + int16(dy)}
			cur := field[idx]
			if int32(cd.dx)*int32(cd.dx)+int32(cd.dy)*int32(cd.dy) <
				int32(cur.dx)*int32(cur.dx)+int32(cur.dy)*int32(cur.dy) {
				field[idx] = cd
			}
		}
		if fwd {
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					i := y*w + x
					cmp(i, -1, 0)
					cmp(i, 0, -1)
					cmp(i, -1, -1)
					cmp(i, 1, -1)
				}
			}
		} else {
			for y := h - 1; y >= 0; y-- {
				for x := w - 1; x >= 0; x-- {
					i := y*w + x
					cmp(i, 1, 0)
					cmp(i, 0, 1)
					cmp(i, -1, 1)
					cmp(i, 1, 1)
				}
			}
		}
	}

	sweep(outside, true)
	sweep(outside, false)
	sweep(inside, true)
	sweep(inside, false)

	out := make([]byte, w*h)
	for i := 0; i < w*h; i++ {
		var d float32
		if mask[i] >= 128 {
			// Inside pixel — distance to nearest OUTSIDE pixel.
			// Stored in outside[i] after the outside sweep, which
			// propagates from outside seeds.
			o := outside[i]
			if o.dx == FAR {
				d = -spread
			} else {
				d = -float32(math.Sqrt(float64(int32(o.dx)*int32(o.dx) + int32(o.dy)*int32(o.dy))))
			}
		} else {
			// Outside pixel — distance to nearest INSIDE pixel.
			in := inside[i]
			if in.dx == FAR {
				d = spread
			} else {
				d = float32(math.Sqrt(float64(int32(in.dx)*int32(in.dx) + int32(in.dy)*int32(in.dy))))
			}
		}
		// Map [-spread, +spread] → [255, 0]. Inside pixels (negative d)
		// land high; outside pixels (positive d) land low; the contour
		// (d ≈ 0) maps to 128.
		norm := -d/spread*0.5 + 0.5
		if norm < 0 {
			norm = 0
		} else if norm > 1 {
			norm = 1
		}
		out[i] = byte(norm * 255)
	}
	return out
}

// FontSDF is reserved for a future Font variant that stores SDF glyphs
// instead of raster masks. Today the type is an empty stub so call sites
// in higher layers can compile against the future API. When the real
// generator above lands, this becomes a thin wrapper around Font that
// post-processes each rasterised glyph through generateSDF before atlas
// upload.
type FontSDF struct {
	// Future: face, atlas, sdf-specific spread/clamp parameters.
	// Intentionally empty — adding fields here without a working impl
	// would mislead callers.
}
