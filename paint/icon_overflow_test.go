package paint

import "testing"

func TestToolbarOverflowIconUsesHiDPIProceduralGlyph(t *testing.T) {
	ic, ok := LoadIcon("toolbar-overflow").(*icon)
	if !ok || ic.name != "toolbar-overflow" || ic.draw == nil {
		t.Fatal("overflow routed to a missing glyph")
	}
	for _, size := range []int{16, 20, 24, 32, 40, 64} {
		pix := ic.Pixmap(size)
		img, err := pix.Image()
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if b.Dx() != size || b.Dy() != size {
			t.Fatal("wrong physical icon size", size, b)
		}
		left, right := false, false
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a > 0 {
					if x < size/2 {
						left = true
					} else {
						right = true
					}
				}
			}
		}
		if !left || !right {
			t.Fatal("incomplete overflow chevrons", size)
		}
	}
}
