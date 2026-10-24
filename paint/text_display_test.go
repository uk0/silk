package paint

import (
	"image/color"
	"reflect"
	"testing"
)

func TestDisplayMalformedText(t *testing.T) {
	for _, tc := range []struct{ name, raw, display string }{
		{"invalid", "one a\xffb", "one a\ufffdb"},
		{"adjacent", "a\xff\xfez", "a\ufffd\ufffdz"},
		{"broken_sequence", "seam \xe2X\x82\xac end", "seam \ufffdX\ufffd\ufffd end"},
		{"nul", "a\x00z", "a\u2400z"},
		{"unicode", "中😀\xffend", "中😀\ufffdend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pm := NewPixmap(260, 80)
			defer CloseSurface(pm)
			g := pm.NewPainter().(*cairoPainter)
			defer ClosePainter(g)
			g.SetFont(NewFont("monospace", 16, false, false))
			sf := g.applyFont()
			want := sf.TextToGlyphs(0, 0, tc.display)
			if got := sf.TextToGlyphs(0, 0, tc.raw); len(got) == 0 || !reflect.DeepEqual(got, want) {
				t.Errorf("glyphs differ: got %v want %v", got, want)
			}
			if got := sf.textToGlyphsInto(0, 0, tc.raw, nil); !reflect.DeepEqual(got, want) {
				t.Errorf("buffer glyphs differ")
			}
			expected := *sf.TextExtents(tc.display)
			if got := *sf.TextExtents(tc.raw); got != expected || got.XAdvance <= 0 {
				t.Errorf("extents=%+v want %+v", got, expected)
			}
			draw := func(text string) []uint32 {
				pix := NewPixmap(260, 80)
				defer CloseSurface(pix)
				p := pix.NewPainter().(*cairoPainter)
				defer ClosePainter(p)
				p.SetFont(NewFont("monospace", 16, false, false))
				p.SetBrush(Color{A: 255})
				p.DrawText1(4, 24, text)
				p.DrawText1(4, 60, "AFTER")
				if status := p.cairo.Status(); status != 0 {
					t.Errorf("paint context error: %v", status)
				}
				img, err := pix.Image()
				if err != nil {
					t.Fatal(err)
				}
				var pixels []uint32
				ink := [2]bool{}
				for y := 0; y < 80; y++ {
					for x := 0; x < 260; x++ {
						r, g, b, a := color.RGBAModel.Convert(img.At(x, y)).RGBA()
						pixels = append(pixels, r, g, b, a)
						if a != 0 {
							ink[y/40] = true
						}
					}
				}
				if !ink[0] || !ink[1] {
					t.Errorf("missing text row(s): %v", ink)
				}
				return pixels
			}
			if !reflect.DeepEqual(draw(tc.raw), draw(tc.display)) {
				t.Error("raw/display pixels differ")
			}
		})
	}
}

func TestDisplayTextKeepsColumnsAndValidFastPath(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"ASCII", "ASCII"}, {"中😀", "中😀"}, {"a\xff\xfez", "a\ufffd\ufffdz"}, {"\x00", "\u2400"}, {"\xe2X\x82\xac", "\ufffdX\ufffd\ufffd"},
	} {
		if got := displayText(tc.raw); got != tc.want {
			t.Errorf("displayText(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
	// Pure helper: no native surfaces/workers are created in this measurement.
	if n := testing.AllocsPerRun(100, func() { _ = displayText("ordinary 中😀 text") }); n != 0 {
		t.Fatalf("valid display allocs=%v", n)
	}
}

func TestGlyphBufferEmptyTextClearsPreviousGlyphs(t *testing.T) {
	pm := NewPixmap(100, 40)
	defer CloseSurface(pm)
	p := pm.NewPainter().(*cairoPainter)
	defer ClosePainter(p)
	p.SetFont(NewFont("monospace", 14, false, false))
	sf := p.applyFont()
	buf := sf.textToGlyphsInto(0, 0, "previous", nil)
	if len(buf) == 0 {
		t.Fatal("initial glyphs absent")
	}
	if got := sf.textToGlyphsInto(0, 0, "", buf); len(got) != 0 {
		t.Fatal("empty text reused stale glyphs")
	}
}
