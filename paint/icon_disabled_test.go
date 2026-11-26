package paint

import (
	"image"
	"image/color"
	"testing"
)

func TestDisabledRasterIconKeepsToneAndTint(t *testing.T) {
	const n = 32
	src := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 4; y < n-4; y++ {
		for x := 4; x < n-4; x++ {
			ink := color.RGBA{R: 12, G: 44, B: 80, A: 255}
			if x >= n/2 {
				ink = color.RGBA{R: 225, G: 243, B: 253, A: 255}
			}
			src.SetRGBA(x, y, ink)
		}
	}
	a, err := NewSpriteAtlas(src, 1, 1, []string{"form"})
	if err != nil {
		t.Fatal(err)
	}
	icon, _ := a.Icon("form")
	before := CurrentIconPalette()
	defer SetIconPalette(before)
	var warmMask *subIcon
	var warmTone uint8
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		out := renderIconPixels(t, icon, n, true)
		deep := out.RGBAAt(8, n/2)
		detail := out.RGBAAt(n-8, n/2)
		if deep.A != 255 || deep.R != palette.Disabled.R || deep.G != palette.Disabled.G || deep.B != palette.Disabled.B {
			t.Fatalf("opaque ink changed theme tint: %v (want %v)", deep, palette.Disabled)
		}
		if detail.A == 0 || int(deep.A)-int(detail.A) < 48 {
			t.Fatalf("colored detail was erased: ink=%v detail=%v", deep, detail)
		}
		if out.RGBAAt(0, 0).A != 0 || out.RGBAAt(n-1, n-1).A != 0 {
			t.Fatal("disabled processing painted outside the sprite")
		}
		sub := icon.(*atlasIcon).getSub(n)
		if sub.disabledMask == nil || sub.disabledMask.pat == nil {
			t.Fatal("colored sprite did not retain the resolved mask and pattern")
		}
		if warmMask != nil && (sub.disabledMask != warmMask || detail.A != warmTone) {
			t.Fatal("theme retint rebuilt or changed the raster mask")
		}
		warmMask, warmTone = sub.disabledMask, detail.A
	}
	out := renderIconPixels(t, icon, n, false)
	if got, want := out.RGBAAt(n-8, n/2), src.RGBAAt(n-8, n/2); got != want {
		t.Fatalf("cached disabled paint mutated the enabled raster: got %v want %v", got, want)
	}
}

func TestDisabledSatinSaveRetainsInteriorTone(t *testing.T) {
	icon, err := SatinIcon("save")
	if err != nil {
		t.Fatal(err)
	}
	const n = 64
	active := renderIconPixels(t, icon, n, false)
	muted := renderIconPixels(t, icon, n, true)
	low, high := 255, 0
	opaque := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			a := active.RGBAAt(x, y).A
			if a == 0 && muted.RGBAAt(x, y).A != 0 {
				t.Fatalf("satin mask painted outside original coverage at %v", image.Pt(x, y))
			}
			if a != 255 {
				continue
			}
			opaque++
			tone := int(muted.RGBAAt(x, y).A)
			low, high = min(low, tone), max(high, tone)
		}
	}
	if opaque < 30 || high-low < 48 {
		t.Fatalf("original opaque pixels lost tonal separation: count=%d range=%d..%d", opaque, low, high)
	}
}

func TestDisabledMonochromeUsesOriginalMask(t *testing.T) {
	const n = 16
	src := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			src.Set(x, y, color.NRGBA{R: 40, G: 60, B: 80, A: uint8(17 * x)})
		}
	}
	a, err := NewSpriteAtlas(src, 1, 1, []string{"mono"})
	if err != nil {
		t.Fatal(err)
	}
	i, _ := a.Icon("mono")
	active := renderIconPixels(t, i, n, false)
	disabled := renderIconPixels(t, i, n, true)
	sub := i.(*atlasIcon).getSub(n)
	if !sub.disabledMaskReady || sub.disabledMask != nil {
		t.Fatal("monochrome glyph unexpectedly generated a new mask")
	}
	for y := range n {
		for x := range n {
			if disabled.RGBAAt(x, y).A != active.RGBAAt(x, y).A {
				t.Fatalf("monochrome coverage differs at %d,%d", x, y)
			}
		}
	}
}

type suppliedBitmapIcon struct{ pixmap Pixmap }

func (i suppliedBitmapIcon) IsAir() bool          { return false }
func (i suppliedBitmapIcon) AvailableSize() []int { return []int{i.pixmap.Width()} }
func (i suppliedBitmapIcon) Pixmap(int) Pixmap    { return i.pixmap }

func TestDisabledCustomBitmapKeepsBorrowedSource(t *testing.T) {
	const n = 16
	src := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			if x < n/2 {
				src.SetRGBA(x, y, color.RGBA{A: 255})
			} else {
				src.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	p := NewPixmap(n, n)
	defer CloseSurface(p)
	if err := p.SetImage(src); err != nil {
		t.Fatal(err)
	}
	i := suppliedBitmapIcon{pixmap: p}
	disabled := renderIconPixels(t, i, n, true)
	if left, right := disabled.RGBAAt(3, 8), disabled.RGBAAt(12, 8); left.A != 255 || right.A == 0 || right.A >= left.A {
		t.Fatalf("custom bitmap lost its detail: %v, %v", left, right)
	}
	active := renderIconPixels(t, i, n, false)
	if active.RGBAAt(12, 8) != src.RGBAAt(12, 8) {
		t.Fatal("rendering a disabled custom icon altered the borrowed pixmap")
	}
}

func renderIconPixels(t *testing.T, ico Icon, n int, disabled bool) *image.RGBA {
	t.Helper()
	dst := NewPixmap(n, n)
	defer CloseSurface(dst)
	p := dst.NewPainter()
	defer ClosePainter(p)
	p.DrawIcon(ico, float64(n), disabled)
	img, err := dst.Image()
	if err != nil {
		t.Fatal(err)
	}
	return img.(*image.RGBA)
}
