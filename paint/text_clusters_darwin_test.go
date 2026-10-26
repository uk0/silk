//go:build darwin && cgo && !silk_pure_go

package paint

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/geom"
)

// Probe the native bridge directly: disabling the integration for a controlled
// regression replay must fail the assertions, not silently skip the tests.
func requireNativeClusterBackend(t *testing.T) {
	t.Helper()
	sf := NewFont("Menlo", 20, false, false).(*font).scaledFont(nil)
	if _, ok := nativeShapeCluster(sf, "e\u0301"); !ok {
		t.Skip("Cairo Quartz font backend unavailable")
	}
}

func TestNativeClustersComposeAndRetainFonts(t *testing.T) {
	requireNativeClusterBackend(t)
	f := NewFont("Menlo", 20, false, false)
	for _, text := range []string{"🙂‍↕️", "👩‍💻", "🇨🇳", "👍🏽", "e\u0301", "1\ufe0f\u20e3", "가"} {
		t.Run(fmt.Sprintf("%x", []rune(text)), func(t *testing.T) {
			layout := LayoutText(f.ScaledFont(nil), text)
			n := utf8.RuneCountInString(text)
			if len(layout.Clusters) != 1 || layout.Clusters[0] != (TextCluster{0, n}) || len(layout.Carets) != n+1 {
				t.Fatalf("lost cluster: %+v", layout)
			}
			count := 0
			for _, r := range layout.Runs {
				count += len(r.Glyphs)
				for _, g := range r.Glyphs {
					if cairo.Glyph(g).Index() == 0 {
						t.Fatal("missing shaped glyph")
					}
				}
			}
			if count != 1 {
				t.Fatalf("composed fixture remained %d glyphs", count)
			}
			for _, x := range layout.Carets[:n] {
				if x != 0 {
					t.Fatal("internal code point acquired independent width")
				}
			}
			if layout.Carets[n] <= 0 || math.Abs(layout.Carets[n]-f.TextExtents(text).XAdvance) > .001 {
				t.Fatal("draw/measure disagree")
			}
		})
	}
}

func TestNativeClustersMixedMetricsAndPixelParity(t *testing.T) {
	requireNativeClusterBackend(t)
	text := "A中🙂‍↕️B e\u0301 🇨🇳 👍🏽Z"
	for _, zoom := range []float64{1, 1.25, 2} {
		t.Run(fmt.Sprint(zoom), func(t *testing.T) {
			var ctm geom.Mat3x2
			ctm.InitScale(zoom, zoom)
			f := NewFont("Menlo", 20, true, true)
			sf := f.ScaledFont(&ctm)
			layout := LayoutText(sf, text)
			if len(layout.Clusters) != 4 {
				t.Fatalf("cluster count=%d", len(layout.Clusters))
			}
			// Ordinary prefix keeps exactly the legacy fallback metrics.
			if got, want := layout.Carets[2], sf.TextExtents("A中").XAdvance; got != want {
				t.Fatalf("ordinary prefix changed %g != %g", got, want)
			}
			actual, expected := NewPixmap(800, 180), NewPixmap(800, 180)
			defer CloseSurface(actual)
			defer CloseSurface(expected)
			a, b := actual.NewPainter(), expected.NewPainter()
			defer ClosePainter(a)
			defer ClosePainter(b)
			for _, p := range []Painter{a, b} {
				p.Scale(zoom, zoom)
				p.Translate(10, 40)
				p.SetFont(f)
				p.SetBrush1(Color{R: 60, G: 90, B: 140, A: 180})
				p.Rectangle(0, -35, 600, 120)
				p.Clip()
			}
			a.DrawText(text)
			for _, run := range layout.Runs {
				b.SetFont(run.Font)
				b.DrawGlyphs(run.Glyphs)
			}
			// Subsequent raw glyph draw must not inherit the final emoji face.
			b.SetFont(f)
			glyphs := sf.TextToGlyphs(0, 50, "ASCII")
			a.DrawGlyphs(glyphs)
			b.DrawGlyphs(glyphs)
			actual.Flush()
			expected.Flush()
			if !bytes.Equal(unsafe.Slice((*byte)(actual.DataPtr()), actual.Stride()*actual.Height()), unsafe.Slice((*byte)(expected.DataPtr()), expected.Stride()*expected.Height())) {
				t.Fatal("text/retained-run pixels or painter restoration diverged")
			}
		})
	}
}

func TestNativeClustersOwnershipCacheAndWrap(t *testing.T) {
	requireNativeClusterBackend(t)
	sf := NewFont("Menlo", 20, false, false).(*font).scaledFont(nil)
	const text = "A🙂‍↕️B"
	a := LayoutText(sf, text)
	expected := a.Carets[len(a.Carets)-1]
	a.Carets[1] = 900
	a.Runs[0].Glyphs[0].X = 800
	a.Runs[0].Font.SetSize(300)
	a.Clusters[0].End = 100
	b := LayoutText(sf, text)
	if b.Carets[1] == 900 || b.Runs[0].Glyphs[0].X == 800 || b.Runs[0].Font.Size() == 300 || b.Clusters[0].End == 100 || b.Carets[len(b.Carets)-1] != expected {
		t.Fatal("public layout mutation poisoned cached shape")
	}
	if b.WrapEnd(0, 3) != 1 || b.WrapEnd(1, 2) != 5 || b.Boundary(3, false) != 1 || b.Boundary(3, true) != 5 {
		t.Fatal("break split cluster")
	}
	for i := 0; i < maxComplexLayouts+12; i++ {
		sf.TextExtents(fmt.Sprintf("%d:%s", i, text))
	}
	if len(complexLayouts) > maxComplexLayouts || complexLayoutBytes > maxComplexLayoutBytes {
		t.Fatal("layout cache exceeded budget")
	}
	for _, text := range []string{"func main() {}", "你好🙂", "\ue000\ufe0f"} {
		if len(LayoutText(sf, text).Clusters) != 0 {
			t.Fatalf("ordinary/private-use layout replaced: %q", text)
		}
	}
	for _, text := range []string{"a\xffe\u0301", "a\x00🙂‍↕️"} {
		if len(LayoutText(sf, text).Carets) != utf8.RuneCountInString(text)+1 {
			t.Fatal("display sanitization lost source offsets")
		}
	}
}

func BenchmarkNativeClusterWarm(b *testing.B) {
	sf := NewFont("Menlo", 16, false, false).(*font).scaledFont(nil)
	text := strings.Repeat("A🙂‍↕️e\u0301 ", 100)
	sf.TextExtents(text)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sf.TextExtents(text)
	}
}

func TestNativeClusterFractionalFallbackSize(t *testing.T) {
	requireNativeClusterBackend(t)
	// Typographic advance is not the font size: Apple's emoji strikes can
	// use an 18-unit advance at size 13.5. Assert actual run size and distinct
	// native/scaled-font cache identity, not an assumed square emoji cell.
	for _, text := range []string{"e\u0301", "🙂‍↕️"} {
		var previous *scaledFont
		for _, size := range []float64{13, 13.5, 14, 13.5} {
			f := NewFont("Menlo", 13, false, false).(*font)
			f.nativeSize = size
			l := LayoutText(f.ScaledFont(nil), text)
			if len(l.Clusters) != 1 || len(l.Runs) != 1 || l.Carets[len(l.Carets)-1] <= 0 {
				t.Fatalf("missing fractional layout at %g: %+v", size, l)
			}
			owner := l.Runs[0].Font.(*font)
			if owner.nativeSize != size {
				t.Fatalf("requested %g, native run rounded to %g", size, owner.nativeSize)
			}
			scaled := owner.scaledFont(nil)
			if scaled.font.nativeSize != size || scaled == previous {
				t.Fatalf("fractional scaled-font cache collided at %g", size)
			}
			previous = scaled
		}
	}
}
