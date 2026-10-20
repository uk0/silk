//go:build windows && !silk_pure_go

package paint

import (
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/uk0/silk/cairo"
)

const maxFallbackRunes = 2048

// Native Cairo's toy font backend does not perform font linking. In
// particular Consolas lacks CJK/emoji. Resolve only genuinely missing glyphs;
// ASCII keeps the old allocation-free glyph-buffer path and original metrics.
func needsTextFallback(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

func (sf *scaledFont) hasNativeGlyph(r rune) bool {
	present := false
	err := sf.ScaledFont.TextToGlyphs_hack(0, 0, string(r), func(buf unsafe.Pointer, n int) {
		present = n > 0
		for _, g := range unsafe.Slice((*cairo.Glyph)(buf), n) {
			if g.Index() == 0 {
				present = false
			}
		}
	})
	return err == nil && present
}

func (sf *scaledFont) fallbackFamily(r rune) string {
	// Private-use characters identify glyphs in a specific icon font. An
	// unrelated fallback face may assign the same code point a different icon.
	if r < utf8.RuneSelf || unicode.Is(unicode.Co, r) {
		return ""
	}
	if family, ok := sf.fallbackFamilies[r]; ok {
		return family
	}
	family := ""
	if !sf.hasNativeGlyph(r) {
		// Installed coverage is checked by the actual Cairo backend, not
		// inferred from a family name. Missing optional language fonts simply
		// fall through, with the primary .notdef retained as the last resort.
		families := []string{"Segoe UI Emoji", "Segoe UI Symbol", "Microsoft YaHei UI", "Yu Gothic UI", "Malgun Gothic", "SimSun-ExtB"}
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			families = []string{"Microsoft YaHei UI", "Yu Gothic UI", "Malgun Gothic", "SimSun-ExtB", "Segoe UI Symbol"}
		}
		for _, candidate := range families {
			if !strings.EqualFold(candidate, sf.font.family) && sf.fontForFamily(candidate).hasNativeGlyph(r) {
				family = candidate
				break
			}
		}
	}
	// Bound per-scaled-font state, including negative coverage results. Font
	// size/style/DPI are already part of the owning scaled-font cache key.
	if sf.fallbackFamilies == nil || len(sf.fallbackFamilies) >= maxFallbackRunes {
		sf.fallbackFamilies = make(map[rune]string)
	}
	sf.fallbackFamilies[r] = family
	return family
}
