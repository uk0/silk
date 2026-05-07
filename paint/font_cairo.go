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

func murmur3_32(_p uintptr, _len int) uint32 {
	const c1 uint32 = 0xcc9e2d51
	const c2 uint32 = 0x1b873593
	const r1 uint32 = 15
	const r2 uint32 = 13
	const m uint32 = 5
	const n uint32 = 0xe6546b64

	p := unsafe.Pointer(_p)

	var hash uint32 = 0x5a7cbfed

	len0 := uint32(_len)
	for _len >= 4 {
		k := *((*uint32)(p))
		k *= c1
		k = (k << r1) | (k >> (32 - r1))
		k *= c2

		hash ^= k
		hash = (hash << r2) | (hash >> (32 - r2))
		hash = hash*m + n
		p = unsafe.Pointer(uintptr(p) + 4)
		_len -= 4
	}

	if _len != 0 {
		var k uint32 = 0
		switch _len {
		case 3:
			p2 := (*uint8)(unsafe.Pointer(uintptr(p) + 2))
			k ^= uint32(*p2) << 16
		case 2:
			p1 := (*uint8)(unsafe.Pointer(uintptr(p) + 1))
			k ^= uint32(*p1) << 8
		case 1:
			p0 := (*uint8)(p)
			k ^= uint32(*p0)
			k *= c1
			k = (k << r1) | (k >> (32 - r1))
			k *= c2
			hash ^= k
		}
	}

	hash ^= len0
	hash ^= (hash >> 16)
	hash *= 0x85ebca6b
	hash ^= (hash >> 13)
	hash *= 0xc2b2ae35
	hash ^= (hash >> 16)
	return hash
}

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

func hashScaledFontCacheKey(i interface{}) uint32 {
	m := i.(*scaledFontCacheKey)
	return murmur3_32(uintptr(unsafe.Pointer(m)), int(unsafe.Sizeof(*m)))
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
