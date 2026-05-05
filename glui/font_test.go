package glui

import (
	"os"
	"testing"
)

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

// TestFontCacheReuse verifies that requesting the same size twice yields
// the same instance, and different sizes yield different instances.
func TestFontCacheReuse(t *testing.T) {
	c := NewFontCache()
	a := c.At(14)
	b := c.At(14)
	if a != b {
		t.Errorf("FontCache.At(14) returned different instances on repeat call")
	}
	d := c.At(16)
	if a == d {
		t.Errorf("FontCache.At(14) == FontCache.At(16); should differ by size")
	}
	// Sub-point variations rounding to the same int share an instance.
	if c.At(14.2) != a {
		t.Errorf("FontCache.At(14.2) did not share the size-14 instance")
	}
}

// TestDefaultFontShared verifies the package-level DefaultFont returns the
// same instance across calls for the same size.
func TestDefaultFontShared(t *testing.T) {
	a := DefaultFont(12)
	b := DefaultFont(12)
	if a != b {
		t.Errorf("DefaultFont(12) returned different instances on repeat call")
	}
}

// TestFontSDFModeProducesGradient flips on SILK_GLUI_SDF and checks that
// the rasterised 'M' glyph lands a gradient — multiple distinct alpha
// values — into the atlas instead of the binary mask the raster path
// produces. The histogram must contain at least 5 distinct bytes; in
// raster mode you'd typically see 2-3 (background + 1-2 anti-aliased
// edge values), so 5+ is a meaningful discriminator.
func TestFontSDFModeProducesGradient(t *testing.T) {
	os.Setenv("SILK_GLUI_SDF", "1")
	defer os.Unsetenv("SILK_GLUI_SDF")

	f := NewFont(16)
	if !f.useSDF {
		t.Skip("SDF mode not enabled")
	}

	g := f.Glyph('M')
	if g.region.W == 0 || g.region.H == 0 {
		t.Fatalf("'M' glyph rasterisation produced empty region")
	}

	// Inspect the slot the glyph occupies in the atlas.
	pix := f.AtlasPixels()
	stride := f.atlasW
	histogram := make(map[byte]int)
	for y := g.region.Y; y < g.region.Y+g.region.H; y++ {
		for x := g.region.X; x < g.region.X+g.region.W; x++ {
			i := y*stride + x
			if i < len(pix) {
				histogram[pix[i]]++
			}
		}
	}
	if len(histogram) < 5 {
		t.Errorf("SDF should produce gradient values, only got %d distinct: %v", len(histogram), histogram)
	}
}

// TestFontHelloWorldRasterises is a sanity check that an opentype-backed
// face rasterises a mixed-script string into the atlas. The Latin portion
// renders through the primary Go Regular face; the CJK portion either
// renders through a discovered system fallback (when fonts-noto-cjk or
// PingFang etc. are present) or falls back to zero-glyph records. Either
// way the Latin half MUST produce real anti-aliased coverage.
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
