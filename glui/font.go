package glui

import (
	"image"
	"image/draw"

	"silk/glui/atlas"

	"github.com/go-gl/gl/v2.1/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Font owns a single font face plus the atlas that caches its rasterised
// glyphs. A Font is bound to one GL Context (because of the texture) but
// otherwise is independent of any per-frame Renderer.
//
// The current implementation uses image/font/basicfont — a fixed-size
// raster face — and uploads its alpha masks via gl.LUMINANCE. It is good
// enough for UI labels at 1× and 2× scale; a future revision will swap in
// FreeType-rasterised SDFs without changing the Font interface.
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

// NewFont creates a font using basicfont.Face7x13. The atlas is 512x512;
// at 7x13 that comfortably fits the entire ASCII range plus several
// hundred extra glyphs.
func NewFont() *Font {
	return newFontFromFace(basicfont.Face7x13, 512, 512)
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
		ascent:  float32(metrics.Ascent.Round()),
		descent: float32(metrics.Descent.Round()),
		height:  float32(metrics.Height.Round()),
	}
	return f
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
// request. If the atlas is full an empty record is returned.
func (f *Font) Glyph(r rune) glyphInfo {
	if g, ok := f.glyphs[r]; ok {
		return g
	}

	// Ask the face for the glyph's mask and metrics.
	dr, mask, maskp, advance, ok := f.face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		// No glyph (or fallback) — cache as a zero-width record so we
		// don't keep asking. Advance is whatever the face suggests.
		g := glyphInfo{advance: float32(advance.Round())}
		f.glyphs[r] = g
		return g
	}

	w := dr.Dx()
	h := dr.Dy()
	if w == 0 || h == 0 {
		// Whitespace glyph: no mask, just advance.
		g := glyphInfo{advance: float32(advance.Round())}
		f.glyphs[r] = g
		return g
	}

	region, fit := f.atlas.Pack(w+1, h+1) // 1-pixel padding to prevent bleed
	if !fit {
		// Atlas is full — drop the glyph silently, returning an info that
		// still advances the pen. A more sophisticated implementation
		// would grow the atlas or evict by LRU.
		g := glyphInfo{advance: float32(advance.Round())}
		f.glyphs[r] = g
		return g
	}

	// Copy the glyph mask into our CPU-side atlas pixel buffer.
	dst := newAlphaView(f.pixels, f.atlasW, f.atlasH)
	dstRect := image.Rect(region.X, region.Y, region.X+w, region.Y+h)
	draw.Draw(dst, dstRect, mask, maskp, draw.Src)

	g := glyphInfo{
		region:  atlas.Region{X: region.X, Y: region.Y, W: w, H: h},
		offX:    float32(dr.Min.X),
		offY:    float32(dr.Min.Y),
		advance: float32(advance.Round()),
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
