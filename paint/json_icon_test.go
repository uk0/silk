package paint

import "testing"

func TestJSONViewVectorIcon(t *testing.T) {
	old := CurrentIconPalette()
	t.Cleanup(func() { SetIconPalette(old) })
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		draw := proceduralFallbacks["json-view"]
		if draw == nil {
			t.Fatal("missing JSON icon")
		}
		icon := genProceduralIcon("json-view", draw)
		pix := icon.Pixmap(128)
		if iconPixel(t, pix, 0, 0).A != 0 {
			t.Fatal("icon background not transparent")
		}
		img, err := pix.Image()
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				if ColorModel.Convert(img.At(x, y)).(Color) == palette.Violet {
					count++
				}
			}
		}
		if count < 3 {
			t.Fatal("theme vector color missing")
		}
	}
}
