package paint

import (
	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/hashmap"
	"runtime"
	"strconv"
	"unsafe"
)

var fontFaceCache = make(map[string]*fontFace)
var fontFaceCacheAccessId uint64
var scaledFontCache = hashmap.NewHashMap(hashScaledFontCacheKey, equalScaledFontCacheKey)
var scaledFontCacheAccessId uint64
var globalFontOptions = cairo.NewFontOptions()
var fontFaceCount = 0
var scaledFontCount = 0
var identityMatrix = geom.Mat3x2{1, 0, 0, 1, 0, 0}

type FontExtents cairo.FontExtents
type TextExtents cairo.TextExtents
type Glyph cairo.Glyph

func murmur3_32(_p uintptr, _len int) uint32 {
	const c1 uint32 = 0xcc9e2d51
	const c2 uint32 = 0x1b873593
	const r1 uint32 = 15
	const r2 uint32 = 13
	const m uint32 = 5
	const n uint32 = 0xe6546b64

	p := unsafe.Pointer(_p)

	var hash uint32 = 0x5a7cbfed // seed

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

type ScaledFont interface {
	Font() Font
	FontExtents() *FontExtents
	TextExtents(text string) *TextExtents
	GlyphExtents(glyphs []Glyph) *TextExtents
	TextToGlyphs(x, y float64, text string) []Glyph
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

func (this *scaledFont) Font() Font {
	return this.font
}

func (this *scaledFont) GlyphExtents(glyphs []Glyph) *TextExtents {
	n := len(glyphs)
	var p unsafe.Pointer
	if n == 0 {
		p = nil
	} else {
		p = unsafe.Pointer(&glyphs[0])
	}
	return (*TextExtents)(this.ScaledFont.GlyphExtents_hack(p, n))
}

func (this *scaledFont) TextToGlyphs(x, y float64, text string) (ret []Glyph) {
	this.ScaledFont.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			ret = make([]Glyph, num)
			//C.memcpy(unsafe.Pointer(&ret[0]), buf, C.size_t(num*24))
			for i := 0; i < num; i++ {
				ret[i] = *(*Glyph)(unsafe.Pointer(uintptr(buf) + uintptr(i)*24))
			}
		})
	return
}

// textToGlyphsInto shapes text into dst, reusing dst's backing array when it
// has the capacity and only allocating a fresh slice when the buffer must grow.
// It returns the filled slice (valid until the buffer is next reused). Unlike
// the exported TextToGlyphs — which always allocates a fresh slice the caller
// may retain — this is the internal hot-path variant for DrawText/DrawText1,
// where the glyphs are consumed synchronously and never kept.
func (this *scaledFont) textToGlyphsInto(x, y float64, text string, dst []Glyph) []Glyph {
	this.ScaledFont.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			if cap(dst) < num {
				dst = make([]Glyph, num)
			} else {
				dst = dst[:num]
			}
			for i := 0; i < num; i++ {
				dst[i] = *(*Glyph)(unsafe.Pointer(uintptr(buf) + uintptr(i)*24))
			}
		})
	return dst
}

func (this *scaledFont) TextExtents(text string) *TextExtents {
	return (*TextExtents)(this.ScaledFont.TextExtents(text))
}

func (this *scaledFont) FontExtents() *FontExtents {
	return (*FontExtents)(this.ScaledFont.FontExtents())
}

// 字体
type Font interface {
	Family() string
	SetFamily(family string)
	Size() int
	SetSize(sz int)
	Italic() bool
	SetItalic(b bool)
	Bold() bool
	SetBold(b bool)
	Dup() Font
	Equal(f Font) bool
	String() string
	ScaledFont(ctm *geom.Mat3x2) ScaledFont
	FontExtents() *FontExtents
	TextExtents(text string) *TextExtents
	GlyphExtents(glyphs []Glyph) *TextExtents
	TextToGlyphs(x, y float64, text string) []Glyph
}

type font struct {
	family string
	size   int
	italic bool
	bold   bool
	face   *cairo.FontFace // 为cairo做特别优化
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

func (this *font) Family() string {
	return this.family
}

func (this *font) SetFamily(family string) {
	this.family = family
	this.face = nil
	this.desc = ""
}

func (this *font) Size() int {
	return this.size
}

func (this *font) SetSize(sz int) {
	this.size = sz
	this.face = nil
	this.desc = ""
}

func (this *font) Italic() bool {
	return this.italic
}

func (this *font) SetItalic(b bool) {
	this.italic = b
	this.face = nil
	this.desc = ""
}

func (this *font) Bold() bool {
	return this.bold
}

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
		italic: italic}
}
