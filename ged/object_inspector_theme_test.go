package ged

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"testing"
)

func TestObjectInspectorDrawUsesThemeAndReusesFonts(t *testing.T) {
	previous := gui.CurrentThemeMode()
	defer gui.SetThemeMode(previous)
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		p := NewObjectInspector()
		p.SetSize(260, 160)
		p.items = []inspectorItem{{name: "hovered", typeName: "Button"}, {name: "selected", typeName: "Form"}}
		p.hoverIdx, p.selectedIdx = 0, 1
		pix := paint.NewPixmap(260, 160)
		r := &widgetListBrushRecorder{Painter: pix.NewPainter()}
		p.Draw(r)
		theme := gui.Theme()
		for _, want := range []paint.Color{theme.ViewBGColor, theme.FormLightColor, theme.ButtonHoverBGColor, theme.TextColor, theme.MenuActiveBGColor, theme.MenuActiveTextColor, theme.MenuGrayTextColor} {
			found := false
			for _, got := range r.brushes {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("mode %v: missing theme color %v", mode, want)
			}
		}
		if aboutContrast(theme.FormLightColor, theme.TextColor) < 4.5 || aboutContrast(theme.ButtonHoverBGColor, theme.TextColor) < 4.5 {
			t.Errorf("mode %v: header/hover text does not meet 4.5:1 contrast", mode)
		}
		regular, bold := p.rowFont, p.boldFont
		p.Draw(r)
		if p.rowFont != regular || p.boldFont != bold {
			t.Fatal("unchanged draw reallocates row fonts")
		}
		for _, size := range [][2]float64{{260, 160}, {10, 8}, {0, 0}} {
			p.SetSize(size[0], size[1])
			b := p.filterBox
			if b.X() < 0 || b.Y() < 0 || b.Width() < 0 || b.Height() < 0 || b.X()+b.Width() > p.Width() || b.Y()+b.Height() > p.Height() {
				t.Fatal("filter escapes a small parent")
			}
		}
	}
}
