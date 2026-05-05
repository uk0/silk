package glui

import (
	"image"
	"image/draw"

	"silk/glui/atlas"

	"github.com/go-gl/gl/v2.1/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// DefaultFontDPI is the resolution opentype rasterises at. 72 DPI keeps
// "size" in CSS-style points so a font created with size 14 produces text
// approximately 14 logical units tall on screen.
const DefaultFontDPI = 72

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
	face   font.Face
	atlas  *atlas.Atlas
	glyphs map[rune]glyphInfo

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

// NewFont creates an opentype-rasterised font at the given point size,
// using the bundled "Go Regular" typeface. The atlas is sized 1024x1024
// which comfortably holds an ASCII-plus-Latin-Extended subset at 14pt and
// scales down for 9–10pt UI labels.
//
// On error (which would only happen if the embedded TTF data became
// invalid — i.e. never, in normal operation) NewFont falls back to a
// zero-glyph Font so the caller's draw loop won't panic on nil deref.
func NewFont(size float64) *Font {
	face, err := newGoRegularFace(size)
	if err != nil {
		// Defensive: allocate a Font with no face. Glyph() will return
		// zero-advance records so layout still terminates.
		return &Font{
			atlas:  atlas.New(1024, 1024),
			glyphs: make(map[rune]glyphInfo),
			pixels: make([]byte, 1024*1024),
			atlasW: 1024,
			atlasH: 1024,
			dirty:  true,
		}
	}
	return newFontFromFace(face, 1024, 1024)
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
		atlas:   atlas.New(w, h),
		glyphs:  make(map[rune]glyphInfo),
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

// Glyph returns the cached glyph info for r, rasterising it on first
// request. If the atlas is full or no face is loaded, an empty record
// (with whatever advance the face reports) is returned.
func (f *Font) Glyph(r rune) glyphInfo {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	if f.face == nil {
		g := glyphInfo{}
		f.glyphs[r] = g
		return g
	}

	// Ask the face for the glyph's mask and metrics.
	dr, mask, maskp, advance, ok := f.face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		// No glyph (or fallback) — cache as a zero-width record so we
		// don't keep asking. Advance is whatever the face suggests.
		g := glyphInfo{advance: fixedToF32(advance)}
		f.glyphs[r] = g
		return g
	}

	w := dr.Dx()
	h := dr.Dy()
	if w == 0 || h == 0 {
		// Whitespace glyph: no mask, just advance.
		g := glyphInfo{advance: fixedToF32(advance)}
		f.glyphs[r] = g
		return g
	}

	region, fit := f.atlas.Pack(w+1, h+1) // 1-pixel padding to prevent bleed
	if !fit {
		// Atlas is full — drop the glyph silently, returning an info that
		// still advances the pen. A more sophisticated implementation
		// would grow the atlas or evict by LRU.
		g := glyphInfo{advance: fixedToF32(advance)}
		f.glyphs[r] = g
		return g
	}

	// Copy the glyph mask into our CPU-side atlas pixel buffer. The
	// opentype rasteriser returns mask data already in *image.Alpha form
	// (or its rendering subset), so draw.Draw with draw.Src reads exactly
	// the alpha byte we want.
	dst := newAlphaView(f.pixels, f.atlasW, f.atlasH)
	dstRect := image.Rect(region.X, region.Y, region.X+w, region.Y+h)
	draw.Draw(dst, dstRect, mask, maskp, draw.Src)

	g := glyphInfo{
		region:  atlas.Region{X: region.X, Y: region.Y, W: w, H: h},
		offX:    float32(dr.Min.X),
		offY:    float32(dr.Min.Y),
		advance: fixedToF32(advance),
	}
	f.glyphs[r] = g
	f.dirty = true
	return g
}

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
