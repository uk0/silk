package paint

import (
	"encoding/binary"
	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/hashmap"
	"math"
	"runtime"
	"strconv"
	"sync/atomic"
	"unsafe"
)

var fontFaceCache = make(map[string]*fontFace)
var fontFaceCacheAccessId uint64
var scaledFontCache = hashmap.NewHashMap(hashScaledFontCacheKey, equalScaledFontCacheKey)
var scaledFontCacheAccessId uint64
var globalFontOptions = newUIFontOptions()

func newUIFontOptions() *cairo.FontOptions {
	options := cairo.NewFontOptions()
	if runtime.GOOS == "windows" {
		// We render into ARGB offscreen surfaces, not directly to a known RGB
		// LCD. ClearType's colored coverage becomes fringes when composited,
		// scaled or viewed remotely. Explicit grayscale AA is orientation-neutral
		// and also avoids inheriting disabled font smoothing from an RDP session.
		options.SetAntialias(cairo.ANTIALIAS_GRAY)
		options.SetHintStyle(cairo.HINT_STYLE_FULL)
		options.SetHintMetrics(cairo.HINT_METRICS_ON)
	}
	return options
}

// Atomic because the decrements happen in finalizers, which the runtime runs
// on its own goroutine concurrently with whatever thread is allocating.
var fontFaceCount atomic.Int64
var scaledFontCount atomic.Int64
var identityMatrix = geom.Mat3x2{1, 0, 0, 1, 0, 0}

type FontExtents cairo.FontExtents
type TextExtents cairo.TextExtents
type Glyph cairo.Glyph

func murmur3_32(b []byte) uint32 {
	const c1 uint32 = 0xcc9e2d51
	const c2 uint32 = 0x1b873593
	const r1 uint32 = 15
	const r2 uint32 = 13
	const m uint32 = 5
	const n uint32 = 0xe6546b64

	var hash uint32 = 0x5a7cbfed // seed

	_len := len(b)
	len0 := uint32(_len)
	i := 0
	for _len >= 4 {
		k := uint32(b[i]) | uint32(b[i+1])<<8 | uint32(b[i+2])<<16 | uint32(b[i+3])<<24
		k *= c1
		k = (k << r1) | (k >> (32 - r1))
		k *= c2

		hash ^= k
		hash = (hash << r2) | (hash >> (32 - r2))
		hash = hash*m + n
		i += 4
		_len -= 4
	}

	if _len != 0 {
		var k uint32 = 0

		switch _len {
		case 3:
			k ^= uint32(b[i+2]) << 16
		case 2:
			k ^= uint32(b[i+1]) << 8
		case 1:
			k ^= uint32(b[i])
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
	// Value snapshots are essential: callers reuse/mutate their CTM, and Cairo
	// may return the same toy face for several font sizes.
	m          geom.Mat3x2
	face       *cairo.FontFace
	fontMatrix geom.Mat3x2
}

func equalScaledFontCacheKey(a, b interface{}) bool {
	if a == b {
		return true
	}
	ma := a.(*scaledFontCacheKey)
	mb := b.(*scaledFontCacheKey)
	return *ma == *mb
}

func hashScaledFontCacheKey(i interface{}) uint32 {
	k := i.(*scaledFontCacheKey)
	// Hash the same values equality compares, not the address of a caller's
	// matrix or struct padding. The face address is identity only: never turn
	// the integer back into a pointer (which would violate checkptr provenance).
	var data [13 * 8]byte
	binary.LittleEndian.PutUint64(data[:8], uint64(uintptr(unsafe.Pointer(k.face))))
	values := [...]float64{k.m.Xx, k.m.Yx, k.m.Xy, k.m.Yy, k.m.X0, k.m.Y0,
		k.fontMatrix.Xx, k.fontMatrix.Yx, k.fontMatrix.Xy, k.fontMatrix.Yy, k.fontMatrix.X0, k.fontMatrix.Y0}
	for n, value := range values {
		// Go considers +0 and -0 equal; their hashes must also be equal.
		if value == 0 {
			value = 0
		}
		binary.LittleEndian.PutUint64(data[(n+1)*8:], math.Float64bits(value))
	}
	return murmur3_32(data[:])
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
	ctm    geom.Mat3x2
	// Family names, not font pointers: avoid finalizer-retaining cycles between
	// cached fonts. Missing/primary results use an empty family name.
	fallbackFamilies map[rune]string
}

func (this *scaledFont) setFinalizer() {
	scaledFontCount.Add(1)
	runtime.SetFinalizer(this, func(p *scaledFont) {
		p.ScaledFont.Destroy()
		scaledFontCount.Add(-1)
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
	text = displayText(text)
	this.ScaledFont.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			ret = make([]Glyph, num)
			//C.memcpy(unsafe.Pointer(&ret[0]), buf, C.size_t(num*24))
			for i := 0; i < num; i++ {
				ret[i] = *(*Glyph)(unsafe.Pointer(uintptr(buf) + uintptr(i)*24))
				if runtime.GOOS == "windows" {
					// cairo_glyph_t's unsigned long is 32-bit on Windows;
					// A is alignment padding, which native malloc leaves undefined.
					ret[i].A = 0
				}
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
	return this.validTextToGlyphsInto(x, y, displayText(text), dst)
}

// validTextToGlyphsInto consumes text already checked at the public paint boundary.
func (this *scaledFont) validTextToGlyphsInto(x, y float64, text string, dst []Glyph) []Glyph {
	dst = dst[:0]
	this.ScaledFont.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			if cap(dst) < num {
				dst = make([]Glyph, num)
			} else {
				dst = dst[:num]
			}
			for i := 0; i < num; i++ {
				dst[i] = *(*Glyph)(unsafe.Pointer(uintptr(buf) + uintptr(i)*24))
				if runtime.GOOS == "windows" {
					dst[i].A = 0
				}
			}
		})
	return dst
}

func (this *scaledFont) TextExtents(text string) *TextExtents {
	text = displayText(text)
	if shaped := this.complexLayout(text); shaped != nil {
		e := shaped.extents
		return &e
	}
	if needsTextFallback(text) {
		return this.fallbackTextExtents(text)
	}
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
	family     string
	size       int
	italic     bool
	bold       bool
	face       *cairo.FontFace // 为cairo做特别优化
	nativeSize float64         // exact size of a retained Core Text fallback face
	desc       string
}

type fontFace struct {
	face   *cairo.FontFace
	access uint64
}

func (this *fontFace) setFinalizer() {
	fontFaceCount.Add(1)
	runtime.SetFinalizer(this, func(p *fontFace) {
		p.face.Destroy()
		fontFaceCount.Add(-1)
	})
}

func (this *font) Family() string {
	return this.family
}

func (this *font) SetFamily(family string) {
	this.family = family
	this.face = nil
	this.desc = ""
	this.nativeSize = 0
}

func (this *font) Size() int {
	return this.size
}

func (this *font) SetSize(sz int) {
	this.size = sz
	this.face = nil
	this.desc = ""
	this.nativeSize = 0
}

func (this *font) Italic() bool {
	return this.italic
}

func (this *font) SetItalic(b bool) {
	this.italic = b
	this.face = nil
	this.desc = ""
	this.nativeSize = 0
}

func (this *font) Bold() bool {
	return this.bold
}

func (this *font) SetBold(b bool) {
	this.bold = b
	this.face = nil
	this.desc = ""
	this.nativeSize = 0
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
	key := &scaledFontCacheKey{m: identityMatrix, face: face}
	if ctm != nil {
		key.m = *ctm
	}
	// Cairo ignores CTM translation when creating a scaled font. Retaining
	// widget positions in this cache key kept an entry for every drag/scroll
	// coordinate even though font scale, hinting and glyph metrics were equal.
	key.m.X0, key.m.Y0 = 0, 0
	size := float64(this.size)
	if this.nativeSize > 0 {
		size = this.nativeSize
	}
	key.fontMatrix.InitScale(size, size)
	isf, ok := scaledFontCache.Find(key)
	if ok {
		sf = isf.(*scaledFont)
		sf.access = scaledFontCacheAccessId
	} else {
		sf0 := cairo.NewScaledFont(face,
			&key.fontMatrix, &key.m, globalFontOptions)
		// The cache entry represents this font at creation time, even if the
		// original Font is subsequently resized or restyled in place.
		snapshot := *this
		sf = &scaledFont{ScaledFont: sf0, font: &snapshot, access: scaledFontCacheAccessId, ctm: key.m}
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
