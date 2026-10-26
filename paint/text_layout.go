package paint

import "unicode/utf8"

// GlyphRun retains the font that owns its glyph indices. Begin and End are
// rune offsets in the source text; glyph coordinates share the layout origin.
// Treat the font and glyph slice as read-only while retaining a layout.
type GlyphRun struct {
	Font       Font
	Glyphs     []Glyph
	Begin, End int
}

// TextLayout is a retained, simple left-to-right layout. Carets contains one
// fractional X coordinate per rune boundary (including the final boundary).
// macOS native composed sequences have cluster boundaries; this is not a bidi engine. Drawing the retained
// runs does not repeat coverage lookup or text shaping on every frame.
type TextLayout struct {
	Runs     []GlyphRun
	Carets   []float64
	Clusters []TextCluster
}

// LayoutText uses the same native font fallback as DrawText and TextExtents,
// without mixing indices from different fonts. Font/ScaledFont implementations
// need no new methods. Each run owns a duplicate of its font, so later changes
// to the caller's font cannot silently invalidate the retained glyphs.
func LayoutText(sf ScaledFont, text string) TextLayout {
	text = displayText(text)
	if native, ok := sf.(*scaledFont); ok {
		if shaped := native.complexLayout(text); shaped != nil {
			return cloneTextLayout(shaped.layout)
		}
	}
	return layoutSimpleText(sf, text)
}

func layoutSimpleText(sf ScaledFont, text string) TextLayout {
	out := TextLayout{Carets: make([]float64, 1, utf8.RuneCountInString(text)+1)}
	if text == "" {
		return out
	}
	glyphs := sf.TextToGlyphs(0, 0, text)
	if native, ok := sf.(*scaledFont); ok && needsTextFallback(text) && textHasMissingGlyph(glyphs) {
		native.forEachTextRun(text, func(run *scaledFont, part string) {
			out.appendRun(run, part, run.TextToGlyphs(0, 0, part))
		})
	} else {
		out.appendRun(sf, text, glyphs)
	}
	return out
}

func rawTextAdvance(sf ScaledFont, text string) float64 {
	if native, ok := sf.(*scaledFont); ok {
		return native.ScaledFont.TextExtents(text).XAdvance
	}
	return sf.TextExtents(text).XAdvance
}

func (out *TextLayout) appendRun(sf ScaledFont, text string, glyphs []Glyph) {
	count := utf8.RuneCountInString(text)
	owner := sf.Font().Dup()
	if len(glyphs) != count {
		// The public glyph API provides no cluster map. Preserve every rune and
		// every returned glyph rather than truncating the whole TextBlock. Only
		// unusual/custom backends take this bounded per-rune compatibility path.
		for _, r := range text {
			part := string(r)
			out.appendGlyphs(owner, sf.TextToGlyphs(0, 0, part), 1, rawTextAdvance(sf, part))
		}
		return
	}
	out.appendGlyphs(owner, glyphs, count, rawTextAdvance(sf, text))
}

func (out *TextLayout) appendGlyphs(owner Font, glyphs []Glyph, count int, advance float64) {
	begin := len(out.Carets) - 1
	origin := out.Carets[begin]
	if len(glyphs) == count {
		for i := 1; i < count; i++ {
			out.Carets = append(out.Carets, origin+glyphs[i].X)
		}
	}
	out.Carets = append(out.Carets, origin+advance)
	for i := range glyphs {
		glyphs[i].X += origin
	}
	out.Runs = append(out.Runs, GlyphRun{Font: owner, Glyphs: glyphs, Begin: begin, End: begin + count})
}
