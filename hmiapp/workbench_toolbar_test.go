package hmiapp

import (
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestWorkbenchToolbarIconsAndCaptionsFit(t *testing.T) {
	locale := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(locale) })
	icons := []string{"run", "stop", "save", "folder-open", "go-file", "fit-content", "log", "tree-view", "notifications"}
	for _, variant := range []struct {
		locale string
		labels []string
	}{
		{"en", []string{"Run", "Stop", "Save", "Load", "Generate Go", "Fit", "Snapshot", "Live points", "Live alarms"}},
		{"zh", []string{"运行", "停止", "保存", "加载", "生成 Go", "适应画布", "快照", "实时点位", "实时报警"}},
	} {
		t.Run(variant.locale, func(t *testing.T) {
			i18n.SetLocale(variant.locale)
			f, err := New(Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer f.Join()
			defer f.Close()
			if len(f.buttons) != len(icons) {
				t.Fatalf("got %d toolbar commands", len(f.buttons))
			}
			missing := gui.LoadIcon("image-missing")
			for i, b := range f.buttons {
				if b.Icon() == missing || b.Icon() != gui.LoadIcon(icons[i]) || !b.IsTextVisible() || b.Text() != variant.labels[i] {
					t.Fatalf("command %d: glyph or %q caption missing", i, variant.labels[i])
				}
			}
			for _, width := range []float64{430, 640, 1040} {
				f.SetSize(width, 560)
				f.OnResize()
				for i, b := range f.buttons {
					x, y, w, h := b.Bounds()
					hint := b.SizeHints()
					if x < 8 || x+w > width-8 || y+h > f.Editor.Y() || w < hint.Width || h < hint.Height {
						t.Fatalf("command %d does not fit at width %.0f", i, width)
					}
					for _, earlier := range f.buttons[:i] {
						ex, ey, ew, eh := earlier.Bounds()
						if x < ex+ew && ex < x+w && y < ey+eh && ey < y+h {
							t.Fatalf("command %d overlaps another at width %.0f", i, width)
						}
					}
				}
			}
			if !f.buttons[0].IsEnabled() || f.buttons[1].IsEnabled() {
				t.Fatal("idle Run/Stop state changed")
			}
		})
	}
}
