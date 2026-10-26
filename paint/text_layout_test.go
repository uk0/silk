package paint

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"runtime"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/geom"
)

func TestTextLayoutRetainsFallbackFontsAndPixels(t *testing.T) {
	family := "Menlo"
	if runtime.GOOS == "windows" {
		family = "Consolas"
	}
	text := "A中🙂B🍎C"
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			var ctm geom.Mat3x2
			ctm.InitScale(scale, scale)
			f := NewFont(family, 17, false, false)
			sf := f.ScaledFont(&ctm)
			layout := LayoutText(sf, text)
			if len(layout.Carets) != utf8.RuneCountInString(text)+1 {
				t.Fatal("lost rune boundaries")
			}
			if math.Abs(layout.Carets[len(layout.Carets)-1]-sf.TextExtents(text).XAdvance) > .001 {
				t.Fatal("fallback measurement diverged")
			}
			for _, run := range layout.Runs {
				for _, g := range run.Glyphs {
					if cairo.Glyph(g).Index() == 0 {
						if os.Getenv("SILK_TEST_FONT_FALLBACK") == "1" {
							t.Fatalf("missing glyph in retained %q run", run.Font.Family())
						}
						t.Skip("installed fonts do not cover fixture")
					}
					if g.A != 0 {
						t.Fatal("native glyph ABI overwritten by geometry")
					}
				}
			}
			actual, expected := NewPixmap(600, 150), NewPixmap(600, 150)
			defer CloseSurface(actual)
			defer CloseSurface(expected)
			g, ref := actual.NewPainter(), expected.NewPainter()
			defer ClosePainter(g)
			defer ClosePainter(ref)
			for _, p := range []Painter{g, ref} {
				p.Scale(scale, scale)
				p.Translate(8, 30)
				p.SetFont(f)
				p.SetBrush1(Color{A: 255})
			}
			// Independent text API uses its native fallback iterator; the retained
			// path paints only raw font-owned glyph indices.
			ref.DrawText(text)
			for _, run := range layout.Runs {
				g.SetFont(run.Font)
				g.DrawGlyphs(run.Glyphs)
			}
			actual.Flush()
			expected.Flush()
			a := unsafe.Slice((*byte)(actual.DataPtr()), actual.Stride()*actual.Height())
			b := unsafe.Slice((*byte)(expected.DataPtr()), expected.Stride()*expected.Height())
			if !bytes.Equal(a, b) {
				t.Fatal("retained glyphs differ from text-aware fallback pixels")
			}
			f.SetSize(40)
			f.SetFamily("serif")
			for _, run := range layout.Runs {
				if run.Font.Size() != 17 {
					t.Fatal("caller mutation changed retained font")
				}
			}
		})
	}
}

type unusualGlyphFont struct {
	ScaledFont
	calls int
}

func (s *unusualGlyphFont) TextToGlyphs(x, y float64, text string) []Glyph {
	s.calls++
	// No cluster map: entire strings collapse; individual runes expand.
	if utf8.RuneCountInString(text) > 1 {
		return []Glyph{{X: x, Y: y}}
	}
	return []Glyph{{X: x, Y: y}, {X: x + 2, Y: y}}
}
func (s *unusualGlyphFont) TextExtents(text string) *TextExtents {
	return &TextExtents{XAdvance: float64(utf8.RuneCountInString(text)) * 7.25}
}
func TestTextLayoutCustomFontPreservesEveryRune(t *testing.T) {
	sf := &unusualGlyphFont{ScaledFont: NewFont("sans", 14, false, false).ScaledFont(nil)}
	layout := LayoutText(sf, "a中🙂b")
	if sf.calls != 5 || len(layout.Runs) != 4 || len(layout.Carets) != 5 {
		t.Fatalf("not linear/lost text: %+v calls=%d", layout, sf.calls)
	}
	for i, run := range layout.Runs {
		if run.Begin != i || run.End != i+1 || len(run.Glyphs) != 2 || layout.Carets[i] != float64(i)*7.25 {
			t.Fatalf("bad compatibility run %+v", run)
		}
	}
	if layout.Carets[4] != 29 {
		t.Fatal("lost final boundary")
	}
}

func TestTextLayoutEmptyAndDisplaySanitization(t *testing.T) {
	sf := NewFont("sans", 14, false, false).ScaledFont(nil)
	for _, text := range []string{"", "a\x00b", "\xff\xff中"} {
		layout := LayoutText(sf, text)
		if len(layout.Carets) != utf8.RuneCountInString(text)+1 {
			t.Fatalf("sanitization lost offsets: %q", text)
		}
	}
}
