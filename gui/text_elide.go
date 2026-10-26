package gui

import (
	"github.com/uk0/silk/paint"
	"math"
)

// elideMark is the tail a cut string ends with — the same single-rune ellipsis
// the rune-count truncators elsewhere in the tree append.
const elideMark = "…"

// elideToWidth shortens s until it paints within max pixels in font f, ending
// the cut string with elideMark. A string that already fits comes back
// untouched; max too small for even the ellipsis yields "".
//
// Unlike the rune-count truncators (codeeditor's truncateBlame, the git log
// panel's logTruncate) this measures, because the strips it feeds — the status
// bar's message area, a toast's text column — are bounded in pixels, and a rune
// budget that fits "控件控件" overflows with "MMMM". Every candidate is measured
// whole rather than summing per-rune advances: kerning makes the width of a
// string more than the sum of its parts.
func elideToWidth(f paint.Font, s string, max float64) string {
	// Subtracting padding from a measured cell can lose a few ULPs. This
	// sub-pixel tolerance avoids replacing a fully fitting CJK caption.
	max += 1e-7
	if f.TextExtents(s).Width <= max {
		return s
	}
	if f.TextExtents(elideMark).Width > max {
		return ""
	}
	// range over a string yields the byte offset of each rune's first byte, so
	// s[:i] is always a whole-rune prefix.
	fit := elideMark // the empty prefix, already known to fit
	for i := range s {
		if i == 0 {
			continue
		}
		cand := s[:i] + elideMark
		if f.TextExtents(cand).Width > max {
			return fit
		}
		fit = cand
	}
	return fit
}

// ElideText fits text to a logical-pixel width in font, using a Unicode ellipsis
// when needed. The input is never changed; callers may expose it in a tooltip.
// Nil fonts and nonpositive/NaN widths produce no text. Paint operations should
// still clip their allocated rectangle to contain glyph overhangs.
func ElideText(font paint.Font, text string, width float64) string {
	if font == nil || width <= 0 || math.IsNaN(width) {
		return ""
	}
	return elideToWidth(font, text, width)
}
