package paint

import (
	"testing"

	"github.com/uk0/silk/geom"
)

// TestHashScaledFontCacheKeyRuns guards the scaled-font cache key hash against
// being handed a bare uintptr again. While the key address travelled as an
// integer it carried no pointer provenance, so every gui and ged test binary
// built with -race died with "fatal error: checkptr: pointer arithmetic result
// points to invalid allocation" on the first text measurement, before a single
// test ran.
func TestHashScaledFontCacheKeyRuns(t *testing.T) {
	k := &scaledFontCacheKey{&geom.Mat3x2{1, 0, 0, 1, 0, 0}, nil}
	if hashScaledFontCacheKey(k) != hashScaledFontCacheKey(k) {
		t.Fatal("hashScaledFontCacheKey is not deterministic for one key")
	}
}

// TestMurmur3_32 covers the word loop and every tail length. The loop reads the
// input by index now instead of by pointer arithmetic, so an off-by-one either
// panics on a short slice or silently rehashes the first word and makes keys
// that differ only in a later word collide.
func TestMurmur3_32(t *testing.T) {
	base := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	for n := 0; n <= len(base); n++ {
		if got := murmur3_32(base[:n]); got != murmur3_32(base[:n]) {
			t.Errorf("len %d: hash is not deterministic", n)
		}
	}

	for i := range base {
		other := append([]byte(nil), base...)
		other[i]++
		if murmur3_32(base) == murmur3_32(other) {
			t.Errorf("byte %d does not affect the hash", i)
		}
	}
}
