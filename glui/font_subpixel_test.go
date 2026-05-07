package glui

import (
	"testing"
)

// TestSubpixelBucketQuantization checks the bucketing math: zero maps to
// bucket 0, the upper half of the unit interval maps to higher buckets,
// and out-of-range inputs wrap or clamp.
func TestSubpixelBucketQuantization(t *testing.T) {
	cases := []struct {
		in   float32
		want uint8
	}{
		{0.0, 0},
		{0.1, 0},
		{0.249, 0},
		{0.25, 1},
		{0.49, 1},
		{0.5, 2},
		{0.74, 2},
		{0.75, 3},
		{0.99, 3},
		{1.0, 0},  // wraps
		{1.5, 2},  // wraps to 0.5
		{-0.25, 3}, // -0.25 -> 0.75 -> bucket 3
	}
	for _, tc := range cases {
		got := subpixelBucket(tc.in)
		if got != tc.want {
			t.Errorf("subpixelBucket(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestSubpixelDisabledByDefault locks in that NewFont without the env
// var keeps the legacy single-bucket behaviour.
func TestSubpixelDisabledByDefault(t *testing.T) {
	f := NewFont(14)
	if f.SubpixelEnabled() {
		t.Errorf("subpixel should default to off, got on")
	}
}

// TestSubpixelOptInProducesDistinctMasks verifies that turning subpixel
// on causes opentype to emit different mask data for the same rune at
// different fractional pen offsets. The test inspects atlas pixel slices
// for two buckets and asserts they are not byte-identical.
func TestSubpixelOptInProducesDistinctMasks(t *testing.T) {
	f := NewFont(14)
	f.SetSubpixel(true)
	if !f.SubpixelEnabled() {
		t.Fatalf("SetSubpixel(true) should enable")
	}

	g0 := f.GlyphAt('m', 0.0)  // bucket 0
	g2 := f.GlyphAt('m', 0.5)  // bucket 2

	if g0.region.W == 0 || g2.region.W == 0 {
		t.Fatalf("expected non-empty masks for 'm'")
	}
	// Buckets land in distinct atlas slots.
	if g0.region.X == g2.region.X && g0.region.Y == g2.region.Y {
		t.Errorf("buckets 0 and 2 share atlas region; should be distinct")
	}
	// Bytewise compare the two slot bodies — they should differ at least
	// in some pixel because the rasterisation dot was different.
	differ := false
	pix := f.AtlasPixels()
	for y := 0; y < g0.region.H && !differ; y++ {
		for x := 0; x < g0.region.W; x++ {
			off0 := (g0.region.Y+y)*f.atlasW + g0.region.X + x
			off2 := (g2.region.Y+y)*f.atlasW + g2.region.X + x
			if off0 < len(pix) && off2 < len(pix) && pix[off0] != pix[off2] {
				differ = true
				break
			}
		}
	}
	if !differ {
		t.Errorf("bucket 0 and bucket 2 masks are byte-identical; rasteriser should have shifted by 0.5 px")
	}
}

// TestSubpixelMeasureTextUnaffected guarantees the measured advance
// is identical with subpixel on and off — only the mask differs, not
// the layout metrics.
func TestSubpixelMeasureTextUnaffected(t *testing.T) {
	const text = "Hello"
	off := NewFont(14)
	on := NewFont(14)
	on.SetSubpixel(true)

	wOff := off.MeasureText(text)
	wOn := on.MeasureText(text)
	if wOff != wOn {
		t.Errorf("MeasureText differs: off=%g on=%g", wOff, wOn)
	}
}

// TestDrawTextFractionalPenPreservesPositionWithoutSubpixel verifies
// the historical default: the resulting first vertex's clip-space X
// must shift by the expected fractional amount when the pen X moves
// by 0.5 px.
func TestDrawTextFractionalPenPreservesPositionWithoutSubpixel(t *testing.T) {
	r := newAdapterTestRenderer()
	f := NewFont(14)

	r.DrawText(f, "M", 10.0, 50, Color{1, 1, 1, 1})
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted")
	}
	x0 := r.verts[0].X
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone

	r.DrawText(f, "M", 10.5, 50, Color{1, 1, 1, 1})
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted on second call")
	}
	x1 := r.verts[0].X

	// Clip-space units: project maps logical X by (X/frameW)*2-1, and
	// frameW=100 in the test renderer. So a 0.5 logical-px shift
	// translates to (0.5/100)*2 = 0.01 in clip space.
	want := float32(0.01)
	got := x1 - x0
	if abs32(got-want) > 1e-4 {
		t.Errorf("clip-x delta = %g, want ~%g (subpixel off keeps fractional pen)", got, want)
	}
}

// TestDrawTextSubpixelSnapsToInteger verifies that with subpixel on,
// pen X = 10.0 and pen X = 10.5 produce the same first-vertex clip-X —
// the subpixel shift is encoded in the rasterised mask, not in the
// quad position.
func TestDrawTextSubpixelSnapsToInteger(t *testing.T) {
	r := newAdapterTestRenderer()
	f := NewFont(14)
	f.SetSubpixel(true)

	r.DrawText(f, "M", 10.0, 50, Color{1, 1, 1, 1})
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted")
	}
	x0 := r.verts[0].X
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.curKind = kindNone

	r.DrawText(f, "M", 10.5, 50, Color{1, 1, 1, 1})
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted on second call")
	}
	x1 := r.verts[0].X

	if abs32(x1-x0) > 1e-4 {
		t.Errorf("subpixel mode should integer-snap; got delta=%g", x1-x0)
	}
}

// TestGlyphSubBucketZeroMatchesLegacyGlyph proves that when subpixel is
// disabled the glyph cache layout is identical to the pre-refactor
// single-bucket cache: every Glyph(r) must equal GlyphAt(r, 0.0).
func TestGlyphSubBucketZeroMatchesLegacyGlyph(t *testing.T) {
	f := NewFont(14) // subpixel off by default
	g0 := f.Glyph('A')
	gAt := f.GlyphAt('A', 0.0)
	if g0 != gAt {
		t.Errorf("Glyph(r) and GlyphAt(r, 0.0) diverge with subpixel off:\n  Glyph: %+v\n  At:    %+v", g0, gAt)
	}
}

// TestGlyphSubpixelOffIgnoresFraction is the converse of the snap test
// but at the cache level: with subpixel disabled, GlyphAt(r, 0.7) must
// return the bucket-0 entry — different fracX must NOT cause new atlas
// slots, otherwise the cache would explode for callers who haven't
// opted in.
func TestGlyphSubpixelOffIgnoresFraction(t *testing.T) {
	f := NewFont(14)
	g00 := f.GlyphAt('A', 0.0)
	g07 := f.GlyphAt('A', 0.7)
	if g00 != g07 {
		t.Errorf("subpixel-off cache should ignore fracX; bucket 0 and 0.7 diverged:\n  0.0: %+v\n  0.7: %+v", g00, g07)
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
