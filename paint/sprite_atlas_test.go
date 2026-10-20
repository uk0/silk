package paint

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSpriteAtlasGridIsolationAndValidation(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 6, 68, 38))
	for y := 6; y < 38; y++ {
		for x := 4; x < 68; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 36 {
				c = color.RGBA{0, 0, 255, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	a, err := NewSpriteAtlas(src, 2, 1, []string{"red", "blue"})
	if err != nil {
		t.Fatal(err)
	}
	src.SetRGBA(4, 6, color.RGBA{})
	for _, size := range []int{16, 20, 24, 32} {
		for n, name := range []string{"red", "blue"} {
			i, _ := a.Icon(name)
			p := i.Pixmap(size)
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					c := iconPixel(t, p, x, y)
					if (n == 0 && (c.R != 255 || c.B != 0)) || (n == 1 && (c.B != 255 || c.R != 0)) {
						t.Fatalf("cell bleed at %s %d %d,%d: %v", name, size, x, y, c)
					}
				}
			}
			CloseSurface(p)
		}
	}
	for _, names := range [][]string{{"same", "same"}, {"", "b"}, {"one"}} {
		if _, err := NewSpriteAtlas(src, 2, 1, names); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	if _, ok := a.Icon("unknown"); ok {
		t.Fatal("unknown icon found")
	}
	if _, err := NewSpriteAtlas(src, 3, 1, []string{"a", "b", "c"}); err == nil {
		t.Fatal("non-divisible atlas accepted")
	}
}
func TestSatinIconsDeviceResolutionCacheAndAlpha(t *testing.T) {
	for _, name := range satinNames {
		ico, err := SatinIcon(name)
		if err != nil {
			t.Fatal(err)
		}
		i := ico.(*atlasIcon)
		opaque, transparent := false, false
		for _, size := range []int{16, 20, 24, 32, 48, 64} {
			dst := NewPixmap(size, size)
			p := dst.NewPainter()
			p.Scale(float64(size)/16, float64(size)/16)
			p.DrawIcon(i, 16, false)
			ClosePainter(p)
			sub := i.getSub(size)
			if i.getSub(size) != sub {
				t.Fatal("warm repaint missed cache")
			}
			got, _ := dst.Image()
			want, _ := sub.img.Image()
			assertIconImagesEqual(t, size, got, want)
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					_, _, _, a := got.At(x, y).RGBA()
					opaque = opaque || a > 60000
					transparent = transparent || a == 0
				}
			}
			CloseSurface(dst)
		}
		if !opaque || !transparent {
			t.Fatalf("%s lacks real transparent/opaque pixels", name)
		}
		for size := 65; size < 65+spriteCacheLimit+3; size++ {
			i.getSub(size)
		}
		if len(i.cache) > spriteCacheLimit {
			t.Fatal("unbounded sprite cache")
		}
		if i.getSub(4096).size != i.rect.Dx() {
			t.Fatal("upscale allocated above native resolution")
		}
	}
}
func TestDisabledSpriteMasksColorAndKeepsPath(t *testing.T) {
	old := CurrentIconPalette()
	defer SetIconPalette(old)
	palette := DarkIconPalette()
	palette.Disabled = Color{R: 100, G: 100, B: 100, A: 255}
	SetIconPalette(palette)
	i, _ := SatinIcon("play")
	dst := NewPixmap(64, 64)
	defer CloseSurface(dst)
	p := dst.NewPainter()
	defer ClosePainter(p)
	p.MoveTo(1, 2)
	p.LineTo(3, 4)
	p.DrawIcon(i, 64, true)
	if x, y := p.CurrentPoint(); x != 3 || y != 4 {
		t.Fatal("path changed")
	}
	img, _ := dst.Image()
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && (math.Abs(float64(r)-float64(g)) > 1 || math.Abs(float64(g)-float64(b)) > 1) {
				t.Fatal("disabled sprite retained saturation")
			}
		}
	}
}
func TestSatinPreferenceUpdatesExistingIcon(t *testing.T) {
	old := preferSatinIcons
	defer SetPreferSatinIcons(old)
	i := genProceduralIcon("save", drawSaveDisk)
	SetPreferSatinIcons(true)
	got := i.Pixmap(32)
	want, _ := SatinIcon("save")
	wp := want.Pixmap(32)
	defer CloseSurface(got)
	defer CloseSurface(wp)
	a, _ := got.Image()
	b, _ := wp.Image()
	assertIconImagesEqual(t, 32, a, b)
}
func TestRenderSatinAtlasContactSheet(t *testing.T) {
	dir := os.Getenv("SILK_SPRITE_QA_DIR")
	if dir == "" {
		t.Skip("optional visual artifact")
	}
	os.MkdirAll(dir, 0755)
	dst := NewPixmap(1024, 600)
	defer CloseSurface(dst)
	p := dst.NewPainter()
	defer ClosePainter(p)
	for row, bg := range []Color{{R: 28, G: 31, B: 38, A: 255}, {R: 242, G: 244, B: 248, A: 255}} {
		p.SetBrush1(bg)
		p.Rectangle(0, float64(row*300), 1024, 300)
		p.Fill()
		for n, name := range satinNames {
			i, _ := SatinIcon(name)
			for k, size := range []float64{16, 24, 32, 48, 64} {
				p.DrawIcon1(i, float64(n*64), float64(row*300+k*56), size, false)
			}
		}
	}
	if err := dst.WritePNG(filepath.Join(dir, "satin-contact-sheet.png")); err != nil {
		t.Fatal(err)
	}
}
func BenchmarkWarmSatinIcon(b *testing.B) {
	i, _ := SatinIcon("save")
	p := NewPixmap(32, 32)
	defer CloseSurface(p)
	g := p.NewPainter()
	defer ClosePainter(g)
	g.DrawIcon(i, 32, false)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		g.DrawIcon(i, 32, false)
	}
}
