package main

import (
	"strings"
	"testing"
)

// TestBookmarkLabelForLineTrimsAndTruncates locks in the label shape
// the BookmarksPanel rows use: whitespace stripped from both ends and
// long content capped with a single-char ellipsis. The cap matters for
// the panel width — without it a 200-character line would overrun the
// row width and clip mid-token.
func TestBookmarkLabelForLineTrimsAndTruncates(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \t\n", ""},
		{"trim leading + trailing", "   hello world   ", "hello world"},
		{"short stays whole", "func main() {", "func main() {"},
		{
			"exact 50 chars stays whole",
			strings.Repeat("a", 50),
			strings.Repeat("a", 50),
		},
		{
			"51 chars gets ellipsis",
			strings.Repeat("a", 51),
			strings.Repeat("a", 49) + "…",
		},
		{
			"long line truncated to 50 runes with ellipsis",
			"func veryLongFunctionNameThatGoesOnAndOnAndOnAndOnAndOn() {",
			"func veryLongFunctionNameThatGoesOnAndOnAndOnAndO" + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := bookmarkLabelForLine(c.in)
			if got != c.want {
				t.Errorf("bookmarkLabelForLine(%q) = %q, want %q", c.in, got, c.want)
			}
			// Ellipsis case must respect the 50-rune ceiling — count
			// runes (not bytes) because "…" is multi-byte in UTF-8.
			if r := []rune(got); len(r) > 50 {
				t.Errorf("label exceeded 50 runes: %d", len(r))
			}
		})
	}
}
