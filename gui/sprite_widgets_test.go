package gui

import (
	"math"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestSpriteAndProcessWidgetsRenderWithinBounds(t *testing.T) {
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		old := CurrentThemeMode()
		SetThemeMode(mode)
		gauge := NewGauge()
		gauge.SetTitle("Gauge title")
		for _, w := range []IWidget{NewSpriteView(), NewProcessValue(), gauge} {
			for _, sz := range [][2]float64{{1, 1}, {16, 12}, {96, 96}, {220, 108}} {
				w.SetSize(sz[0], sz[1])
				dst := paint.NewPixmap(240, 120)
				p := dst.NewPainter()
				w.Draw(p)
				paint.ClosePainter(p)
				image, _ := dst.Image()
				for y := 0; y < 120; y++ {
					for x := 0; x < 240; x++ {
						if float64(x) >= sz[0]+1 || float64(y) >= sz[1]+1 {
							_, _, _, a := image.At(x, y).RGBA()
							if a != 0 {
								t.Fatalf("%T painted outside %v at %d,%d", w, sz, x, y)
							}
						}
					}
				}
				paint.CloseSurface(dst)
			}
		}
		SetThemeMode(old)
	}
}

func TestGaugeBackgroundAndFontCacheFollowTheme(t *testing.T) {
	old := CurrentThemeMode()
	defer SetThemeMode(old)
	w := NewGauge()
	w.SetSize(220, 155)
	for _, mode := range []ThemeMode{ThemeDark, ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		dst := paint.NewPixmap(220, 155)
		p := dst.NewPainter()
		w.Draw(p)
		font := w.valueFont
		w.Draw(p)
		if w.valueFont != font {
			t.Fatal("gauge reallocates unchanged fonts")
		}
		paint.ClosePainter(p)
		img, err := dst.Image()
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, a := img.At(1, 1).RGBA()
		want := Theme().ViewBGColor
		if r>>8 != uint32(want.R) || g>>8 != uint32(want.G) || b>>8 != uint32(want.B) || a != 65535 {
			t.Fatalf("gauge background does not follow theme: %v", img.At(1, 1))
		}
		paint.CloseSurface(dst)
	}
}
func TestProcessValueRangeAndFiniteValues(t *testing.T) {
	w := NewProcessValue()
	w.SetValue(90)
	if !w.IsAlarm() || w.Fraction() != 0.9 {
		t.Fatal("threshold/range")
	}
	w.SetValue(math.NaN())
	w.SetValue(math.Inf(1))
	if w.Value() != 90 {
		t.Fatal("accepted invalid value")
	}
	w.SetMin(200)
	if w.Fraction() != 0 {
		t.Fatal("invalid range")
	}
	w.SetPrecision(99)
	if w.Precision() != 6 {
		t.Fatal("precision unbounded")
	}
	w.SetMin(-math.MaxFloat64)
	w.SetMax(math.MaxFloat64)
	w.SetValue(0)
	if w.Fraction() != 0.5 {
		t.Fatal("overflowed finite range")
	}
}

func TestSpriteActiveIconDefaultsToConfiguredSymbol(t *testing.T) {
	w := NewSpriteView()
	w.SetIconName("satin:tank")
	if w.ActiveIconName() != "" || w.selected != nil {
		t.Fatal("default active symbol overrides configured icon")
	}
	w.SetActiveIconName("satin:valve")
	if w.selected == nil {
		t.Fatal("explicit active symbol missing")
	}
	w.SetActiveIconName("")
	if w.selected != nil {
		t.Fatal("clearing active symbol did not restore normal icon")
	}
}

func TestProcessValueCachesPresentation(t *testing.T) {
	w := NewProcessValue()
	w.SetSize(220, 108)
	dst := paint.NewPixmap(220, 108)
	defer paint.CloseSurface(dst)
	p := dst.NewPainter()
	defer paint.ClosePainter(p)
	w.Draw(p)
	font, text := w.valueFont, w.valueText
	w.Draw(p)
	if w.valueFont != font || w.valueText != text {
		t.Fatal("unchanged presentation not reused")
	}
	w.SetValue(42.5)
	w.SetUnit("bar")
	w.SetPrecision(2)
	w.Draw(p)
	if w.valueText != "42.50 bar" {
		t.Fatal("stale presentation", w.valueText)
	}
}
