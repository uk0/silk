package paint

import "silk/geom"

// FontExtents are the per-font metrics. Mirrors cairo_font_extents_t
// field-by-field; the Cairo build copies from cairo.FontExtents at
// each query, the pure-Go build fills it from opentype metrics.
type FontExtents struct {
	Ascent      float64
	Descent     float64
	Height      float64
	MaxXAdvance float64
	MaxYAdvance float64
}

// TextExtents are the per-string metrics. Mirrors cairo_text_extents_t.
type TextExtents struct {
	XBearing float64
	YBearing float64
	Width    float64
	Height   float64
	XAdvance float64
	YAdvance float64
}

// Glyph identifies one rasterised glyph in a face. The 'index' field
// corresponds to cairo_glyph_t.index; A is unused by Cairo but used
// by widget code as a per-glyph alpha hint. Layout matches Cairo's
// for unsafe-pointer copies in the legacy text-shaping path.
type Glyph struct {
	index uint32 // cairo glyph index, lowercase to keep Cairo-only access
	A     uint32
	X     float64
	Y     float64
}

// ScaledFont is a font materialised at a specific size + transform.
// Returned by Font.ScaledFont; widgets use it for measurement and
// glyph layout.
type ScaledFont interface {
	Font() Font
	FontExtents() *FontExtents
	TextExtents(text string) *TextExtents
	GlyphExtents(glyphs []Glyph) *TextExtents
	TextToGlyphs(x, y float64, text string) []Glyph
}

// Font is the public font handle. Concrete impls live in font_cairo.go
// (!silk_no_cairo) and font_pure.go (silk_no_cairo). NewFont is the
// build-tag-aware constructor.
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
