package paint

import "testing"

func TestTransferProgressVectorIcon(t *testing.T) {
	old := CurrentIconPalette()
	t.Cleanup(func() { SetIconPalette(old) })
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		draw := proceduralFallbacks["transfer-progress"]
		if draw == nil {
			t.Fatal("missing drawer")
		}
		icon := genProceduralIcon("transfer-progress", draw)
		pix := icon.Pixmap(128)
		if iconPixel(t, pix, 0, 0).A != 0 {
			t.Fatal("opaque icon background")
		}
		img, err := pix.Image()
		if err != nil {
			t.Fatal(err)
		}
		colors := map[Color]int{}
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				colors[ColorModel.Convert(img.At(x, y)).(Color)]++
			}
		}
		if colors[palette.Accent] < 3 || colors[palette.Success] < 3 {
			t.Fatal("exact theme colors absent")
		}
	}
}
