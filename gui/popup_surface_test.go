package gui

import (
	"fmt"
	"github.com/uk0/silk/paint"
	"testing"
)

func TestPopupOpaqueSurfaceNeverLeavesUnpaintedCorners(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, scale := range []float64{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%v/%g", mode, scale), func(t *testing.T) {
				m := NewPopupMenu()
				m.SetSize(120, 80)
				// An attached window with no alpha capability models GDI or an opaque
				// GLFW compositor. Offscreen ARGB and lazy popups are tested separately.
				m.win = &Window{widget: m, wt: WtPopup}
				p := paint.NewPixmap(int(120*scale), int(80*scale))
				defer paint.CloseSurface(p)
				g := p.NewPainter()
				defer paint.ClosePainter(g)
				g.Scale(scale, scale)
				m.Draw(g)
				p.Flush()
				img, err := p.Image()
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < img.Bounds().Dy(); y++ {
					for x := 0; x < img.Bounds().Dx(); x++ {
						_, _, _, a := img.At(x, y).RGBA()
						if a != 65535 {
							t.Fatalf("opaque popup left unpainted pixel (%d,%d), alpha=%d", x, y, a)
						}
					}
				}
			})
		}
	}
}

func TestPopupTransparentSurfaceHasCleanRoundedCorners(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		p := paint.NewPixmap(int(120*scale), int(80*scale))
		defer paint.CloseSurface(p)
		g := p.NewPainter()
		defer paint.ClosePainter(g)
		g.Scale(scale, scale)
		m := NewPopupMenu()
		m.SetSize(120, 80)
		m.Draw(g)
		p.Flush()
		img, err := p.Image()
		if err != nil {
			t.Fatal(err)
		}
		for _, pt := range [][2]int{{0, 0}, {img.Bounds().Dx() - 1, 0}, {0, img.Bounds().Dy() - 1}, {img.Bounds().Dx() - 1, img.Bounds().Dy() - 1}} {
			r, g, b, a := img.At(pt[0], pt[1]).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				t.Fatalf("corner not clear: scale=%g point=%v rgba=%v", scale, pt, []uint32{r, g, b, a})
			}
		}
		r, gc, b, a := img.At(int(60*scale), int(40*scale)).RGBA()
		wr, wg, wb, wa := Theme().MenuBGColor.RGBA()
		if r != wr || gc != wg || b != wb || a != wa {
			t.Fatal("menu center changed color")
		}

		// Border is inset rather than losing half its stroke outside the window.
		r, gc, b, a = img.At(int(60*scale), 0).RGBA()
		wr, wg, wb, wa = Theme().MenuBorderColor.RGBA()
		if r != wr || gc != wg || b != wb || a != wa {
			t.Fatalf("top border clipped scale=%g", scale)
		}
	}
}

func BenchmarkPopupSurfaceDraw(b *testing.B) {
	p := paint.NewPixmap(400, 300)
	defer paint.CloseSurface(p)
	g := p.NewPainter()
	defer paint.ClosePainter(g)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drawPopupSurface(g, 400, 300, Theme().MenuBGColor, Theme().MenuBorderColor, 6)
	}
}
