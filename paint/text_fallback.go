package paint

import (
	"math"
	"unicode"
	"unsafe"

	"github.com/uk0/silk/cairo"
)

func textHasMissingGlyph(glyphs []Glyph) bool {
	for _, glyph := range glyphs {
		if cairo.Glyph(glyph).Index() == 0 {
			return true
		}
	}
	return false
}

// Text fallback belongs to the text API, not TextToGlyphs/DrawGlyphs: glyph
// indices are local to a single face and cannot be mixed without retaining the
// owning font. Keep one run iterator for both measurement and drawing so carets,
// selections and hit testing use exactly the advances that reach the screen.
// This is simple LTR fallback, not a bidi/complex-script shaping engine.
func (sf *scaledFont) forEachTextRun(text string, visit func(*scaledFont, string)) {
	start, family := 0, ""
	for i, r := range text {
		next := family
		// Do not detach marks, variation selectors or joiners from their base.
		if !unicode.IsMark(r) && r != '\u200c' && r != '\u200d' {
			next = sf.fallbackFamily(r)
		}
		if next != family {
			if i > start {
				visit(sf.fontForFamily(family), text[start:i])
			}
			start, family = i, next
		}
	}
	if start < len(text) {
		visit(sf.fontForFamily(family), text[start:])
	}
}

func (sf *scaledFont) fontForFamily(family string) *scaledFont {
	if family == "" {
		return sf
	}
	f := *sf.font
	f.SetFamily(family)
	return f.scaledFont(&sf.ctm)
}

func (sf *scaledFont) fallbackTextExtents(text string) *TextExtents {
	out := &TextExtents{}
	hasInk := false
	sf.forEachTextRun(text, func(run *scaledFont, part string) {
		// Raw extents: do not recursively resolve fallback on fallback fonts.
		e := run.ScaledFont.TextExtents(part)
		if e.Width != 0 && e.Height != 0 {
			x, y := out.XAdvance+e.XBearing, out.YAdvance+e.YBearing
			if !hasInk {
				out.XBearing, out.YBearing, out.Width, out.Height = x, y, e.Width, e.Height
				hasInk = true
			} else {
				right := math.Max(out.XBearing+out.Width, x+e.Width)
				bottom := math.Max(out.YBearing+out.Height, y+e.Height)
				out.XBearing, out.YBearing = math.Min(out.XBearing, x), math.Min(out.YBearing, y)
				out.Width, out.Height = right-out.XBearing, bottom-out.YBearing
			}
		}
		out.XAdvance += e.XAdvance
		out.YAdvance += e.YAdvance
	})
	return out
}

func (p *cairoPainter) drawFallbackText(sf *scaledFont, text string) {
	p.applyBrush()
	x, y := 0.0, 0.0
	sf.forEachTextRun(text, func(run *scaledFont, part string) {
		p.cairo.SetScaledFont(run.ScaledFont)
		p.glyphBuf = run.textToGlyphsInto(x, y, part, p.glyphBuf[:0])
		if len(p.glyphBuf) != 0 {
			p.cairo.ShowGlyphs_hack(unsafe.Pointer(&p.glyphBuf[0]), len(p.glyphBuf))
		}
		e := run.ScaledFont.TextExtents(part)
		x, y = x+e.XAdvance, y+e.YAdvance
	})
	// SetScaledFont changes Cairo state, not the Go painter's logical font.
	// Restore it even if the final run used a fallback, including before a
	// following DrawGlyphs or a Save/Restore pair.
	p.cairo.SetScaledFont(sf.ScaledFont)
}
