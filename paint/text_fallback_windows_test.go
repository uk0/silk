//go:build windows && !silk_pure_go

package paint

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"unsafe"

	"github.com/uk0/silk/geom"
)

func requireFallbackCoverage(t *testing.T, sf *scaledFont, text string) {
	t.Helper()
	for _, r := range text {
		family := sf.fallbackFamily(r)
		if !sf.fontForFamily(family).hasNativeGlyph(r) {
			if os.Getenv("SILK_TEST_FONT_FALLBACK") == "1" {
				t.Fatalf("required installed fallback missing for %U", r)
			}
			t.Skipf("host has no installed fallback for %U", r)
		}
		t.Logf("%U -> %q", r, family)
	}
}

func TestWindowsTextFallbackASCII(t *testing.T) {
	f := NewFont("Consolas", 16, false, false).(*font)
	sf := f.scaledFont(nil)
	text := "func main() { return 123 }"
	if needsTextFallback(text) {
		t.Fatal("ASCII took fallback path")
	}
	if got, want := *sf.TextExtents(text), TextExtents(*sf.ScaledFont.TextExtents(text)); got != want {
		t.Fatalf("ASCII metrics changed: %+v != %+v", got, want)
	}
	if got := sf.fallbackFamily('\ue000'); got != "" {
		t.Fatalf("icon code point substituted by unrelated family %q", got)
	}
}

func TestWindowsTextFallbackRuns(t *testing.T) {
	sf := NewFont("Consolas", 16, false, false).(*font).scaledFont(nil)
	requireFallbackCoverage(t, sf, "你好🚀𠀀")
	text := "A你好B🚀𠀀C"
	var parts, families []string
	sf.forEachTextRun(text, func(run *scaledFont, part string) {
		parts = append(parts, part)
		families = append(families, run.font.family)
		if part == "A" || part == "B" || part == "C" {
			if run != sf {
				t.Fatal("ASCII lost its original monospace face")
			}
		}
		for _, r := range part {
			if !run.hasNativeGlyph(r) {
				t.Fatalf("run %q still has .notdef for %U", run.font.family, r)
			}
		}
	})
	if strings.Join(parts, "") != text || len(parts) < 5 {
		t.Fatalf("bad run segmentation: %q / %q", parts, families)
	}
	// Preserve combining/variation sequences rather than separating the mark.
	parts = nil
	sf.forEachTextRun("A你\u0301\ufe0fB", func(_ *scaledFont, part string) { parts = append(parts, part) })
	if len(parts) != 3 || parts[1] != "你\u0301\ufe0f" {
		t.Fatalf("detached continuation: %q", parts)
	}
}

func TestWindowsTextFallbackMeasurementAndPixels(t *testing.T) {
	for _, zoom := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprint(zoom), func(t *testing.T) {
			var ctm geom.Mat3x2
			ctm.InitScale(zoom, zoom)
			f := NewFont("Consolas", 16, false, false).(*font)
			sf := f.scaledFont(&ctm)
			requireFallbackCoverage(t, sf, "你好🚀𠀀")
			// Independent, explicit runs: expected pixels are raw single-face
			// glyph draws, not another call to DrawText or forEachTextRun.
			parts := []string{"A", "你好", "B", "🚀", "𠀀", "C"}
			actual, expected := NewPixmap(600, 120), NewPixmap(600, 120)
			defer CloseSurface(actual)
			defer CloseSurface(expected)
			g, ref := actual.NewPainter().(*cairoPainter), expected.NewPainter().(*cairoPainter)
			defer ClosePainter(g)
			defer ClosePainter(ref)
			for _, p := range []*cairoPainter{g, ref} {
				p.Scale(zoom, zoom)
				p.Translate(8, 28)
				p.SetFont(f)
				p.SetBrush1(Color{A: 255})
			}
			var x float64
			for _, part := range parts {
				r := []rune(part)[0]
				run := sf.fontForFamily(sf.fallbackFamily(r))
				ref.cairo.SetScaledFont(run.ScaledFont)
				ref.applyBrush()
				glyphs := run.TextToGlyphs(x, 0, part)
				ref.cairo.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), len(glyphs))
				x += run.ScaledFont.TextExtents(part).XAdvance
				if run.ctm != sf.ctm {
					t.Fatal("fallback lost DPI/transform")
				}
			}
			text := strings.Join(parts, "")
			if got := sf.TextExtents(text).XAdvance; math.Abs(got-x) > .001 {
				t.Fatalf("measured advance=%g, actual runs=%g", got, x)
			}
			g.DrawText(text)
			// Follow a fallback-ending draw with a raw glyph draw: the native
			// font must be restored even when applyFont reuses its cache.
			g.DrawText1(0, 40, "你")
			run := sf.fontForFamily(sf.fallbackFamily('你'))
			ref.cairo.SetScaledFont(run.ScaledFont)
			glyphs := run.TextToGlyphs(0, 40, "你")
			ref.cairo.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), len(glyphs))
			ref.cairo.SetScaledFont(sf.ScaledFont)
			glyphs = sf.TextToGlyphs(50, 40, "ASCII")
			g.DrawGlyphs(glyphs)
			ref.cairo.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), len(glyphs))
			actual.Flush()
			expected.Flush()
			a := unsafe.Slice((*byte)(actual.DataPtr()), actual.Stride()*actual.Height())
			b := unsafe.Slice((*byte)(expected.DataPtr()), expected.Stride()*expected.Height())
			if !bytes.Equal(a, b) {
				t.Fatal("fallback text differs from native per-font glyph rendering")
			}
		})
	}
}

func TestWindowsTextFallbackCacheBounded(t *testing.T) {
	sf := NewFont("Consolas", 17, false, false).(*font).scaledFont(nil)
	sf.fallbackFamilies = make(map[rune]string)
	for i := 0; i < maxFallbackRunes; i++ {
		sf.fallbackFamilies[rune(0x20000+i)] = ""
	}
	sf.fallbackFamily('你')
	if len(sf.fallbackFamilies) > maxFallbackRunes {
		t.Fatal("coverage cache exceeded bound")
	}
	if got := sf.fallbackFamily('\U0010ffff'); got != "" {
		t.Fatalf("noncharacter unexpectedly resolved to %q", got)
	}
	if _, ok := sf.fallbackFamilies['\U0010ffff']; !ok {
		t.Fatal("missing coverage was not cached")
	}
	styled := NewFont("Consolas", 19, true, true).(*font).scaledFont(nil)
	f := styled.fontForFamily("Microsoft YaHei UI")
	if f.font.size != 19 || !f.font.bold || !f.font.italic {
		t.Fatal("fallback lost style")
	}
}

func TestWindowsTextFallbackWhitespaceExtents(t *testing.T) {
	sf := NewFont("Consolas", 16, false, false).(*font).scaledFont(nil)
	requireFallbackCoverage(t, sf, "你")
	run := sf.fontForFamily(sf.fallbackFamily('你'))
	ink := run.ScaledFont.TextExtents("你")
	space := sf.ScaledFont.TextExtents(" ").XAdvance
	got := sf.TextExtents(" 你 ")
	want := TextExtents{
		XBearing: space + ink.XBearing, YBearing: ink.YBearing,
		Width: ink.Width, Height: ink.Height,
		XAdvance: 2*space + ink.XAdvance, YAdvance: ink.YAdvance,
	}
	if *got != want {
		t.Fatalf("spaces changed ink bounds or advance: %+v != %+v", *got, want)
	}
}

func BenchmarkWindowsTextFallbackWarm(b *testing.B) {
	sf := NewFont("Consolas", 16, false, false).(*font).scaledFont(nil)
	const text = "// 你好 世界 123"
	sf.TextExtents(text)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sf.TextExtents(text)
	}
}
