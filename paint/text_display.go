package paint

import (
	"strings"
	"unicode/utf8"
)

// displayText protects native text APIs from malformed UTF-8 and C-string NUL
// truncation. This is display-only: callers retain the original document bytes.
// Each invalid byte becomes one replacement rune, matching Go/editor columns;
// strings.ToValidUTF8 would incorrectly collapse adjacent invalid bytes.
func displayText(text string) string {
	if utf8.ValidString(text) && strings.IndexByte(text, 0) < 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for _, r := range text {
		if r == 0 {
			r = '\u2400'
		}
		out.WriteRune(r)
	}
	return out.String()
}
