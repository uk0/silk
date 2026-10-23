package paint

import "testing"

func TestWorkflowIconsHaveThemeColorsAndTransparentCorners(t *testing.T) {
	old := CurrentIconPalette()
	t.Cleanup(func() { SetIconPalette(old) })
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		for _, name := range []string{"printer", "gantt", "http-request"} {
			draw := proceduralFallbacks[name]
			if draw == nil {
				t.Fatalf("%s missing vector fallback", name)
			}
			icon := genProceduralIcon(name, draw)
			pix := icon.Pixmap(128)
			if got := iconPixel(t, pix, 0, 0); got.A != 0 {
				t.Fatalf("%s corner not transparent: %v", name, got)
			}
			img, err := pix.Image()
			if err != nil {
				t.Fatal(err)
			}
			counts := map[Color]int{}
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					counts[ColorModel.Convert(img.At(x, y)).(Color)]++
				}
			}
			for _, color := range []Color{palette.Accent, palette.Success} {
				if counts[color] < 3 {
					t.Fatalf("%s missing exact theme color %v", name, color)
				}
			}
		}
	}
}
