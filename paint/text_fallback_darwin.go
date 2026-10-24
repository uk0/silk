//go:build darwin && !silk_pure_go

package paint

import "unicode"

// Ask the actual Cairo backend about coverage; a candidate name alone is not
// evidence that the face is installed. These are system families, not bundled
// fonts or user-specific font paths. The base ASCII face is never replaced.
func nativeFallbackFamilies(r rune) []string {
	if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
		return []string{"PingFang SC", "Heiti SC", "Hiragino Sans", "Apple SD Gothic Neo", "Songti SC", "Arial Unicode MS", "Apple Symbols"}
	}
	return []string{"Apple Color Emoji", "Apple Symbols", "Arial Unicode MS", "PingFang SC", "Heiti SC"}
}
