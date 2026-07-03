package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// colorChannelDelta sums the per-channel RGB distance between two colors.
// Used as a cheap "visually distinguishable" proxy for text-on-fill checks.
func colorChannelDelta(a, b paint.Color) int {
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
}

// TestLightModeTabTextColorReadable guards against the regression where the
// light theme's inactive TabTextColor was pure white — invisible on the
// FormColor chrome fill the programmatic DrawTab uses for inactive tabs.
func TestLightModeTabTextColorReadable(t *testing.T) {
	orig := CurrentThemeMode()
	defer SetThemeMode(orig)

	SetThemeMode(ThemeLight)
	th := Theme()

	white := paint.Color{255, 255, 255, 255}
	if th.TabTextColor == white {
		t.Errorf("light-mode TabTextColor is pure white; unreadable on FormColor %v", th.FormColor)
	}
	if d := colorChannelDelta(th.TabTextColor, th.FormColor); d < 90 {
		t.Errorf("light-mode TabTextColor %v too close to FormColor %v (channel delta %d, want >= 90)",
			th.TabTextColor, th.FormColor, d)
	}
}

// TestEditSizeHintsIncludesVerticalPadding verifies a hint-height Edit fully
// contains padding.T + fontHeight + padding.B, so text and caret are not
// clipped at the hinted height.
func TestEditSizeHintsIncludesVerticalPadding(t *testing.T) {
	e := NewEdit()
	pad := e.Padding()
	fh := e.Font().FontExtents().Height

	want := fh + pad.T + pad.B
	if got := e.SizeHints().Height; got < want {
		t.Errorf("Edit SizeHints height = %v, want >= fontHeight(%v) + pad.T(%v) + pad.B(%v) = %v",
			got, fh, pad.T, pad.B, want)
	}
}
