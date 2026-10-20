package paint

import (
	"math"
	"testing"

	"github.com/uk0/silk/geom"
)

func TestScaledFontCacheKeepsFontSize(t *testing.T) {
	var previousHeight float64
	var previous ScaledFont
	for _, size := range []int{14, 20, 28} {
		font := NewFont("sans-serif", size, false, false)
		scaled := font.ScaledFont(nil)
		height := scaled.FontExtents().Height
		t.Logf("size=%d height=%g", size, height)
		// The native Cairo backend exposes its actual font matrix; purecairo
		// does not, but still has to pass every measured-height/cache assertion.
		if native, ok := scaled.(interface{ GetFontMatrix(*geom.Mat3x2) }); ok {
			var matrix geom.Mat3x2
			native.GetFontMatrix(&matrix)
			if matrix.Xx != float64(size) || matrix.Yy != float64(size) {
				t.Errorf("size %d reused a different font matrix: %v", size, matrix)
			}
		}
		if height <= previousHeight || scaled == previous {
			t.Errorf("font size %d did not increase measured height: %g <= %g", size, height, previousHeight)
		}
		if again := NewFont("sans-serif", size, false, false).ScaledFont(nil); again != scaled {
			t.Error("equivalent fonts did not reuse their cached scaled font")
		}
		previous, previousHeight = scaled, height
	}
}

func TestScaledFontCacheKeepsWeightAndSlant(t *testing.T) {
	seen := map[ScaledFont]bool{}
	for _, style := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		font := NewFont("sans-serif", 19, style[0], style[1])
		scaled := font.ScaledFont(nil)
		if seen[scaled] {
			t.Fatalf("bold=%v italic=%v reused a different style", style[0], style[1])
		}
		seen[scaled] = true
		if !scaled.Font().Equal(font) {
			t.Fatal("cached font description differs from requested style")
		}
		if font.ScaledFont(nil) != scaled {
			t.Fatal("same style was not cached")
		}
	}
}

func TestScaledFontCacheSurvivesFontMutation(t *testing.T) {
	font := NewFont("sans-serif", 14, false, false)
	small := font.ScaledFont(nil)
	font.SetSize(28)
	large := font.ScaledFont(nil)
	font.SetBold(true)
	bold := font.ScaledFont(nil)
	if small == large || large == bold || small == bold {
		t.Fatal("font mutation reused a stale cache entry")
	}
	if small.Font().Size() != 14 || small.Font().Bold() || large.Font().Size() != 28 || large.Font().Bold() || !bold.Font().Bold() {
		t.Fatal("font mutation changed cached font descriptions")
	}
	font.SetBold(false)
	font.SetSize(14)
	if font.ScaledFont(nil) != small {
		t.Fatal("returning to the original font missed its cached entry")
	}
}

func TestScaledFontCacheCTMUsesImmutableValues(t *testing.T) {
	font := NewFont("sans-serif", 21, false, false)
	identity := geom.Mat3x2{Xx: 1, Yy: 1}
	if font.ScaledFont(nil) != font.ScaledFont(&identity) {
		t.Error("nil and explicit identity CTM did not reuse the same font")
	}
	matrix := geom.Mat3x2{Xx: 1.5, Yx: 0.1, Xy: 0.2, Yy: 2, X0: 3, Y0: 4}
	scaled := font.ScaledFont(&matrix)
	// Equal transform values at distinct addresses must reach the same bucket.
	for i := 0; i < 16; i++ {
		copy := matrix
		if got := font.ScaledFont(&copy); got != scaled {
			t.Errorf("equal CTM copy %d missed the cache", i)
		}
	}
	original := matrix
	matrix.Xx = 3
	changed := font.ScaledFont(&matrix)
	if changed == scaled {
		t.Error("mutating caller CTM reused stale scaled font")
	}
	if restored := font.ScaledFont(&original); restored != scaled {
		t.Error("mutating caller CTM corrupted original cache entry")
	}
	zero := geom.Mat3x2{Xx: 1, Yy: 1, Xy: math.Copysign(0, -1)}
	if font.ScaledFont(&zero) != font.ScaledFont(&identity) {
		t.Error("equal positive/negative zero CTMs hashed differently")
	}
}

func TestScaledFontCacheIgnoresTranslationButKeepsLinearTransform(t *testing.T) {
	f := NewFont("sans-serif", 13, false, false)
	matrix := geom.Mat3x2{Xx: 1.25, Yy: 1.5}
	original := f.ScaledFont(&matrix)
	before := scaledFontCache.Len()
	for i := 0; i < 1000; i++ {
		matrix.X0, matrix.Y0 = float64(i)/3, float64(-i)/7
		if f.ScaledFont(&matrix) != original {
			t.Fatal("widget movement creates redundant scaled fonts")
		}
	}
	if scaledFontCache.Len() != before {
		t.Fatal("translation-only drawing grew the font cache")
	}
	matrix.Xx = 2
	if f.ScaledFont(&matrix) == original {
		t.Fatal("DPI scaling lost its own font metrics")
	}
}
