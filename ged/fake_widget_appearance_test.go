package ged

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func TestFakeWidgetAppearanceInvalidatesPreviewCache(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func()
	}{
		{"theme switch", func() { gui.SetThemeMode(gui.ThemeDark) }},
		{"icon palette", func() {
			p := paint.CurrentIconPalette()
			p.Disabled = paint.Color{R: 227, G: 18, B: 130, A: 255}
			paint.SetIconPalette(p)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			theme, palette := gui.CurrentThemeMode(), paint.CurrentIconPalette()
			t.Cleanup(func() {
				gui.SetThemeMode(theme)
				paint.SetIconPalette(palette)
			})
			gui.SetThemeMode(gui.ThemeLight)

			fake, err := NewFakeWidgetFromFactory("gui.Button")
			if err != nil {
				t.Fatal(err)
			}
			ico, err := paint.SatinIcon("save")
			if err != nil {
				t.Fatal(err)
			}
			button := fake.Widget().(*gui.Button)
			button.SetIcon(ico)
			button.SetText("Save")
			button.SetTextVisible(true)
			button.SetEnabled(false)
			fake.SetBounds(10, 10, 40, 16)

			target := paint.NewPixmap(160, 96)
			defer paint.CloseSurface(target)
			p := target.NewPainter()
			defer paint.ClosePainter(p)
			fake.DrawSelf(p)
			before := fake.cachedPixmap
			if before == nil {
				t.Fatal("fixture did not populate the preview cache")
			}
			oldImage := encodedPreview(t, before)
			for range 10 {
				fake.DrawSelf(p)
				if fake.cachedPixmap != before {
					t.Fatal("unchanged draw replaced the preview")
				}
			}

			tc.change()
			fake.DrawSelf(p)
			after := fake.cachedPixmap
			firstDraw := encodedPreview(t, after)
			// Build an uncached frame as the pixel oracle. Theme tests must
			// prove the changed appearance would actually render differently.
			fake.MarkDirty()
			fake.DrawSelf(p)
			want := encodedPreview(t, fake.cachedPixmap)
			if bytes.Equal(oldImage, want) {
				t.Fatal("fixture did not change appearance")
			}
			if after == before || !bytes.Equal(firstDraw, want) {
				t.Fatal("one draw after an appearance change used a stale pixmap")
			}
			// An already-recorded frame can still reference the old surface.
			if !bytes.Equal(encodedPreview(t, before), oldImage) {
				t.Fatal("refresh mutated the old frame's pixmap")
			}
		})
	}
}

func encodedPreview(t *testing.T, pm paint.Pixmap) []byte {
	t.Helper()
	img, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
