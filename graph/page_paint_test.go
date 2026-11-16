package graph

import (
	"math"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestViewPageHugeHasNoArtificialClipBorder(t *testing.T) {
	for _, offset := range []float64{0, -40.5} {
		var samples [2][][4]uint32
		for i, size := range []float64{10000, 1e12} {
			pix := paint.NewPixmap(64, 64)
			g := pix.NewPainter()
			g.Translate(offset, offset)
			drawViewPage(g, size, size)
			img, err := pix.Image()
			paint.ClosePainter(g)
			paint.CloseSurface(pix)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 64; y++ {
				for x := 0; x < 64; x++ {
					r, g, b, a := img.At(x, y).RGBA()
					samples[i] = append(samples[i], [4]uint32{r, g, b, a})
				}
			}
		}
		for i, want := range samples[0] {
			if samples[1][i] != want {
				t.Fatalf("offset=%g pixel=%d huge page changed visible fill/border", offset, i)
			}
		}
	}
}

func TestViewPageInvalidAndNonfiniteZoom(t *testing.T) {
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		pix := paint.NewPixmap(8, 8)
		g := pix.NewPainter()
		drawViewPage(g, bad, 100)
		drawViewPage(g, 100, bad)
		img, err := pix.Image()
		paint.ClosePainter(g)
		paint.CloseSurface(pix)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a != 0 {
					t.Fatal("invalid page painted", bad)
				}
			}
		}
	}
	for _, c := range []struct{ in, want float64 }{{math.NaN(), 1}, {math.Inf(1), 100}, {math.Inf(-1), .01}, {1.25, 1.25}} {
		if got := limitZoomFactor(c.in); got != c.want {
			t.Fatalf("zoom %g => %g want %g", c.in, got, c.want)
		}
	}
}
