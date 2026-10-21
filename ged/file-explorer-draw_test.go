package ged

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type explorerDrawText struct {
	text                        string
	x, y, cx, cy, cw, ch, width float64
}
type explorerDrawRecorder struct {
	paint.Painter
	texts  []explorerDrawText
	colors []paint.Color
}

func (r *explorerDrawRecorder) SetBrush1(c paint.Color) {
	r.colors = append(r.colors, c)
	r.Painter.SetBrush1(c)
}
func (r *explorerDrawRecorder) DrawText1(x, y float64, s string) {
	cx, cy, cw, ch := r.ClipBounds()
	r.texts = append(r.texts, explorerDrawText{s, x, y, cx, cy, cw, ch, explorerTextWidth(r.Font(), s)})
	r.Painter.DrawText1(x, y, s)
}
func TestExplorerRealPainterThemeClipAndFont(t *testing.T) {
	oldMode, oldFont := gui.CurrentThemeMode(), gui.Theme().Font
	t.Cleanup(func() { gui.SetThemeMode(oldMode); gui.Theme().Font = oldFont })
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		e := NewFileExplorer()
		e.SetSize(200, 160)
		e.rootDir = "project"
		e.root = &fileEntry{name: "project", isDir: true}
		for i := 0; i < 5000; i++ {
			e.flatList = append(e.flatList, &fileEntry{name: fmt.Sprintf("%04d-%s.go", i, strings.Repeat("很长的名称", 5)), path: fmt.Sprint(i), depth: 1})
		}
		e.selectedIdx = 1
		e.hoverIdx = 2
		e.gitStatus = map[string]rune{"1": 'M'}
		for _, size := range []int{14, 24} {
			gui.Theme().Font = paint.NewFont(oldFont.Family(), size, false, false)
			e.ensureFonts()
			e.scrollY = e.rowHeight / 2
			pix := paint.NewPixmap(200, 160)
			r := &explorerDrawRecorder{Painter: pix.NewPainter()}
			before := r.CurrentState()
			e.Draw(r)
			if r.CurrentState() != before {
				t.Fatal("unbalanced painter state")
			}
			if e.normalFont.Size() != size || e.rowHeight < e.normalFont.FontExtents().Height+8 {
				t.Fatal("font preference ignored")
			}
			if len(r.texts) > 12 {
				t.Fatalf("offscreen rows drawn: %d", len(r.texts))
			}
			for i, text := range r.texts {
				if text.x+text.width > 200.1 {
					t.Fatalf("text overflow: %+v", text)
				}
				if i > 0 && math.Abs(text.cy-e.headerHeight()) > .01 {
					t.Fatalf("row can cover fixed header: %+v", text)
				}
				if strings.HasPrefix(text.text, "0001-") && text.x+text.width > 172.1 {
					t.Fatalf("filename covers badge: %+v", text)
				}
			}
			for _, want := range []paint.Color{gui.Theme().FormColor, gui.Theme().ViewBGColor, gui.Theme().TextColor, gui.Theme().HighLightColor, gui.Theme().MenuActiveTextColor, gui.Theme().ButtonHoverBGColor} {
				found := false
				for _, got := range r.colors {
					found = found || got == want
				}
				if !found {
					t.Fatalf("theme %d ignored %v", mode, want)
				}
			}
		}
	}
}
