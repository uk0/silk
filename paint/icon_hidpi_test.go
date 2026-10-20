package paint

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/geom"
)

func checkerIconPixmap(size int) *cairoSurface {
	pix := NewPixmap(size, size)
	p := pix.NewPainter()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := Color{R: 255, A: 255}
			if (x+y)%2 == 0 {
				c = Color{G: 255, A: 255}
			}
			p.SetBrush(c)
			p.Rectangle(float64(x), float64(y), 1, 1)
			p.Fill()
		}
	}
	ClosePainter(p)
	return pix
}

func TestRasterIconsSelectActualDeviceResolution(t *testing.T) {
	for _, size := range []int{16, 20, 24, 32, 48} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			low, high := checkerIconPixmap(16), checkerIconPixmap(size)
			defer CloseSurface(low)
			defer CloseSurface(high)
			ico := &icon{subs: []*subIcon{{size: 16, img: low}, {size: size, img: high}}}
			dst := NewPixmap(size, size)
			defer CloseSurface(dst)
			p := dst.NewPainter()
			defer ClosePainter(p)
			p.Scale(float64(size)/16, float64(size)/16)
			p.DrawIcon(ico, 16, false)
			got, _ := dst.Image()
			want, _ := high.Image()
			assertIconImagesEqual(t, size, got, want)
		})
	}
}

type resolutionProbeIcon struct {
	requested int
	pix       Pixmap
}

func (i *resolutionProbeIcon) AvailableSize() []int { return nil }
func (i *resolutionProbeIcon) IsAir() bool          { return false }
func (i *resolutionProbeIcon) Pixmap(size int) Pixmap {
	i.requested = size
	i.pix = checkerIconPixmap(size)
	return i.pix
}

func TestGenericIconReceivesDeviceSizeWithoutChangingLogicalBounds(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2, 3} {
		dst := NewPixmap(64, 64)
		p := dst.NewPainter()
		p.Scale(scale, scale)
		p.MoveTo(2, 3)
		p.LineTo(7, 9)
		ico := &resolutionProbeIcon{}
		p.DrawIcon(ico, 15.5, false)
		if want := int(math.Ceil(15.5 * scale)); ico.requested != want {
			t.Errorf("scale %g request=%d want=%d", scale, ico.requested, want)
		}
		if x, y := p.CurrentPoint(); x != 7 || y != 9 {
			t.Fatal("icon corrupted path")
		}
		if got := iconPixel(t, dst, int(math.Ceil(15.5*scale))+1, 2); got.A != 0 {
			t.Fatal("icon exceeded logical size")
		}
		ClosePainter(p)
		CloseSurface(dst)
		CloseSurface(ico.pix)
	}
}

func TestIconSizeRejectsInvalidInputWithoutPoisoningPainter(t *testing.T) {
	dst := NewPixmap(32, 32)
	defer CloseSurface(dst)
	p := dst.NewPainter()
	defer ClosePainter(p)
	ico := &resolutionProbeIcon{}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		p.DrawIcon(ico, size, false)
	}
	if ico.requested != 0 {
		t.Fatal("invalid size reached icon")
	}
	var m geom.Mat3x2
	p.GetMatrix(&m)
	if m.Xx != 1 || m.Yy != 1 {
		t.Fatal("invalid icon poisoned CTM")
	}
}

func TestIconDeviceSizeTransformsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    geom.Mat3x2
		want int
	}{
		{"identity", geom.Mat3x2{Xx: 1, Yy: 1}, 16},
		{"rotation", geom.Mat3x2{Xx: math.Sqrt(.5), Xy: -math.Sqrt(.5), Yx: math.Sqrt(.5), Yy: math.Sqrt(.5)}, 16},
		{"anisotropic", geom.Mat3x2{Xx: 1, Yy: 2}, 32},
		{"reflected", geom.Mat3x2{Xx: -1.5, Yy: 1.5}, 24},
		{"shear", geom.Mat3x2{Xx: 1, Xy: 2, Yy: 1}, 39},
		{"allocation bound", geom.Mat3x2{Xx: 1e8, Yy: 1e8}, 4096},
		{"invalid", geom.Mat3x2{Xx: math.Inf(1), Yy: 1}, 0},
	} {
		if got := iconDeviceSize(16, tc.m); got != tc.want {
			t.Errorf("%s size=%d want=%d", tc.name, got, tc.want)
		}
	}
}
