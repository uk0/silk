package glui

import (
	"image"
	"image/draw"
	"os"

	"silk/glui/atlas"

	"github.com/go-gl/gl/v2.1/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// sdfPadding is the per-side padding (in pixels) added to each glyph slot
// when SDF mode is active. The distance transform needs room around the
// glyph contour so its falloff is not clipped by the atlas neighbours; an
// 8-pixel border (same as sdfSpread) keeps soft falloff fully on-tile.
const sdfPadding = 4

// numSubpixelBuckets is the count of fractional X positions cached per
// glyph when subpixel rendering is enabled. Four buckets give a quarter-
// pixel quantization (0.0, 0.25, 0.5, 0.75 px) — small enough that the
// residual error after bucketing is below human perceptual threshold for
// 14pt text on retina displays. The cost is up to 4× more atlas slots
// per used glyph; ASCII still fits comfortably in the default 2048×2048
// atlas, and CJK working sets stay under a few thousand glyphs after
// bucket multiplication.
const numSubpixelBuckets = 4

// sdfSpread sets the maximum distance (in pixels) the SDF generator
// considers before saturating to 0/255. Matches sdfPadding so the spread
// just reaches the edge of the padded slot.
const sdfSpread = 8.0

// DefaultFontDPI is the resolution opentype rasterises at. 72 DPI keeps
// "size" in CSS-style points so a font created with size 14 produces text
// approximately 14 logical units tall on screen.
const DefaultFontDPI = 72

// defaultAtlasSize is the side length of a fresh glyph atlas. 2048×2048 is
// chosen so a typical CJK working set (~3000 commonly used Han characters)
// fits at 14pt SDF without overflow. At single-channel storage that's 4 MB
// per atlas — affordable for desktop GPUs and dwarfed by a single full-
// screen RGBA framebuffer. ASCII-only workloads pay the same memory cost
// but never grow beyond it; the atlas is allocated once per Font instance
// and reused across frames.
const defaultAtlasSize = 2048

// Font owns a single font face at a single point size, plus the atlas that
// caches its rasterised glyph masks. A Font is bound to one GL Context
// (because of the texture) but otherwise is independent of any per-frame
// Renderer.
//
// Glyphs are vector-rasterised by the opentype package into 8-bit alpha
// masks (one channel) and uploaded as gl.LUMINANCE — anti-aliased at the
// rendered size. To render text at a different size, allocate a different
// Font; sharing one Font across sizes would defeat the per-size atlas.
type Font struct {
	face font.Face // primary face — owns the layout metrics

	// faces is the lookup chain searched for each glyph. faces[0] is always
	// the primary (matches face). Subsequent entries are CJK or other
	// fallbacks discovered at construction time. Glyph() walks the chain
	// and uses the first face whose .Glyph returns ok=true.
	//
	// Per advisor guidance: layout metrics (Ascent/Descent/LineHeight) come
	// from the primary face only — pre-cached in ascent/descent/height — so
	// mixed Latin + CJK lines stay on a single baseline. Per-glyph offX,
	// offY and advance come from the rasterising face, otherwise CJK glyphs
	// would read against the wrong bearings and float above the baseline.
	faces  []font.Face
	atlas  *atlas.Atlas
	glyphs map[glyphKey]glyphInfo

	// Atlas pixels (single channel) live on the CPU until upload. The
	// dirty flag means we have to re-upload at next Texture() call.
	pixels []byte
	atlasW int
	atlasH int
	dirty  bool

	// GPU texture id; lazily created on the first upload.
	texture uint32

	// Cached face metrics so we don't query the face for every line.
	ascent  float32
	descent float32
	height  float32

	// useSDF flips glyph rasterisation through generateSDF instead of
	// uploading raw alpha masks. Opt-in via SILK_GLUI_SDF=1; the default
	// is the raster path so existing tests and visual baselines stay put.
	// Toggling this on a constructed Font is not supported — the atlas
	// dimensions and per-glyph padding are baked in at construction.
	useSDF bool

	// subpixel turns on the multi-bucket subpixel-positioning cache. When
	// enabled, each glyph is rasterised at numSubpixelBuckets fractional X
	// offsets (0.0, 0.25, 0.5, 0.75 px). DrawText picks the matching bucket
	// for the current pen fraction and snaps the quad to integer X — the
	// subpixel shift is baked into the mask, not produced by texture
	// filtering, so glyphs stay sharp at any pen position. Opt-in via
	// SILK_GLUI_SUBPIXEL=1; default off so existing tests and pixel-perfect
	// baselines are unaffected.
	subpixel bool
}

// glyphKey identifies a cached glyph variant. r is the rune; sub is the
// subpixel-offset bucket [0, numSubpixelBuckets) when subpixel mode is on.
// In default (non-subpixel) mode every key has sub == 0 and the cache
// behaves identically to the previous map[rune]glyphInfo.
type glyphKey struct {
	r   rune
	sub uint8
}

// glyphInfo is the per-rune cache record. (X, Y) inside the atlas are the
// pixel coordinates of the glyph mask. (offX, offY) are the offsets from
// the pen position to the top-left of the rendered quad — i.e. the glyph's
// bearings in face-space.
type glyphInfo struct {
	region  atlas.Region
	offX    float32
	offY    float32
	advance float32
}

// subpixelBucket quantises a fractional pen-X offset into [0, numSubpixelBuckets).
// Negative inputs wrap into [0, 1) before bucketing so callers don't have
// to normalise themselves.
func subpixelBucket(fracX float32) uint8 {
	// Reduce to [0, 1).
	f := fracX - float32(int32(fracX))
	if f < 0 {
		f += 1
	}
	b := int(f * float32(numSubpixelBuckets))
	if b >= numSubpixelBuckets {
		b = numSubpixelBuckets - 1
	}
	if b < 0 {
		b = 0
	}
	return uint8(b)
}

// NewFont creates an opentype-rasterised font at the given point size,
// using the bundled "Go Regular" typeface as primary and any locally-
// available CJK font (PingFang on macOS, Noto Sans CJK on Linux, Microsoft
// YaHei on Windows) as fallback. The atlas is sized defaultAtlasSize ×
// defaultAtlasSize which comfortably holds the ASCII-plus-Latin-Extended
// set together with several thousand Han glyphs at 14pt.
//
// When the SILK_GLUI_SDF env var is set to "1" the font runs each glyph
// through generateSDF before atlas upload, producing a signed distance
// field instead of a raster mask. Sampling the SDF with smoothstep gives
// crisp edges at extreme zoom — useful for the designer canvas. SDF mode
// allocates the atlas at 1024x1024 still, but each glyph now occupies a
// `sdfPadding`-padded slot so the distance field has room to spread.
//
// On error (which would only happen if the embedded TTF data became
// invalid — i.e. never, in normal operation) NewFont falls back to a
// zero-glyph Font so the caller's draw loop won't panic on nil deref.
func NewFont(size float64) *Font {
	useSDF := os.Getenv("SILK_GLUI_SDF") == "1"
	useSubpixel := os.Getenv("SILK_GLUI_SUBPIXEL") == "1"
	face, err := newGoRegularFace(size)
	if err != nil {
		// Defensive: allocate a Font with no face. Glyph() will return
		// zero-advance records so layout still terminates.
		return &Font{
			faces:    nil, // Glyph() walks faces; len==0 returns zero record
			atlas:    atlas.New(defaultAtlasSize, defaultAtlasSize),
			glyphs:   make(map[glyphKey]glyphInfo),
			pixels:   make([]byte, defaultAtlasSize*defaultAtlasSize),
			atlasW:   defaultAtlasSize,
			atlasH:   defaultAtlasSize,
			dirty:    true,
			useSDF:   useSDF,
			subpixel: useSubpixel,
		}
	}
	f := newFontFromFace(face, defaultAtlasSize, defaultAtlasSize)
	f.useSDF = useSDF
	f.subpixel = useSubpixel
	// Append CJK fallbacks. discoverSystemCJKFaces is best-effort: missing
	// system fonts return an empty slice and the font still renders Latin
	// text correctly via the primary face. newFontFromFace already seeded
	// f.faces with the primary so we only append the discovered fallbacks.
	for _, fb := range discoverSystemCJKFaces(size) {
		f.faces = append(f.faces, fb)
	}
	return f
}

// newGoRegularFace parses the bundled Go Regular TTF and creates a face at
// the requested point size.
func newGoRegularFace(size float64) (font.Face, error) {
	ttf, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     DefaultFontDPI,
		Hinting: font.HintingFull,
	})
}

func newFontFromFace(face font.Face, w, h int) *Font {
	metrics := face.Metrics()
	f := &Font{
		face:    face,
		faces:   []font.Face{face},
		atlas:   atlas.New(w, h),
		glyphs:  make(map[glyphKey]glyphInfo),
		pixels:  make([]byte, w*h),
		atlasW:  w,
		atlasH:  h,
		dirty:   true,
		ascent:  fixedToF32(metrics.Ascent),
		descent: fixedToF32(metrics.Descent),
		height:  fixedToF32(metrics.Height),
	}
	return f
}

// fixedToF32 converts a 26.6 fixed-point value to a float32 in points,
// preserving sub-pixel precision that the opentype rasteriser exposes.
func fixedToF32(v fixed.Int26_6) float32 {
	return float32(v) / 64
}

// Ascent returns the font's ascent in points.
func (f *Font) Ascent() float32 { return f.ascent }

// Descent returns the font's descent in points.
func (f *Font) Descent() float32 { return f.descent }

// LineHeight returns the recommended distance between baselines.
func (f *Font) LineHeight() float32 {
	if f.height > 0 {
		return f.height
	}
	return f.ascent + f.descent
}

// Glyph returns the cached glyph info for r at the integer-pen variant
// (subpixel bucket 0). All callers that don't care about subpixel
// positioning use this entry point; it preserves the legacy behaviour
// of one rasterisation per rune.
func (f *Font) Glyph(r rune) glyphInfo {
	return f.glyphForKey(glyphKey{r: r, sub: 0})
}

// GlyphAt returns the cached glyph info appropriate for drawing r at a
// pen position whose fractional X component is fracX. When subpixel
// rendering is disabled it forwards to Glyph(r); otherwise it picks the
// right subpixel-bucket variant, rasterising it on first request.
func (f *Font) GlyphAt(r rune, fracX float32) glyphInfo {
	if !f.subpixel {
		return f.glyphForKey(glyphKey{r: r, sub: 0})
	}
	return f.glyphForKey(glyphKey{r: r, sub: subpixelBucket(fracX)})
}

// glyphForKey is the unified rasterise+cache entry point. The lookup
// walks f.faces in order: the primary (Latin) face is tried first;
// CJK fallback faces are only consulted when primary returns ok=false.
// opentype.Face.Glyph reports ok=false for runes that resolve to glyph
// index 0 (.notdef), so the chain skips faces that would otherwise emit
// a tofu box.
//
// The subpixel offset is encoded in the rasterisation dot — a 26.6 fixed
// point. With numSubpixelBuckets=4 the dots are 0, 16, 32, 48 (= 0.0,
// 0.25, 0.5, 0.75 px). The opentype rasteriser uses the fractional dot
// for hinting, producing a pixel mask with that subpixel shift baked in.
// Each cache entry therefore carries a sharp glyph at one specific
// quarter-pixel offset; DrawText picks the right one for the live pen.
func (f *Font) glyphForKey(key glyphKey) glyphInfo {
	if g, ok := f.glyphs[key]; ok {
		return g
	}
	if len(f.faces) == 0 {
		g := glyphInfo{}
		f.glyphs[key] = g
		return g
	}

	// Encode the subpixel bucket as a fractional 26.6 dot. Bucket 0 means
	// dot=(0,0) which is identical to the legacy rasterisation, so a
	// non-subpixel font (which always passes sub=0) hits the same path.
	dotX := fixed.Int26_6(int(key.sub) * 64 / numSubpixelBuckets)

	var (
		dr      image.Rectangle
		mask    image.Image
		maskp   image.Point
		advance fixed.Int26_6
		matched bool
	)
	for _, fc := range f.faces {
		gdr, gmask, gmaskp, gadv, ok := fc.Glyph(fixed.Point26_6{X: dotX}, key.r)
		if !ok {
			continue
		}
		dr = gdr
		mask = gmask
		maskp = gmaskp
		advance = gadv
		matched = true
		break
	}
	if !matched {
		// All faces miss — cache a zero-record so we don't re-walk every
		// frame. Pen does not advance for the missing rune.
		f.glyphs[key] = glyphInfo{}
		return f.glyphs[key]
	}

	w := dr.Dx()
	h := dr.Dy()
	if w == 0 || h == 0 {
		// Whitespace glyph: no mask, just advance.
		g := glyphInfo{advance: fixedToF32(advance)}
		f.glyphs[key] = g
		return g
	}

	// In SDF mode each glyph slot is padded by sdfPadding pixels on every
	// side so the distance transform can spread without clipping into a
	// neighbour. The packed region therefore measures (w+pad*2+1) by
	// (h+pad*2+1) — same +1 bleed guard as the raster path.
	pad := 0
	if f.useSDF {
		pad = sdfPadding
	}
	slotW := w + pad*2
	slotH := h + pad*2

	region, fit := f.atlas.Pack(slotW+1, slotH+1) // 1-pixel padding to prevent bleed
	if !fit {
		// Atlas is full — drop the glyph silently, returning an info that
		// still advances the pen. A more sophisticated implementation
		// would grow the atlas or evict by LRU.
		g := glyphInfo{advance: fixedToF32(advance)}
		f.glyphs[key] = g
		return g
	}

	// Copy the glyph mask into our CPU-side atlas pixel buffer. The
	// opentype rasteriser returns mask data already in *image.Alpha form
	// (or its rendering subset), so draw.Draw with draw.Src reads exactly
	// the alpha byte we want. In SDF mode we centre the glyph inside the
	// padded slot, leaving an empty border that becomes the SDF spread.
	dst := newAlphaView(f.pixels, f.atlasW, f.atlasH)
	maskX := region.X + pad
	maskY := region.Y + pad
	dstRect := image.Rect(maskX, maskY, maskX+w, maskY+h)
	draw.Draw(dst, dstRect, mask, maskp, draw.Src)

	if f.useSDF {
		// Run the distance transform over the padded slot in-place.
		// Extract the slot to a contiguous buffer (SDF expects packed rows
		// without an atlas stride), transform, then write back.
		buf := make([]byte, slotW*slotH)
		for yy := 0; yy < slotH; yy++ {
			srcOff := (region.Y+yy)*f.atlasW + region.X
			copy(buf[yy*slotW:(yy+1)*slotW], f.pixels[srcOff:srcOff+slotW])
		}
		sdf := generateSDF(buf, slotW, slotH, sdfSpread)
		for yy := 0; yy < slotH; yy++ {
			dstOff := (region.Y+yy)*f.atlasW + region.X
			copy(f.pixels[dstOff:dstOff+slotW], sdf[yy*slotW:(yy+1)*slotW])
		}
	}

	g := glyphInfo{
		region:  atlas.Region{X: region.X, Y: region.Y, W: slotW, H: slotH},
		offX:    float32(dr.Min.X - pad),
		offY:    float32(dr.Min.Y - pad),
		advance: fixedToF32(advance),
	}
	f.glyphs[key] = g
	f.dirty = true
	return g
}

// SubpixelEnabled reports whether the font caches multiple subpixel
// variants per glyph. Tests and tools that introspect rendering quality
// can branch on this.
func (f *Font) SubpixelEnabled() bool { return f.subpixel }

// SetSubpixel toggles subpixel rendering after construction. Useful for
// runtime A/B comparisons; widget code typically passes the env var
// SILK_GLUI_SUBPIXEL at startup instead.
func (f *Font) SetSubpixel(on bool) { f.subpixel = on }

// MeasureText returns the total advance width of text in this font.
func (f *Font) MeasureText(text string) float32 {
	var w float32
	for _, ch := range text {
		w += f.Glyph(ch).advance
	}
	return w
}

// Texture returns the GL texture id, uploading dirty atlas pixels first.
func (f *Font) Texture() uint32 {
	if f.dirty || f.texture == 0 {
		f.upload()
	}
	return f.texture
}

func (f *Font) upload() {
	if f.texture == 0 {
		gl.GenTextures(1, &f.texture)
		gl.BindTexture(gl.TEXTURE_2D, f.texture)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, f.texture)
	}

	// UNPACK_ALIGNMENT is global GL state — if some other code flips it
	// to the default 4, a subsequent non-4-aligned atlas upload would
	// skew the glyph rows. Set it every upload as defence-in-depth.
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)

	gl.TexImage2D(
		gl.TEXTURE_2D, 0, gl.LUMINANCE,
		int32(f.atlasW), int32(f.atlasH),
		0, gl.LUMINANCE, gl.UNSIGNED_BYTE,
		gl.Ptr(f.pixels),
	)
	f.dirty = false
}

// Destroy releases the GL texture. Calling this without a current GL
// context is safe — it simply leaks the id.
func (f *Font) Destroy() {
	if f.texture != 0 {
		gl.DeleteTextures(1, &f.texture)
		f.texture = 0
	}
}

// AtlasSize returns the dimensions of the glyph atlas in pixels.
func (f *Font) AtlasSize() (w, h int) { return f.atlasW, f.atlasH }

// AtlasPixels returns the CPU-side atlas buffer. Tests and adapters may
// inspect it; mutate at your own risk (call MarkDirty afterwards).
func (f *Font) AtlasPixels() []byte { return f.pixels }

// MarkDirty forces a re-upload at the next Texture() call.
func (f *Font) MarkDirty() { f.dirty = true }

// newAlphaView wraps a byte slice as an image.Alpha so we can use draw.Draw
// for the atlas blit. The slice is treated as a tightly-packed buffer of
// (w * h) bytes — the same memory layout image.Alpha uses with Stride = w.
func newAlphaView(pix []byte, w, h int) *image.Alpha {
	return &image.Alpha{
		Pix:    pix,
		Stride: w,
		Rect:   image.Rect(0, 0, w, h),
	}
}

// FontCache lazily creates Font instances for each requested size.
// All fonts share the same underlying TTF data, so memory overhead per
// size is just the rasterized atlas + glyph table.
//
// FontCache is NOT safe for concurrent use; render threads should hold
// one instance and create per-size fonts on demand.
type FontCache struct {
	fonts map[int]*Font // key = size in points (rounded to nearest int)
}

// NewFontCache returns a fresh, empty cache.
func NewFontCache() *FontCache {
	return &FontCache{fonts: make(map[int]*Font)}
}

// At returns the Font for the requested point size, creating it on first
// request. Sizes are rounded to the nearest integer point — sub-point
// scaling is handled by the GPU at draw time, not by allocating a fresh
// atlas per fractional size.
func (c *FontCache) At(size float64) *Font {
	key := int(size + 0.5)
	if f, ok := c.fonts[key]; ok {
		return f
	}
	f := NewFont(float64(key))
	c.fonts[key] = f
	return f
}

// defaultFontCache is the package-level cache backing DefaultFont.
var defaultFontCache = NewFontCache()

// DefaultFont returns the cached default font at the given point size.
// Repeated calls with the same size return the same Font instance.
func DefaultFont(size float64) *Font {
	return defaultFontCache.At(size)
}
