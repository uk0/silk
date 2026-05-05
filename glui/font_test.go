package glui

import "testing"

// TestFontGlyphCacheASCII makes sure every printable ASCII rune produces
// either a cached glyph with a region or a non-zero advance for whitespace
// — no crashes, no zero-advance for non-blank glyphs.
func TestFontGlyphCacheASCII(t *testing.T) {
	f := NewFont(14)
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

// TestFontMeasureText verifies that measuring a string returns a positive
// width that scales with the number of characters. opentype produces
// proportional advances, so we can't pin an exact value, but the result
// must be monotonically increasing in length and bounded sensibly.
func TestFontMeasureText(t *testing.T) {
	f := NewFont(14)
	short := f.MeasureText("hi")
	long := f.MeasureText("hello")
	if short <= 0 {
		t.Fatalf("MeasureText(%q) = %g; want positive", "hi", short)
	}
	if long <= short {
		t.Fatalf("MeasureText(%q)=%g should exceed MeasureText(%q)=%g", "hello", long, "hi", short)
	}
}

// TestFontAtlasFills checks that loading the entire ASCII range puts at
// least one non-zero pixel into the CPU-side atlas buffer.
func TestFontAtlasFills(t *testing.T) {
	f := NewFont(14)
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
	f := NewFont(14)
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

// TestFontHelloWorldRasterises is a sanity check that an opentype-backed
// face rasterises a mixed-script string into the atlas. Go Regular has no
// CJK glyphs, so the second half is expected to fall back to a missing
// glyph (zero region, non-zero advance), but the first half MUST produce
// real anti-aliased coverage.
func TestFontHelloWorldRasterises(t *testing.T) {
	f := NewFont(14)
	const s = "Hello, 世界"
	for _, ch := range s {
		f.Glyph(ch)
	}
	pix := f.AtlasPixels()
	// Distinct grayscale values are evidence of true alpha rasterisation
	// rather than a 1-bit mask. We expect at least 4 distinct non-zero
	// values in any reasonable rasterised font subset.
	seen := make(map[byte]struct{})
	for _, b := range pix {
		if b == 0 {
			continue
		}
		seen[b] = struct{}{}
	}
	if len(seen) < 4 {
		t.Fatalf("atlas has %d distinct non-zero values; expected anti-aliased grayscale (>=4)", len(seen))
	}
}
