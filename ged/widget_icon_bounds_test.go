package ged

import (
	"fmt"
	"github.com/uk0/silk/paint"
	"testing"
)

func TestPaletteIconsStayInsideSlot(t *testing.T) {
	names := []string{"gui.Rating", "unknown.Plugin"}
	for _, cat := range categoryDefs {
		names = append(names, cat.names...)
	}
	for _, size := range []int{12, 18, 24, 36} {
		for _, name := range names {
			t.Run(fmt.Sprintf("%s/%d", name, size), func(t *testing.T) {
				pix := paint.NewPixmap(80, 80)
				defer paint.CloseSurface(pix)
				g := pix.NewPainter()
				defer paint.ClosePainter(g)
				state := g.CurrentState()
				drawWidgetIcon(g, name, 10, 10, float64(size))
				if g.CurrentState() != state {
					t.Fatal("icon leaked painter state")
				}
				img, err := pix.Image()
				if err != nil {
					t.Fatal(err)
				}
				ink := 0
				for y := 0; y < 80; y++ {
					for x := 0; x < 80; x++ {
						_, _, _, a := img.At(x, y).RGBA()
						if a == 0 {
							continue
						}
						ink++
						if x < 10 || x >= 10+size || y < 10 || y >= 10+size {
							t.Fatalf("icon escapes slot at (%d,%d)", x, y)
						}
					}
				}
				if ink == 0 {
					t.Fatal("empty icon")
				}
			})
		}
	}
}
