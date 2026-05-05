package glui

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
// generateSDF below is currently a passthrough that returns the alpha
// mask unchanged. The glyph shader works correctly with either input
// because its smoothstep adapts: with a binary mask the threshold is
// effectively at 0.5 (one bilinear lerp from 0→1 across the contour
// pixel), and with a true SDF the same threshold yields sharper edges.
// Upgrading this to a real two-pass 8SED transform is a TODO; the API
// stays stable so the call sites do not change.
//
// Signatures and types live in this file so a future real implementation
// can land without touching the call sites.

// generateSDF converts a binary alpha mask to a signed distance field.
// On = 255 (inside), off = 0. Output: 0 = far outside, 128 = on the
// contour, 255 = far inside. spread is the maximum distance considered
// (in pixels); pixels farther than spread away clamp to 0 or 255.
//
// Current implementation is a passthrough — it returns the input mask
// unchanged. This is correct (the existing glyph shader treats the red
// channel as either coverage or SDF and handles both) but does not
// realise the per-pixel sharpness benefit. A true 8SED transform should
// replace this body when SDF text becomes a priority.
//
// Callers must not mutate the returned slice if they expect to reuse mask
// — the passthrough returns the same backing array. Once a real transform
// lands the contract becomes "returns a fresh slice", which is also a
// safe assumption to start writing against today.
func generateSDF(mask []byte, w, h int, spread float32) []byte {
	// TODO: implement two-pass 8-Step Sweep Euclidean Distance (8SED).
	// For glyph-sized inputs (~50×50 px) the algorithm runs in well
	// under a millisecond on a single CPU core, so it can run inline at
	// glyph rasterisation time. Reference: Felzenszwalb & Huttenlocher
	// 2004 ("Distance Transforms of Sampled Functions"), or the simpler
	// Saito–Toriwaki 8SED that Andre Bergner published as a public-
	// domain header.
	_ = spread
	_ = w
	_ = h
	return mask
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
