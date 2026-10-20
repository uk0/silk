package purecairo

import (
	"image/color"
	"math"
	"testing"
	"unsafe"
)

func TestExactSRGBAndPremultipliedAlpha(t *testing.T) {
	for i := 0; i < 256; i++ {
		c := f64ColorRGB(float64(i)/255, float64(i)/255, float64(i)/255, 1)
		if got := color.NRGBAModel.Convert(c).(color.NRGBA).R; got != uint8(i) {
			t.Fatalf("channel %d -> %d", i, got)
		}
	}
	s := NewImageSurface(FORMAT_ARGB32, 2, 2)
	c := s.NewContext()
	c.SetSourceRGBA(1, 0, 0, 0.5)
	c.Paint()
	got := s.img.RGBAAt(0, 0)
	if got.R != 128 || got.A != 128 {
		t.Fatalf("invalid premultiplication: %v", got)
	}
	c.SetSourceRGBA(math.NaN(), 0, 0, 1)
	c.Paint()
	if s.img.RGBAAt(0, 0).R != 0 {
		t.Fatal("NaN channel not clamped")
	}
}

func TestPathRestoreAndConsumption(t *testing.T) {
	c := NewImageSurface(FORMAT_ARGB32, 32, 32).NewContext()
	c.MoveTo(2, 3)
	c.LineTo(7, 9)
	p := c.CopyPath()
	c.NewPath()
	if c.HasCurrentPoint() {
		t.Fatal("NewPath retained current point")
	}
	c.MoveTo(10, 10)
	c.NewPath()
	c.AppendPath(p)
	if x, y := c.CurrentPoint(); x != 7 || y != 9 {
		t.Fatalf("restore cursor = %v,%v", x, y)
	}
	c.ClosePath()
	if x, y := c.CurrentPoint(); x != 2 || y != 3 {
		t.Fatalf("close cursor = %v,%v", x, y)
	}
	c.Stroke()
	if c.HasCurrentPoint() {
		t.Fatal("Stroke retained current point")
	}
	c.Rectangle(0, 0, 2, 2)
	c.Fill()
	if c.HasCurrentPoint() {
		t.Fatal("Fill retained current point")
	}
}

func TestNestedGroupsAndMaskRestoreDeviceCoordinates(t *testing.T) {
	s := NewImageSurface(FORMAT_ARGB32, 32, 32)
	c := s.NewContext()
	c.Scale(2, 2)
	c.PushGroup()
	if c.Target() != s || c.GroupTarget() == s {
		t.Fatal("group/target ownership")
	}
	c.PushGroup()
	c.SetSourceRGB(1, 0, 0)
	c.Rectangle(2, 2, 4, 4)
	c.Fill()
	c.PopGroupToSource()
	c.Paint()
	mask := c.PopGroup()
	c.SetSourceRGB(0, 1, 0)
	c.Mask(mask)
	if got := s.img.RGBAAt(6, 6); got != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("group mask color/position = %v", got)
	}
	if s.img.RGBAAt(0, 0).A != 0 || s.img.RGBAAt(14, 14).A != 0 {
		t.Fatal("group/mask escaped bounds")
	}
	c.SetSourceRGB(0, 0, 1)
	c.Rectangle(12, 12, 2, 2)
	c.Fill()
	if got := s.img.RGBAAt(25, 25); got.B != 255 || got.G != 0 {
		t.Fatalf("old group source leaked: %v", got)
	}
}

func TestDashPhaseOddPatternAndOwnership(t *testing.T) {
	path := []pathSeg{{op: 'M'}, {op: 'L', x1: 12}}
	got, ok := dashedPath(path, Dash{Dashes: []float64{2}, Offset: -1})
	if !ok || len(got) != 6 {
		t.Fatalf("odd dash: %+v", got)
	}
	for i, x := range []float64{1, 3, 5, 7, 9, 11} {
		if math.Abs(got[i].x1-x) > 1e-8 {
			t.Fatalf("phase: %+v", got)
		}
	}
	for _, d := range [][]float64{{0, 0}, {-1, 2}, {math.NaN(), 2}, {math.Inf(1), 1}} {
		if _, ok := dashedPath(path, Dash{Dashes: d}); ok {
			t.Fatalf("invalid dash accepted: %v", d)
		}
	}
	// Zero entries and tiny periods must not make the renderer spin forever.
	for _, d := range [][]float64{{0, 2}, {1e-300, 1e-300}} {
		dashedPath(path, Dash{Dashes: d})
	}
	c := NewImageSurface(FORMAT_ARGB32, 4, 4).NewContext()
	d := Dash{Dashes: []float64{2, 3}}
	c.SetDash(d)
	d.Dashes[0] = 9
	read := c.Dash()
	read.Dashes[0] = 8
	if c.Dash().Dashes[0] != 2 {
		t.Fatal("caller can mutate retained dash state")
	}
}

func TestScaledStrokeAndEmptyGlyphCallback(t *testing.T) {
	s := NewImageSurface(FORMAT_ARGB32, 40, 40)
	c := s.NewContext()
	c.Scale(4, 4)
	c.SetLineWidth(2)
	c.SetSourceRGB(1, 0, 0)
	c.MoveTo(1, 5)
	c.LineTo(9, 5)
	c.Stroke()
	if s.img.RGBAAt(20, 17).A != 255 || s.img.RGBAAt(20, 14).A != 0 {
		t.Fatal("stroke width not transformed with centerline")
	}
	sf := NewScaledFont(NewToyFontFace("", FONT_SLANT_NORMAL, FONT_WEIGHT_NORMAL), nil, nil, nil)
	called := false
	_ = sf.TextToGlyphs_hack(0, 0, "", func(p unsafe.Pointer, n int) {
		called = true
		if p != nil || n != 0 {
			t.Error("empty glyph callback not empty")
		}
	})
	if !called {
		t.Fatal("empty text cannot clear the caller's reused buffer")
	}
}
