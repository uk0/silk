package paint

import "testing"

// TestTextToGlyphsIntoMatchesTextToGlyphs proves the reusable-buffer shaping
// path produces byte-identical glyphs to the allocating TextToGlyphs, so the
// DrawText optimization changes allocations only, never rendered output.
func TestTextToGlyphsIntoMatchesTextToGlyphs(t *testing.T) {
	pm := NewPixmap(200, 60)
	g := pm.NewPainter().(*cairoPainter)
	g.SetFont(NewFont("宋体", 14, false, false))
	sf := g.applyFont()

	const s = "Row label 123"
	want := sf.TextToGlyphs(0, 0, s)
	got := sf.textToGlyphsInto(0, 0, s, nil)
	if len(got) != len(want) {
		t.Fatalf("glyph count: into=%d alloc=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("glyph %d differs: into=%+v alloc=%+v", i, got[i], want[i])
		}
	}
}

// TestDrawTextReusesGlyphBuffer verifies a second same-length DrawText reuses
// the painter's glyph buffer instead of reallocating it.
func TestDrawTextReusesGlyphBuffer(t *testing.T) {
	pm := NewPixmap(200, 60)
	g := pm.NewPainter().(*cairoPainter)
	g.SetFont(NewFont("宋体", 14, false, false))

	g.DrawText("hello")
	if len(g.glyphBuf) == 0 {
		t.Fatal("glyphBuf empty after DrawText")
	}
	cap1 := cap(g.glyphBuf)
	g.DrawText("world") // same rune count -> must reuse the backing array
	if cap(g.glyphBuf) != cap1 {
		t.Errorf("glyphBuf regrew on same-length text: cap %d -> %d", cap1, cap(g.glyphBuf))
	}
}

// TestDrawTextAllocsReduced guards the win: once the buffer is primed for a
// given text, redrawing it allocates no fresh glyph slice.
func TestDrawTextAllocsReduced(t *testing.T) {
	pm := NewPixmap(200, 60)
	g := pm.NewPainter()
	g.SetFont(NewFont("宋体", 14, false, false))
	g.DrawText("Row label") // prime the buffer at this length
	avg := testing.AllocsPerRun(200, func() { g.DrawText("Row label") })
	if avg > 2 {
		t.Errorf("DrawText allocs/op = %v, want <= 2 (glyph buffer should be reused)", avg)
	}
}
