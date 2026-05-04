package glui

import "testing"

// TestFontGlyphCacheASCII makes sure every printable ASCII rune produces
// either a cached glyph with a region or a non-zero advance for whitespace
// — no crashes, no zero-advance for non-blank glyphs.
func TestFontGlyphCacheASCII(t *testing.T) {
	f := NewFont()
	for ch := rune(0x20); ch < rune(0x7f); ch++ {
		g := f.Glyph(ch)
		if ch == ' ' {
			// Space typically has no mask but does advance.
			if g.advance == 0 {
				t.Fatalf("space rune should have advance > 0")
			}
			continue
		}
		if g.region.W == 0 && g.advance == 0 {
			t.Fatalf("printable rune %q produced empty glyph + zero advance", ch)
		}
	}
}

// TestFontMeasureText verifies that measuring a fixed-width string at
// 7 pixels per glyph yields exactly 7 * len(s) for ASCII.
func TestFontMeasureText(t *testing.T) {
	f := NewFont()
	const s = "hello"
	w := f.MeasureText(s)
	// basicfont.Face7x13 has uniform 7-pixel advance for ASCII glyphs.
	want := float32(len(s)) * 7
	if w != want {
		t.Fatalf("MeasureText(%q) = %g, want %g", s, w, want)
	}
}

// TestFontAtlasFills checks that loading the entire ASCII range puts at
// least one non-zero pixel into the CPU-side atlas buffer.
func TestFontAtlasFills(t *testing.T) {
	f := NewFont()
	for ch := rune(0x20); ch < rune(0x7f); ch++ {
		f.Glyph(ch)
	}
	pix := f.AtlasPixels()
	nonZero := 0
	for _, b := range pix {
		if b != 0 {
			nonZero++
		}
	}
	if nonZero < 100 {
		t.Fatalf("only %d non-zero atlas pixels — atlas likely not populated", nonZero)
	}
}

// TestFontMetrics confirms Ascent/Descent/LineHeight are positive and
// internally consistent for the default face.
func TestFontMetrics(t *testing.T) {
	f := NewFont()
	if f.Ascent() <= 0 {
		t.Errorf("ascent %g is not positive", f.Ascent())
	}
	if f.Descent() < 0 {
		t.Errorf("descent %g is negative", f.Descent())
	}
	if lh := f.LineHeight(); lh < f.Ascent() {
		t.Errorf("line height %g is smaller than ascent %g", lh, f.Ascent())
	}
}
