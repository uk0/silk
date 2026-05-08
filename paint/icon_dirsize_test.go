package paint

import "testing"

// TestDirSizeFromNameMatchesNxN: the new helper that lets preload1
// derive a sub-icon's size from "16x16" / "22x22" / "32x32" parent
// directory names. Without this fix all the project's resource
// icons collapsed to size=1 and overwrote each other in iconSrc.
func TestDirSizeFromNameMatchesNxN(t *testing.T) {
	cases := map[string]int{
		"16x16":    16,
		"22x22":    22,
		"32x32":    32,
		"48x48":    48,
		"64x128":   64,  // first half is the side; M is ignored.
		"":         0,
		"icons":    0,
		"x32":      0,
		"32x":      0,
		"abc16x16": 0,  // strict prefix — no leading garbage.
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := dirSizeFromName(in); got != want {
				t.Errorf("dirSizeFromName(%q) = %d, want %d", in, got, want)
			}
		})
	}
}
