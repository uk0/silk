package purecairo_test

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/uk0/silk/purecairo"
)

// TestStandaloneRender shows that purecairo can be exercised without
// any silk-specific glue: surface → context → draw → encode PNG.
//
// Mirrors the README usage example so the example stays in sync with
// the implementation.
func TestStandaloneRender(t *testing.T) {
	s := purecairo.NewImageSurface(purecairo.FORMAT_ARGB32, 400, 300)
	c := s.NewContext()

	// White background.
	c.SetSourceRGB(1, 1, 1)
	c.Paint()

	// Filled rectangle.
	c.SetSourceRGB(0.2, 0.4, 0.9)
	c.Rectangle(50, 50, 300, 200)
	c.Fill()

	// Stroked circle.
	c.SetSourceRGB(0.9, 0.2, 0.2)
	c.SetLineWidth(4)
	c.Arc(200, 150, 70, 0, 2*3.141592653589793)
	c.Stroke()

	// Encode to PNG and verify it parses back as a 400x300 image.
	var buf bytes.Buffer
	if err := s.WritePNGToStream(&buf); err != nil {
		t.Fatalf("WritePNG: %v", err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 300 {
		t.Fatalf("got %v want 400x300", b)
	}

	// Spot-check a rectangle pixel — should be the blue we filled with,
	// at roughly RGB(51, 102, 229) after the 0..1 → 0..255 conversion.
	r, g, b, _ := img.At(200, 100).RGBA()
	r8, g8, b8 := r>>8, g>>8, b>>8
	if !(r8 < 80 && g8 > 80 && b8 > 200) {
		t.Errorf("rect pixel = (%d, %d, %d), expected ~(51, 102, 229)", r8, g8, b8)
	}

	// Spot-check outside the rectangle — should be white from the Paint.
	r, g, b, _ = img.At(20, 20).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Errorf("background pixel = (%d, %d, %d), expected ~white", r>>8, g>>8, b>>8)
	}
}

// TestGradientFill verifies linear gradient interpolation between two
// stops produces an actual gradient — first colour at one end, second
// at the other, blended in between.
func TestGradientFill(t *testing.T) {
	s := purecairo.NewImageSurface(purecairo.FORMAT_ARGB32, 200, 50)
	c := s.NewContext()

	pat := purecairo.NewLinearPattern(0, 0, 200, 0)
	pat.AddColorStopRGBA(0, 1, 0, 0, 1) // red
	pat.AddColorStopRGBA(1, 0, 0, 1, 1) // blue

	c.SetSource(pat)
	c.Rectangle(0, 0, 200, 50)
	c.Fill()

	img, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	// Left edge — red dominant.
	r, _, b, _ := img.At(5, 25).RGBA()
	if r>>8 < 200 || b>>8 > 60 {
		t.Errorf("left = R=%d B=%d, expected red dominant", r>>8, b>>8)
	}
	// Right edge — blue dominant.
	r, _, b, _ = img.At(195, 25).RGBA()
	if b>>8 < 200 || r>>8 > 60 {
		t.Errorf("right = R=%d B=%d, expected blue dominant", r>>8, b>>8)
	}
}

// TestTextRenders verifies the cross-platform font discovery + opentype
// rasteriser actually produces glyph pixels for ASCII text.
func TestTextRenders(t *testing.T) {
	s := purecairo.NewImageSurface(purecairo.FORMAT_ARGB32, 200, 60)
	c := s.NewContext()

	c.SetSourceRGB(1, 1, 1)
	c.Paint()

	c.SetSourceRGB(0, 0, 0)
	c.SelectFontFace("", purecairo.FONT_SLANT_NORMAL, purecairo.FONT_WEIGHT_NORMAL)
	c.MoveTo(20, 35)
	c.ShowText("Hello, purecairo")

	img, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	// Count non-white pixels; ShowText must have written *some* glyph
	// coverage, otherwise the font path is broken.
	dark := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			if r>>8 < 200 && g>>8 < 200 && bb>>8 < 200 {
				dark++
			}
		}
	}
	if dark < 50 {
		t.Errorf("only %d dark pixels — text did not render", dark)
	}
}
