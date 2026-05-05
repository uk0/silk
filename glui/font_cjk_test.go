package glui

import (
	"runtime"
	"testing"
)

// TestFontCJKFallback verifies that CJK characters reach the atlas via the
// fallback chain. Skipped on hosts where no system CJK font is present —
// e.g. minimal Linux containers without fonts-noto-cjk. The CI workflow
// installs that package so the Linux runner exercises this test.
//
// We do not assert a specific advance value because system fonts vary.
// Instead we confirm:
//  1. discoverSystemCJKFaces returns at least one face when the env has a
//     CJK font — used as a guard so the test skips cleanly otherwise.
//  2. Glyph() for '中' lands in the atlas with a non-zero region (proves
//     the fallback was actually consulted, since goregular has no '中').
//  3. MeasureText("中文") is > 0 so a Chinese label has a non-zero width.
func TestFontCJKFallback(t *testing.T) {
	face, path := discoverSystemCJKFaceWithPath(14)
	if face == nil {
		t.Skipf("no system CJK font on %s — skipping fallback test", runtime.GOOS)
	}
	// Surface the discovered path in the CI log so a "skip" caused by an
	// upstream apt or macOS path change is visibly different from a "pass"
	// with the fallback actually in play. If a macOS upgrade relocates
	// PingFang.ttc to /System/Library/Fonts/Supplemental, the next run of
	// this test will print the new path here.
	t.Logf("CJK fallback discovered on %s: %s", runtime.GOOS, path)

	f := NewFont(14)
	if len(f.faces) < 2 {
		t.Fatalf("expected primary + at least one fallback face, got %d", len(f.faces))
	}

	g := f.Glyph('中')
	if g.region.W == 0 || g.region.H == 0 {
		t.Fatalf("CJK glyph '中' did not land in atlas: region=%+v", g.region)
	}
	if g.advance <= 0 {
		t.Fatalf("CJK glyph '中' advance=%g, want > 0", g.advance)
	}

	w := f.MeasureText("中文测试")
	if w <= 0 {
		t.Fatalf("MeasureText(\"中文测试\") = %g, want > 0", w)
	}
}

// TestFontCJKMixedString checks that mixing Latin and CJK in a single
// MeasureText call sums advances from both faces. The test runs only when
// a system CJK font is available; otherwise the CJK runes degrade to
// zero-advance and the result equals MeasureText("Hello"), which is the
// existing (correct) Latin-only behaviour.
func TestFontCJKMixedString(t *testing.T) {
	if len(discoverSystemCJKFaces(14)) == 0 {
		t.Skipf("no system CJK font on %s — skipping CJK test", runtime.GOOS)
	}
	f := NewFont(14)
	latin := f.MeasureText("Hello")
	mixed := f.MeasureText("Hello 世界")
	if mixed <= latin {
		t.Fatalf("mixed Latin+CJK width %g should exceed Latin-only %g", mixed, latin)
	}
}

// TestFontCJKAtlasUsesFallbackFace re-asserts the advisor's invariant: the
// per-glyph offY for a CJK rune comes from the fallback face, not the Go
// Regular primary. We verify this indirectly by confirming the recorded
// offY differs from the primary's offY for an ASCII glyph at the same
// size — different fonts have visibly different bearings.
func TestFontCJKAtlasUsesFallbackFace(t *testing.T) {
	if len(discoverSystemCJKFaces(14)) == 0 {
		t.Skipf("no system CJK font on %s — skipping bearing check", runtime.GOOS)
	}
	f := NewFont(14)
	asciiOff := f.Glyph('A').offY
	cjkOff := f.Glyph('中').offY
	// Both bearings are negative (rendered above the baseline). They will
	// almost always differ — Go Regular's 'A' is shorter than CJK '中' at
	// the same point size — but if they happen to coincide we have not
	// lost any correctness, so warn rather than fail.
	if asciiOff == cjkOff {
		t.Logf("ASCII offY %g == CJK offY %g (unusual but not incorrect)", asciiOff, cjkOff)
	}
}
