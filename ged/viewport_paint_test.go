package ged

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func legacyRulerTicks(length, tick, label float64) []rulerTick {
	var out []rulerTick
	for i := 0; float64(i)*tick <= length+1e-9; i++ {
		p := float64(i) * tick
		out = append(out, rulerTick{p, label > 0 && math.Abs(math.Remainder(p, label)) < 1e-9})
	}
	return out
}

func TestVisibleRulerTicksNormalMatchesLegacy(t *testing.T) {
	for _, length := range []float64{0, 34, 100, 512, 9000} {
		for _, lo := range []float64{-100, 0, 10, 53.5, 213.123, 9001} {
			hi := lo + 135
			want := legacyRulerTicks(length, 10, 50)
			got := visibleRulerTicks(length, lo, hi, 10, 50, 2)
			var a, b []rulerTick
			for _, tick := range want {
				if tick.mm >= lo && tick.mm <= hi {
					a = append(a, tick)
				}
			}
			for _, tick := range got {
				if tick.mm >= lo && tick.mm <= hi {
					b = append(b, tick)
				}
			}
			if fmt.Sprint(a) != fmt.Sprint(b) {
				t.Fatalf("length=%g lo=%g got=%v want=%v", length, lo, b, a)
			}
		}
	}
}

func TestVisibleRulerTicksHugeDenseFiniteBudget(t *testing.T) {
	for _, scale := range []float64{1e-12, 1e-8, .01, 1, 100} {
		for _, start := range []float64{0, 1e6, 1e12} {
			got := visibleRulerTicks(1e15, start, start+1000/scale, 10, 50, scale)
			if len(got) == 0 || len(got) > maxGridAxisLines {
				t.Fatalf("scale=%g start=%g marks=%d", scale, start, len(got))
			}
			last := math.Inf(-1)
			for _, tick := range got {
				if !finiteGridNumber(tick.mm) || tick.mm <= last || tick.mm > 1e15+1e-9 {
					t.Fatal("invalid/nonadvancing tick", tick)
				}
				if finiteGridNumber(last) && (tick.mm-last)*scale < minGridDeviceGap*.999 {
					t.Fatal("dense ticks", got)
				}
				last = tick.mm
			}
		}
	}
}

func TestVisibleRulerTicksInvalidArguments(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for i := 0; i < 6; i++ {
			args := []float64{100, 0, 100, 10, 50, 1}
			args[i] = bad
			got := visibleRulerTicks(args[0], args[1], args[2], args[3], args[4], args[5])
			if i == 4 {
				for _, tick := range got {
					if tick.labeled {
						t.Fatal("invalid label interval numbered a tick")
					}
				}
				continue
			}
			if len(got) != 0 {
				t.Fatal("nonfinite ruler argument produced ticks", i, bad)
			}
		}
	}
	if got := visibleRulerTicks(100, 0, 100, 0, 50, 1); len(got) != 0 {
		t.Fatal(got)
	}
}

func legacyDotPaint(g paint.Painter, w, h, step float64) {
	g.SetPen1(paint.Color{0, 0, 0, 28}, 0)
	for x := step; x < w; x += step {
		for y := step; y < h; y += step {
			paintDesignDot(g, x, y)
		}
	}
	g.Stroke()
}

func TestDesignDotsNormalCairoRasterMatchesLegacy(t *testing.T) {
	for _, m := range []geom.Mat3x2{{Xx: 2, Yy: 2, X0: 1.2, Y0: 3.4}, {Xx: 4, Yy: 4, Xy: .2, Yx: .3, X0: 13.25, Y0: -12.5}, {Xx: -3, Yy: 3, X0: 123}} {
		for _, clip := range []geom.Rect{{0, 0, 128, 112}, {17.25, 11.5, 76.5, 65.25}} {
			var images []image.Image
			for i := 0; i < 2; i++ {
				pix := paint.NewPixmap(128, 112)
				g := pix.NewPainter()
				g.Rectangle1(clip)
				g.Clip()
				g.SetMatrix(&m)
				if i == 0 {
					legacyDotPaint(g, 110, 84, 5)
				} else {
					drawDesignDots(g, 110, 84, 5)
				}
				img, err := pix.Image()
				paint.ClosePainter(g)
				paint.CloseSurface(pix)
				if err != nil {
					t.Fatal(err)
				}
				images = append(images, img)
			}
			assertGridImagesEqual(t, images[0], images[1])
		}
	}
}

func TestDesignDotsHugeDenseClipBudget(t *testing.T) {
	for _, scale := range []float64{1e-12, .01, 1, 100} {
		pix := paint.NewPixmap(96, 96)
		base := pix.NewPainter()
		base.Scale(scale, scale)
		base.Translate(-5, -5)
		r := &gridPaintRecorder{Painter: base, limit: 2 * maxDotAxis * maxDotAxis}
		drawDesignDots(r, 1e15, 1e15, 5)
		if len(r.lines) == 0 {
			t.Fatalf("scale=%g no visible dots", scale)
		}
		paint.ClosePainter(base)
		paint.CloseSurface(pix)
	}
}

func rasterDesignView(t *testing.T, size, zoom, scroll float64, dots bool) image.Image {
	t.Helper()
	v := NewGedView()
	defer v.Close()
	v.SetSize(160, 128)
	v.GedScene().SetSize(size, size)
	v.SetZoomFactor(zoom)
	v.SetShowGrid(dots)
	v.Layout()
	v.SetScrollX(scroll)
	v.SetScrollY(scroll)
	before := v.GedScene().SaveDesign().String()
	pix := paint.NewPixmap(160, 128)
	defer paint.CloseSurface(pix)
	base := pix.NewPainter()
	defer paint.ClosePainter(base)
	r := &gridPaintRecorder{Painter: base, limit: 20000}
	v.Draw(r)
	if before != v.GedScene().SaveDesign().String() {
		t.Fatal("view draw mutated design")
	}
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestDesignViewHugePublicDrawMatchesVisibleNormalPage(t *testing.T) {
	for _, zoom := range []float64{.01, .5, 1, 1.25, 2, 100} {
		for _, scroll := range []float64{0, 37.5, 120} {
			for _, dots := range []bool{false, true} {
				t.Run(fmt.Sprintf("zoom-%g/scroll-%g/dots-%v", zoom, scroll, dots), func(t *testing.T) {
					a := rasterDesignView(t, 1e4, zoom, scroll, dots)
					b := rasterDesignView(t, 1e12, zoom, scroll, dots)
					assertGridImagesEqual(t, a, b)
				})
			}
		}
	}
}

func TestDesignViewDeviceCoordinateBudgetAcrossSizes(t *testing.T) {
	// Intermediate sizes are important: a logical-coordinate-only guard can
	// handle 1e12 mm yet silently overflow at 1e5 mm under 100x zoom.
	for _, size := range []float64{1e4, 1e5, 1e6, 1e7, 1e12} {
		for _, zoom := range []float64{1, 10, 100} {
			t.Run(fmt.Sprintf("size-%g/zoom-%g", size, zoom), func(t *testing.T) {
				assertGridImagesEqual(t, rasterDesignView(t, 1e4, zoom, 0, false), rasterDesignView(t, size, zoom, 0, false))
			})
		}
	}
}
