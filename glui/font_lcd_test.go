package glui

import (
	"testing"

	"golang.org/x/image/font"
)

// TestSubpixelLCDDefaultsOff locks in that NewFont without the env var
// stays on the standard alpha glyph path. A regression that flipped the
// default would expand the atlas memory 4× silently.
func TestSubpixelLCDDefaultsOff(t *testing.T) {
	f := NewFont(14)
	if f.SubpixelLCDEnabled() {
		t.Errorf("subpixelLCD should default to off, got on")
	}
}

// TestSetSubpixelLCDEnables verifies SetSubpixelLCD(true) flips the flag
// and SetSubpixelLCD(false) flips it back. The setter is documented as
// idempotent so calling twice with the same value should leave the cache
// untouched (we can't observe that here without exposing internals, but
// it's the contract).
func TestSetSubpixelLCDEnables(t *testing.T) {
	f := NewFont(14)
	f.SetSubpixelLCD(true)
	if !f.SubpixelLCDEnabled() {
		t.Errorf("SetSubpixelLCD(true) should enable LCD")
	}
	f.SetSubpixelLCD(false)
	if f.SubpixelLCDEnabled() {
		t.Errorf("SetSubpixelLCD(false) should disable LCD")
	}
}

// TestSubpixelLCDStorageIsRGBA verifies that enabling LCD rebuilds the
// atlas pixel buffer to 4 bytes per pixel. Existing alpha-mode code
// expects one byte per pixel, so this size change is what the LCD
// fragment shader needs to interpret the buffer as RGBA.
func TestSubpixelLCDStorageIsRGBA(t *testing.T) {
	f := NewFont(14)
	w, h := f.AtlasSize()
	if got := len(f.AtlasPixels()); got != w*h {
		t.Fatalf("alpha mode atlas: got %d bytes, want %d (1 byte/px)", got, w*h)
	}
	f.SetSubpixelLCD(true)
	if got := len(f.AtlasPixels()); got != w*h*4 {
		t.Errorf("LCD mode atlas: got %d bytes, want %d (4 bytes/px)", got, w*h*4)
	}
}

// TestRasteriseLCDProducesDistinctChannels exercises the rasteriser end
// to end on the bundled Go Regular face. For a typical Latin glyph with
// a vertical stroke ('M', 'I'), the three sub-position rasterisations
// MUST produce different per-pixel output in some pixel — otherwise the
// LCD shader has nothing to sample beyond a uniform grayscale mask.
//
// Failure modes this test pins down:
//   - Sub-positions all came back identical (rasterisation dot ignored)
//   - All three calls landed on the same face but the dr rect was
//     pinned to integer multiples of the dot, hiding the shift
//   - Channel packing accidentally wrote the same mask thrice
func TestRasteriseLCDProducesDistinctChannels(t *testing.T) {
	face, err := newGoRegularFace(14)
	if err != nil {
		t.Fatalf("newGoRegularFace: %v", err)
	}
	rgba, w, h, _, _, _, ok := rasteriseLCD([]font.Face{face}, 'M')
	if !ok {
		t.Fatalf("rasteriseLCD failed for 'M'")
	}
	if w == 0 || h == 0 {
		t.Fatalf("expected non-empty rect for 'M', got %dx%d", w, h)
	}
	if len(rgba) != w*h*4 {
		t.Fatalf("rgba len = %d, want %d", len(rgba), w*h*4)
	}

	// Walk the buffer; require at least one pixel where R != B. (R == B
	// means the 0 px and 2/3 px shifts produced identical coverage, which
	// would defeat the point of LCD striping.)
	differ := false
	for i := 0; i < len(rgba); i += 4 {
		if rgba[i] != rgba[i+2] {
			differ = true
			break
		}
	}
	if !differ {
		t.Errorf("R and B channels are byte-identical; sub-pixel shift didn't take effect")
	}
}

// TestRasteriseLCDEmptyFacesFails: with no face in the chain there's
// nothing to rasterise. Returning ok=false lets glyphForKey cache an
// empty info instead of writing garbage RGBA.
func TestRasteriseLCDEmptyFacesFails(t *testing.T) {
	rgba, w, h, _, _, _, ok := rasteriseLCD(nil, 'A')
	if ok {
		t.Errorf("expected ok=false on empty faces, got rgba=%v w=%d h=%d", rgba != nil, w, h)
	}
}

// TestRasteriseLCDWhitespace: a glyph with zero pixel extent (' ', '\t')
// still has an advance — the pen should progress even though there's no
// mask. ok must be true so the cache stores the advance-only record;
// returning false would wedge text layout against a face that prints
// nothing for whitespace.
func TestRasteriseLCDWhitespace(t *testing.T) {
	face, err := newGoRegularFace(14)
	if err != nil {
		t.Fatalf("newGoRegularFace: %v", err)
	}
	rgba, w, h, _, _, advance, ok := rasteriseLCD([]font.Face{face}, ' ')
	if !ok {
		t.Errorf("space rune should still rasterise (advance only)")
	}
	if w != 0 || h != 0 || rgba != nil {
		t.Errorf("space rune should have empty mask, got %dx%d rgba=%v", w, h, rgba != nil)
	}
	if advance <= 0 {
		t.Errorf("space rune should advance the pen, got advance=%g", advance)
	}
}

// TestSubpixelLCDInvalidatesCache: toggling LCD must drop any glyphs
// rasterised under the previous mode. Otherwise cached alpha-mode
// regions (1 byte/px addressing) would be sampled by the RGBA shader
// and produce visual garbage.
func TestSubpixelLCDInvalidatesCache(t *testing.T) {
	f := NewFont(14)
	gOff := f.Glyph('A')
	if gOff.region.W == 0 {
		t.Fatalf("expected non-empty alpha glyph for 'A'")
	}
	f.SetSubpixelLCD(true)
	// After toggle, the cache should be empty so the next Glyph call
	// performs a fresh rasterisation. We can't peek at len(f.glyphs)
	// from outside the package without an accessor, but we can verify
	// the buffer was rebuilt: it must be zeroed RGBA.
	pix := f.AtlasPixels()
	for _, b := range pix[:1024] { // sample first 1KB
		if b != 0 {
			t.Errorf("expected freshly zeroed atlas after SetSubpixelLCD(true); found non-zero byte")
			break
		}
	}
	// New rasterisation in LCD mode should still succeed.
	gOn := f.Glyph('A')
	if gOn.region.W == 0 {
		t.Errorf("expected non-empty LCD glyph for 'A' after toggle")
	}
}

// TestSubpixelLCDFontGlyphAtIgnoresFraction: LCD encodes sub-pixel
// position in channels, so the bucket cache is bypassed. Calling
// GlyphAt with different fracX values must return identical records.
// (If LCD didn't bypass buckets, we'd cache 4× LCD masks per rune for
// no visual gain — the channels already carry the shift.)
func TestSubpixelLCDFontGlyphAtIgnoresFraction(t *testing.T) {
	f := NewFont(14)
	f.SetSubpixelLCD(true)
	g00 := f.GlyphAt('A', 0.0)
	g05 := f.GlyphAt('A', 0.5)
	if g00 != g05 {
		t.Errorf("LCD mode should ignore fracX: %+v vs %+v", g00, g05)
	}
}

// TestDrawTextLCDRoutesToGlyphLCDBatch verifies that rendering a string
// against a LCD-enabled font lands in kindGlyphLCD, not kindGlyph. This
// is what selects the LCD fragment shader at flush time.
func TestDrawTextLCDRoutesToGlyphLCDBatch(t *testing.T) {
	r := newAdapterTestRenderer()
	f := NewFont(14)
	f.SetSubpixelLCD(true)

	r.DrawText(f, "Hi", 10, 50, Color{1, 1, 1, 1})
	if r.curKind != kindGlyphLCD {
		t.Errorf("expected curKind=kindGlyphLCD (%d) after LCD draw, got %d", kindGlyphLCD, r.curKind)
	}
	if len(r.verts) == 0 {
		t.Errorf("expected non-empty vertex buffer for 'Hi'")
	}
}

// TestDrawTextLCDIntegerSnap mirrors TestDrawTextSubpixelSnapsToInteger
// but for the LCD path: a half-pixel pen shift must NOT change the
// quad position, since the per-channel masks carry the shift internally.
func TestDrawTextLCDIntegerSnap(t *testing.T) {
	r := newAdapterTestRenderer()
	f := NewFont(14)
	f.SetSubpixelLCD(true)

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
		t.Errorf("LCD mode should integer-snap; got delta=%g", x1-x0)
	}
}

// TestDrawTextLCDEmptyText guards the empty-string fast path: the
// renderer must NOT touch the batch state when there's nothing to draw.
// Otherwise an LCD font in an idle frame would force a flush of the
// previous (non-LCD) batch.
func TestDrawTextLCDEmptyText(t *testing.T) {
	r := newAdapterTestRenderer()
	f := NewFont(14)
	f.SetSubpixelLCD(true)

	priorKind := r.curKind
	r.DrawText(f, "", 10, 50, Color{1, 1, 1, 1})
	if r.curKind != priorKind {
		t.Errorf("empty LCD draw mutated curKind: was %d now %d", priorKind, r.curKind)
	}
	if len(r.verts) != 0 {
		t.Errorf("empty LCD draw emitted vertices: %d", len(r.verts))
	}
}

// TestSubpixelLCDMeasureTextUnaffected: layout metrics come from the
// face — they don't depend on rasterisation mode. Toggling LCD must
// not shift advances, otherwise mixed-mode UI (some labels LCD, some
// alpha) would lay out inconsistently.
func TestSubpixelLCDMeasureTextUnaffected(t *testing.T) {
	const text = "Hello"
	off := NewFont(14)
	on := NewFont(14)
	on.SetSubpixelLCD(true)

	wOff := off.MeasureText(text)
	wOn := on.MeasureText(text)
	if wOff != wOn {
		t.Errorf("MeasureText differs: off=%g on=%g", wOff, wOn)
	}
}
