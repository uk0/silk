//go:build windows && !silk_pure_go

package paint

import "unicode"

func nativeFallbackFamilies(r rune) []string {
	if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
		return []string{"Microsoft YaHei UI", "Yu Gothic UI", "Malgun Gothic", "SimSun-ExtB", "Segoe UI Symbol"}
	}
	return []string{"Segoe UI Emoji", "Segoe UI Symbol", "Microsoft YaHei UI", "Yu Gothic UI", "Malgun Gothic", "SimSun-ExtB"}
}
