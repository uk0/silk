package purecairo

import (
	"image"
	"image/color"
	"testing"
)

// vector.Rasterizer.Draw maps mask (0,0) to the destination rectangle's Min,
// not to surface (0,0). A nonzero clip must not translate the painted geometry.
func TestClipPreservesDeviceGeometry(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		for _, shape := range []string{"rectangle", "stroke", "curve"} {
			for _, source := range []string{"solid", "gradient", "surface"} {
				render := func(clipped bool) *image.RGBA {
					s := NewImageSurface(FORMAT_ARGB32, 160, 100)
					c := s.NewContext()
					c.Scale(scale, scale)
					if clipped {
						c.Rectangle(20, 10, 48, 30)
						c.Clip()
					}
					c.SetSourceRGBA(.3, .6, .9, .7)
					if source == "gradient" {
						p := NewLinearPattern(0, 0, 160, 0)
						p.AddColorStopRGBA(0, 1, 0, 0, 1)
						p.AddColorStopRGBA(1, 0, 0, 1, 1)
						c.SetSource(p)
					} else if source == "surface" {
						p := NewImageSurface(FORMAT_ARGB32, 160, 100)
						for y := 0; y < 100; y++ {
							for x := 0; x < 160; x++ {
								p.img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 70, A: 255})
							}
						}
						c.SetSourceSurface(p, 3, 4)
					}
					if shape == "stroke" {
						c.SetLineWidth(2)
						c.MoveTo(25, 15)
						c.LineTo(55, 15)
						c.LineTo(55, 25)
						c.Stroke()
					} else if shape == "curve" {
						c.MoveTo(25, 15)
						c.CurveTo(35, 30, 50, 5, 60, 25)
						c.ClosePath()
						c.Fill()
					} else {
						c.Rectangle(25, 15, 35, 20)
						c.Fill()
					}
					return s.img
				}
				a, b := render(false), render(true)
				clip := image.Rect(int(20*scale), int(10*scale), int(68*scale), int(40*scale)).Intersect(a.Bounds())
				for y := 0; y < a.Bounds().Dy(); y++ {
					for x := 0; x < a.Bounds().Dx(); x++ {
						if image.Pt(x, y).In(clip) {
							if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
								t.Fatalf("clip translated geometry: scale=%v shape=%v source=%v at %d,%d", scale, shape, source, x, y)
							}
						} else if b.RGBAAt(x, y).A != 0 {
							t.Fatalf("drawing escaped clip at %d,%d", x, y)
						}
					}
				}
			}
		}
	}
}
