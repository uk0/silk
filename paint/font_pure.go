//go:build silk_no_cairo

package paint

import (
	"strconv"

	"silk/geom"
)

// pureFont is the silk_no_cairo font handle. Stores the same
// metadata as cairoFont (family/size/style) but doesn't materialise
// a Cairo face — measurement falls back to a fixed-pitch approximation.
//
// The real font measurement in pure-Go mode happens through silk/glui's
// opentype-based Font (with CJK fallback chain). Widget code calling
// Font.TextExtents on a pureFont sees an estimate; the actual rendered
// width comes from glui at draw time. This is "good enough" for most
// layout — labels/buttons get sensible bounds — but not pixel-exact
// for kerning-sensitive layouts. A future round can wire pureFont's
// measurement directly to glui's font cache.
type pureFont struct {
	family string
	size   int
	italic bool
	bold   bool
	desc   string
}

func (f *pureFont) Family() string         { return f.family }
func (f *pureFont) SetFamily(family string) { f.family = family; f.desc = "" }
func (f *pureFont) Size() int               { return f.size }
func (f *pureFont) SetSize(sz int)          { f.size = sz; f.desc = "" }
func (f *pureFont) Italic() bool            { return f.italic }
func (f *pureFont) SetItalic(b bool)        { f.italic = b; f.desc = "" }
func (f *pureFont) Bold() bool              { return f.bold }
func (f *pureFont) SetBold(b bool)          { f.bold = b; f.desc = "" }

func (f *pureFont) String() string {
	if f.desc == "" {
		s := f.family + ":" + strconv.Itoa(f.size)
		if f.bold {
			s += "(b)"
		}
		if f.italic {
			s += "(i)"
		}
		f.desc = s
	}
	return f.desc
}

func (f *pureFont) Dup() Font {
	g := *f
	return &g
}

func (f *pureFont) Equal(other Font) bool {
	o, ok := other.(*pureFont)
	if !ok {
		return false
	}
	return f.family == o.family && f.size == o.size && f.italic == o.italic && f.bold == o.bold
}

// estimateAdvance is a coarse character-width estimate for pure-Go
// mode. ASCII at 0.5×size, CJK at 1.0×size — close enough for layout
// to allocate reasonable bounds. The real glyph advances come from
// glui's opentype path at draw time.
func (f *pureFont) estimateAdvance(text string) float64 {
	w := 0.0
	sz := float64(f.size)
	for _, r := range text {
		switch {
		case r < 0x80:
			w += sz * 0.5
		case r >= 0x4e00 && r <= 0x9fff: // CJK Unified Ideographs
			w += sz
		default:
			w += sz * 0.6
		}
	}
	return w
}

func (f *pureFont) ScaledFont(ctm *geom.Mat3x2) ScaledFont {
	return &pureScaledFont{font: f}
}

func (f *pureFont) FontExtents() *FontExtents {
	sz := float64(f.size)
	return &FontExtents{
		Ascent:      sz * 0.8,
		Descent:     sz * 0.2,
		Height:      sz * 1.2,
		MaxXAdvance: sz,
		MaxYAdvance: sz,
	}
}

func (f *pureFont) TextExtents(text string) *TextExtents {
	sz := float64(f.size)
	w := f.estimateAdvance(text)
	return &TextExtents{
		XBearing: 0,
		YBearing: -sz * 0.8,
		Width:    w,
		Height:   sz,
		XAdvance: w,
		YAdvance: 0,
	}
}

func (f *pureFont) GlyphExtents(glyphs []Glyph) *TextExtents {
	sz := float64(f.size)
	w := float64(len(glyphs)) * sz * 0.6
	return &TextExtents{
		XBearing: 0,
		YBearing: -sz * 0.8,
		Width:    w,
		Height:   sz,
		XAdvance: w,
		YAdvance: 0,
	}
}

func (f *pureFont) TextToGlyphs(x, y float64, text string) []Glyph {
	out := make([]Glyph, 0, len(text))
	pen := x
	sz := float64(f.size)
	for _, r := range text {
		out = append(out, Glyph{
			index: uint32(r),
			X:     pen,
			Y:     y,
		})
		switch {
		case r < 0x80:
			pen += sz * 0.5
		case r >= 0x4e00 && r <= 0x9fff:
			pen += sz
		default:
			pen += sz * 0.6
		}
	}
	return out
}

// pureScaledFont mirrors scaledFont but with no Cairo backing. Just
// proxies measurement queries to the parent pureFont.
type pureScaledFont struct {
	font *pureFont
}

func (s *pureScaledFont) Font() Font                                 { return s.font }
func (s *pureScaledFont) FontExtents() *FontExtents                  { return s.font.FontExtents() }
func (s *pureScaledFont) TextExtents(text string) *TextExtents       { return s.font.TextExtents(text) }
func (s *pureScaledFont) GlyphExtents(glyphs []Glyph) *TextExtents   { return s.font.GlyphExtents(glyphs) }
func (s *pureScaledFont) TextToGlyphs(x, y float64, text string) []Glyph {
	return s.font.TextToGlyphs(x, y, text)
}

// NewFont constructs a pure-Go Font handle. Same signature and
// clamping as the Cairo build.
func NewFont(family string, size int, bold, italic bool) Font {
	if size < 6 {
		size = 6
	}
	if size > 72 {
		size = 72
	}
	return &pureFont{family: family, size: size, bold: bold, italic: italic}
}
