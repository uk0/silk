package paint

import (
	"image/color"
	"testing"
)

func TestTransformMoveIconVisibleXYZAcrossSizes(t *testing.T) {
	icon := LoadIcon("transform-move")
	if icon.IsAir() || icon == LoadIcon("image-missing") {
		t.Fatal("missing transform icon")
	}
	for _, size := range []int{16, 22, 32, 48, 96} {
		pm := icon.Pixmap(size)
		im, err := pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		var colors [3]int
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				p := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
				if p.A < 160 {
					continue
				}
				if int(p.R) > int(p.G)+40 && int(p.R) > int(p.B)+40 {
					colors[0]++
				}
				if int(p.G) > int(p.R)+40 && int(p.G) > int(p.B)+30 {
					colors[1]++
				}
				if int(p.B) > int(p.R)+40 && int(p.B) > int(p.G)+40 {
					colors[2]++
				}
			}
		}
		for _, count := range colors {
			if count == 0 {
				t.Fatal("missing distinguishable axis color", size, colors)
			}
		}
	}
}
