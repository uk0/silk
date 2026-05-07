//go:build !silk_no_cairo

package paint

import (
	"runtime"
	"strconv"
	"unsafe"

	"silk/cairo"
	"silk/core"
	"silk/geom"
	"silk/hashmap"
)

// Cairo-backed Font implementation. The interfaces (Font, ScaledFont)
// and value types (FontExtents, TextExtents, Glyph) live in font.go
// without a build tag; this file fills in the implementation that
// uses Cairo's toy font face + scaled font infrastructure.

var fontFaceCache = make(map[string]*fontFace)
var fontFaceCacheAccessId uint64
var scaledFontCache = hashmap.NewHashMap(hashScaledFontCacheKey, equalScaledFontCacheKey)
var scaledFontCacheAccessId uint64
var globalFontOptions = cairo.NewFontOptions()
var fontFaceCount = 0
var scaledFontCount = 0
var identityMatrix = geom.Mat3x2{Xx: 1, Yx: 0, Xy: 0, Yy: 1, X0: 0, Y0: 0}

// dummy reference so core import isn't pruned when only used in
// rare debug paths.
var _ = core.Warn

// (Deleted murmur3_32 — replaced by mixPtr/mixFloat in
// hashScaledFontCacheKey. The old impl iterated +4 byte chunks past
// the struct end which tripped checkptr under -race.)

type scaledFontCacheKey struct {
	m    *geom.Mat3x2
	face *cairo.FontFace
}

func equalScaledFontCacheKey(a, b interface{}) bool {
	if a == b {
		return true
	}
	ma := a.(*scaledFontCacheKey)
	mb := b.(*scaledFontCacheKey)
	return ma.face == mb.face && (ma.m == mb.m ||
		ma.m != nil && mb.m != nil &&
			ma.m.Xx == mb.m.Xx && ma.m.Yx == mb.m.Yx &&
			ma.m.Xy == mb.m.Xy && ma.m.Yy == mb.m.Yy &&
			ma.m.X0 == mb.m.X0 && ma.m.Y0 == mb.m.Y0)
}

// hashScaledFontCacheKey hashes the cache key by explicit field
// access rather than raw byte iteration over the struct. The earlier
// implementation passed `unsafe.Pointer(m)` + `unsafe.Sizeof(*m)`
// into a generic murmur3 which iterated +4 byte chunks past the end
// of the struct under -race / checkptr — flagged as a checkptr
// violation. The fix avoids unsafe pointer arithmetic entirely.
func hashScaledFontCacheKey(i interface{}) uint32 {
	m := i.(*scaledFontCacheKey)
	// Hash the matrix pointer + face pointer addresses. Pointers are
	// stable while the face / matrix lives in the cache; identity-
	// based hashing is what equalScaledFontCacheKey already mirrors
	// on the matrix-pointer fast path.
	h := uint32(0x5a7cbfed)
	h = mixPtr(h, uintptr(unsafe.Pointer(m.m)))
	h = mixPtr(h, uintptr(unsafe.Pointer(m.face)))
	if m.m != nil {
		// Mat3x2 pointers might collide across instances when the
		// caller re-uses one — stir in the matrix values too.
		h = mixFloat(h, m.m.Xx)
		h = mixFloat(h, m.m.Yx)
		h = mixFloat(h, m.m.Xy)
		h = mixFloat(h, m.m.Yy)
		h = mixFloat(h, m.m.X0)
		h = mixFloat(h, m.m.Y0)
	}
	return h
}

// mixPtr / mixFloat are murmur-3-flavoured 32-bit mixers that
// stir a single 64-bit value into the running hash without unsafe
// pointer arithmetic. Sufficient distribution for the small
// scaled-font cache; don't try to use these for general hashing.
func mixPtr(h uint32, v uintptr) uint32 {
	a := uint32(v)
	b := uint32(v >> 32)
	h ^= a
	h = (h*0x85ebca6b ^ (h >> 16))
	h ^= b
	h = (h*0xc2b2ae35 ^ (h >> 13))
	return h
}

func mixFloat(h uint32, f float64) uint32 {
	bits := *(*uint64)(unsafe.Pointer(&f))
	h ^= uint32(bits)
	h = (h*0x85ebca6b ^ (h >> 16))
	h ^= uint32(bits >> 32)
	h = (h*0xc2b2ae35 ^ (h >> 13))
	return h
}

type scaledFont struct {
	*cairo.ScaledFont
	font   *font
	access uint64
}

func (this *scaledFont) setFinalizer() {
	scaledFontCount++
	runtime.SetFinalizer(this, func(p *scaledFont) {
		p.ScaledFont.Destroy()
		scaledFontCount--
	})
}

func (this *scaledFont) Font() Font { return this.font }

func (this *scaledFont) GlyphExtents(glyphs []Glyph) *TextExtents {
	n := len(glyphs)
	var p unsafe.Pointer
	if n == 0 {
		p = nil
	} else {
		p = unsafe.Pointer(&glyphs[0])
	}
	ce := this.ScaledFont.GlyphExtents_hack(p, n)
	return cairoTextExtentsToPaint(ce)
}

func (this *scaledFont) TextToGlyphs(x, y float64, text string) (ret []Glyph) {
	this.ScaledFont.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			ret = make([]Glyph, num)
			for i := 0; i < num; i++ {
				ret[i] = *(*Glyph)(unsafe.Pointer(uintptr(buf) + uintptr(i)*24))
			}
		})
	return
}

func (this *scaledFont) TextExtents(text string) *TextExtents {
	return cairoTextExtentsToPaint(this.ScaledFont.TextExtents(text))
}

func (this *scaledFont) FontExtents() *FontExtents {
	return cairoFontExtentsToPaint(this.ScaledFont.FontExtents())
}

// cairoTextExtentsToPaint copies a cairo.TextExtents value into a
// fresh paint.TextExtents. The two structs have identical field
// layouts but distinct Go types; field-by-field copy keeps both
// type systems happy without unsafe pointer casts.
func cairoTextExtentsToPaint(c *cairo.TextExtents) *TextExtents {
	if c == nil {
		return &TextExtents{}
	}
	return &TextExtents{
		XBearing: c.XBearing,
		YBearing: c.YBearing,
		Width:    c.Width,
		Height:   c.Height,
		XAdvance: c.XAdvance,
		YAdvance: c.YAdvance,
	}
}

func cairoFontExtentsToPaint(c *cairo.FontExtents) *FontExtents {
	if c == nil {
		return &FontExtents{}
	}
	return &FontExtents{
		Ascent:      c.Ascent,
		Descent:     c.Descent,
		Height:      c.Height,
		MaxXAdvance: c.MaxXAdvance,
		MaxYAdvance: c.MaxYAdvance,
	}
}

type font struct {
	family string
	size   int
	italic bool
	bold   bool
	face   *cairo.FontFace
	desc   string
}

type fontFace struct {
	face   *cairo.FontFace
	access uint64
}

func (this *fontFace) setFinalizer() {
	fontFaceCount++
	runtime.SetFinalizer(this, func(p *fontFace) {
		p.face.Destroy()
		fontFaceCount--
	})
}

func (this *font) Family() string { return this.family }

func (this *font) SetFamily(family string) {
	this.family = family
	this.face = nil
	this.desc = ""
}

func (this *font) Size() int { return this.size }

func (this *font) SetSize(sz int) {
	this.size = sz
	this.face = nil
	this.desc = ""
}

func (this *font) Italic() bool { return this.italic }

func (this *font) SetItalic(b bool) {
	this.italic = b
	this.face = nil
	this.desc = ""
}

func (this *font) Bold() bool { return this.bold }

func (this *font) SetBold(b bool) {
	this.bold = b
	this.face = nil
	this.desc = ""
}

func (this *font) String() string {
	if this.desc == "" {
		s := this.Family() + ":" + strconv.Itoa(this.size)
		if this.bold {
			s = s + "(b)"
		}
		if this.italic {
			s = s + "(i)"
		}
		this.desc = s
	}
	return this.desc
}

func (this *font) Face() *cairo.FontFace {
	if this.face == nil {
		fontFaceCacheAccessId++
		key := this.String()
		slot, ok := fontFaceCache[key]
		if ok {
			slot.access = fontFaceCacheAccessId
			this.face = slot.face
		} else {
			var slant cairo.FontSlant
			if this.italic {
				slant = cairo.FONT_SLANT_ITALIC
			} else {
				slant = cairo.FONT_SLANT_NORMAL
			}
			var weight cairo.FontWeight
			if this.bold {
				weight = cairo.FONT_WEIGHT_BOLD
			} else {
				weight = cairo.FONT_WEIGHT_NORMAL
			}
			this.face = cairo.NewToyFontFace(this.family, slant, weight)
			slot = &fontFace{this.face, fontFaceCacheAccessId}
			slot.setFinalizer()
			fontFaceCache[key] = slot
		}
	}
	return this.face
}

func (this *font) Dup() Font {
	p := new(font)
	*p = *this
	return p
}

func (this *font) Equal(f Font) bool {
	return this.String() == f.String()
}

func (this *font) ScaledFont(ctm *geom.Mat3x2) ScaledFont {
	return this.scaledFont(ctm)
}

func (this *font) scaledFont(ctm *geom.Mat3x2) *scaledFont {
	var sf *scaledFont
	scaledFontCacheAccessId++
	face := this.Face()
	key := &scaledFontCacheKey{ctm, face}
	isf, ok := scaledFontCache.Find(key)
	if ok {
		sf = isf.(*scaledFont)
		sf.access = scaledFontCacheAccessId
	} else {
		if ctm == nil {
			ctm = &identityMatrix
		}
		var m geom.Mat3x2
		m.InitScale(float64(this.size), float64(this.size))
		sf0 := cairo.NewScaledFont(face,
			&m, ctm, globalFontOptions)
		sf = &scaledFont{sf0, this, scaledFontCacheAccessId}
		sf.setFinalizer()
		scaledFontCache.Insert(key, sf)
	}
	return sf
}

func (this *font) FontExtents() *FontExtents {
	return this.scaledFont(nil).FontExtents()
}

func (this *font) TextExtents(text string) *TextExtents {
	return this.scaledFont(nil).TextExtents(text)
}

func (this *font) GlyphExtents(glyphs []Glyph) *TextExtents {
	return this.scaledFont(nil).GlyphExtents(glyphs)
}

func (this *font) TextToGlyphs(x, y float64, text string) []Glyph {
	return this.scaledFont(nil).TextToGlyphs(x, y, text)
}

// NewFont constructs a Cairo-backed font. The pure-Go build provides
// its own NewFont in font_pure.go using opentype.
func NewFont(family string, size int, bold, italic bool) Font {
	if size < 6 {
		size = 6
	}
	if size > 72 {
		size = 72
	}
	return &font{
		family: family,
		size:   size,
		bold:   bold,
		italic: italic,
	}
}
