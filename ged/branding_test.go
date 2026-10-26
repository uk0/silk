package ged

import (
	"slices"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type brandTestIcon struct{ p paint.Pixmap }

func (i brandTestIcon) AvailableSize() []int    { return []int{64} }
func (i brandTestIcon) IsAir() bool             { return false }
func (i brandTestIcon) Pixmap(int) paint.Pixmap { return i.p }

type brandTextRecorder struct {
	paint.Painter
	text []string
}

func (r *brandTextRecorder) DrawText1(x, y float64, text string) {
	r.text = append(r.text, text)
	r.Painter.DrawText1(x, y, text)
}

func TestHostBrandingDrawsLogoWithoutOverlappingAboutText(t *testing.T) {
	mode := gui.CurrentThemeMode()
	t.Cleanup(func() { gui.SetThemeMode(mode) })
	logo := paint.NewPixmap(64, 64)
	lp := logo.NewPainter()
	want := paint.Color{R: 20, G: 180, B: 210, A: 255}
	lp.SetBrush1(want)
	lp.Rectangle(0, 0, 64, 64)
	lp.Fill()
	info := AboutInfo{Name: "Silk IDE", Version: "0.1.3", Logo: brandTestIcon{logo}}
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		d := newAboutDialog(nil, info)
		if d.Icon() != info.Logo {
			t.Fatal("about native icon does not match host")
		}
		body := d.Content().(*aboutContent)
		body.SetSize(480, 420)
		body.Layout()
		lines, _ := body.textLayout(476)
		if len(lines) == 0 || lines[0].x != 64 || lines[0].width != 412 {
			t.Fatal("about title overlaps logo")
		}
		p := paint.NewPixmap(480, 420)
		g := p.NewPainter()
		body.Draw(g)
		img, err := p.Image()
		if err != nil {
			t.Fatal(err)
		}
		if paint.ColorModel.Convert(img.At(20, 20)).(paint.Color) != want {
			t.Fatal("about did not render actual logo pixels")
		}
		w := NewWelcomeScreen()
		w.SetBranding(info)
		w.SetSize(900, 600)
		p = paint.NewPixmap(900, 600)
		w.Draw(p.NewPainter())
		img, err = p.Image()
		if err != nil {
			t.Fatal(err)
		}
		if paint.ColorModel.Convert(img.At(110, 90)).(paint.Color) != want {
			t.Fatal("welcome did not render actual logo pixels")
		}
	}
	if NewWelcomeScreen().branding.Logo != nil || newAboutDialog(nil, AboutInfo{}).Icon() != nil {
		t.Fatal("SDK default unexpectedly adopted IDE branding")
	}
	standalone := NewWelcomeScreen()
	standalone.SetSize(900, 600)
	rec := &brandTextRecorder{Painter: paint.NewPixmap(900, 600).NewPainter()}
	standalone.Draw(rec)
	for _, text := range []string{"Silk Designer", "A visual designer for Go interfaces.", "Silk UI Framework  \u00b7  " + aboutAppVersion + "  \u00b7  " + aboutCopyright} {
		if !slices.Contains(rec.text, text) {
			t.Fatalf("SDK default text changed: %q", text)
		}
	}
}
