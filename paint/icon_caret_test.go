package paint

import (
	"testing"
)

func TestCaretIconsAreDistinctThemeAwareVectors(t *testing.T) {
	old := CurrentIconPalette()
	defer SetIconPalette(old)
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		for _, size := range []int{16, 20, 24, 32, 48, 64} {
			fingerprints := map[string]bool{}
			for _, name := range []string{"whitespace-trim", "newline-insert", "edit-cut", "edit-copy", "edit-paste", "edit-select-all", "cursor-add-above", "cursor-add-below", "cursor-primary", "line-move-up", "line-move-down", "line-duplicate", "line-delete"} {
				ico, ok := LoadIcon(name).(*icon)
				if !ok || ico.name != name || ico.draw == nil {
					t.Fatalf("missing vector %s", name)
				}
				surface := NewPixmap(size, size)
				p := surface.NewPainter()
				p.Scale(float64(size)/16, float64(size)/16)
				p.DrawIcon(ico, 16, false)
				ClosePainter(p)
				img, err := surface.Image()
				if err != nil {
					t.Fatal(err)
				}
				signature := make([]byte, 0, size*size*4)
				painted := 0
				for y := 0; y < size; y++ {
					for x := 0; x < size; x++ {
						r, g, b, a := img.At(x, y).RGBA()
						signature = append(signature, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
						if a != 0 {
							painted++
						}
					}
				}
				CloseSurface(surface)
				if painted == 0 || painted == size*size || fingerprints[string(signature)] {
					t.Fatalf("blank/opaque/duplicate icon %s at %d", name, size)
				}
				fingerprints[string(signature)] = true
			}
		}
	}
}
