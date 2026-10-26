package ged

import (
	"fmt"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWelcomeRecentFilesOwnsInput(t *testing.T) {
	w := NewWelcomeScreen()
	files := []string{"/tmp/first.go"}
	w.SetRecentFiles(files)
	files[0] = "/tmp/replaced.go"
	if w.recentFiles[0] != "/tmp/first.go" {
		t.Fatal("caller mutation changed recent-file action target")
	}
}
func TestWelcomeNarrowActionsDoNotCoverRecentFiles(t *testing.T) {
	w := NewWelcomeScreen()
	w.SetRecentFiles([]string{"/tmp/a.go", "/tmp/b.go", "/tmp/c.go", "/tmp/d.go", "/tmp/e.go"})
	w.SetSize(400, 600)
	p := paint.NewPixmap(400, 600)
	g := p.NewPainter()
	defer paint.ClosePainter(g)
	defer paint.CloseSurface(p)
	w.Draw(g)
	for _, b := range [][4]float64{w.btnNewRect, w.btnOpenRect} {
		for _, r := range w.recentRects {
			if b[0] < r[0]+r[2] && r[0] < b[0]+b[2] && b[1] < r[1]+r[3] && r[1] < b[1]+b[3] {
				t.Fatalf("button %v overlaps recent row %v", b, r)
			}
		}
	}
}
func TestWelcomeEmptyHitRegionsAreNotClickable(t *testing.T) {
	if hitRect(0, 0, [4]float64{}) {
		t.Fatal("empty rectangle accepts click")
	}
}

func welcomeTest(t *testing.T) *WelcomeScreen {
	t.Helper()
	aboutTestEnvironment(t)
	registerWelcomeTranslations()
	w := NewWelcomeScreen()
	w.SetNewProjectCallback(func() {})
	w.SetOpenFileCallback(func() {})
	w.SetOpenRecentCallback(func(string) {})
	return w
}
func TestWelcomeResponsiveGeometryAndFontScale(t *testing.T) {
	w := welcomeTest(t)
	w.SetRecentFiles([]string{"/tmp/项目/窗口🙂.silkui", "/tmp/main.go"})
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		for _, locale := range []string{"en", "zh-CN"} {
			i18n.SetLocale(locale)
			for _, font := range []int{14, 20, 28} {
				gui.Theme().Font = paint.NewFont(gui.Theme().Font.Family(), font, false, false)
				for _, size := range [][2]float64{{0, 0}, {8, 20}, {200, 220}, {400, 600}, {720, 300}, {900, 600}, {1400, 900}} {
					t.Run(fmt.Sprint(mode, locale, font, size), func(t *testing.T) {
						w.SetSize(size[0], size[1])
						w.Layout()
						if len(w.recentRects) == 0 {
							t.Fatal("missing actual list geometry")
						}
						for _, r := range [][4]float64{w.btnNewRect, w.btnOpenRect, w.recentRects[0]} {
							for _, v := range r {
								if math.IsNaN(v) || math.IsInf(v, 0) {
									t.Fatalf("invalid geometry %v", r)
								}
							}
							if r[2] < 0 || r[3] < 0 {
								t.Fatalf("negative extent %v", r)
							}
						}
						overlap := func(a, b [4]float64) bool {
							return a[2] > 0 && a[3] > 0 && b[2] > 0 && b[3] > 0 && a[0] < b[0]+b[2] && b[0] < a[0]+a[2] && a[1] < b[1]+b[3] && b[1] < a[1]+a[3]
						}
						if overlap(w.btnNewRect, w.btnOpenRect) || overlap(w.btnNewRect, w.recentRects[0]) || overlap(w.btnOpenRect, w.recentRects[0]) {
							t.Fatalf("controls overlap: %v %v %v", w.btnNewRect, w.btnOpenRect, w.recentRects)
						}
						if size[0] >= 400 && (w.btnNewRect[2] < w.newButton.SizeHints().Width || w.btnNewRect[3] < gui.Theme().ItemHeight) {
							t.Fatalf("button too small: %v", w.btnNewRect)
						}
					})
				}
			}
		}
	}
}
func TestWelcomeRealControlsAndRecentActivation(t *testing.T) {
	w := welcomeTest(t)
	w.SetSize(900, 600)
	newCount, openCount := 0, 0
	w.SetNewProjectCallback(func() { newCount++ })
	w.SetOpenFileCallback(func() { openCount++ })
	w.newButton.OnKeyDown(gui.KeyEnter, false)
	w.openButton.OnKeyDown(gui.KeySpace, false)
	if newCount != 1 || openCount != 1 {
		t.Fatalf("keyboard buttons did not activate: %d %d", newCount, openCount)
	}
	for _, b := range []*welcomeButton{w.newButton, w.openButton} {
		if !b.IsTextVisible() || b.Parent() != w || b.Icon() == nil || b.Icon().IsAir() {
			t.Fatal("button missing parent or icon")
		}
	}
	w.SetNewProjectCallback(nil)
	w.newButton.OnKeyDown(gui.KeyEnter, false)
	if newCount != 1 || w.newButton.IsEnabled() {
		t.Fatal("missing callback still active")
	}
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("/tmp/项目/file-%02d.go", i)
	}
	w.SetRecentFiles(files)
	var opened []string
	w.SetOpenRecentCallback(func(s string) { opened = append(opened, s) })
	l := w.recent
	l.OnKeyDown(gui.KeyEnd, false)
	if l.ActiveIndex() != 49 || l.ScrollY() <= 0 {
		t.Fatal("last recent file unreachable")
	}
	l.OnKeyDown(gui.KeyEnter, false)
	l.OnKeyDown(gui.KeyEnter, true)
	if !slices.Equal(opened, []string{files[49]}) {
		t.Fatalf("activation or repeat wrong: %v", opened)
	}
	l.OnKeyDown(gui.KeyHome, false)
	l.OnKeyDown(gui.KeyPageDown, false)
	if l.ActiveIndex() <= 0 || l.ActiveIndex() >= 49 {
		t.Fatal("page navigation not wired")
	}
	l.OnKeyDown(gui.KeyHome, false)
	l.OnLeftDown(30, 15)
	l.OnLeftUp(30, 15)
	if len(opened) != 2 || opened[1] != files[0] {
		t.Fatal("single click target mismatch")
	}
	l.OnLeftUp(30, 15)
	l.OnLeftDown(30, 15)
	l.OnLeftUp(-1, -1)
	if len(opened) != 2 {
		t.Fatal("release-only or drag-out activated")
	}
	w.SetRecentFiles(files[:1])
	_, height := l.Size()
	l.OnLeftDown(30, height-1)
	l.OnLeftUp(30, height-1)
	if len(opened) != 2 {
		t.Fatal("blank space reopened selection")
	}
	w.SetOpenRecentCallback(nil)
	l.OnKeyDown(gui.KeyEnter, false)
	l.OnLeftDown(30, 15)
	l.OnLeftUp(30, 15)
	if len(opened) != 2 {
		t.Fatal("disabled recent list activated")
	}
	w.SetRecentFiles(nil)
	if l.IsVisible() {
		t.Fatal("empty list covers empty state")
	}
}
func TestWelcomeFocusScrollAndWheelChaining(t *testing.T) {
	w := welcomeTest(t)
	w.SetRecentFiles([]string{"/tmp/a.go"})
	w.SetSize(400, 300)
	l := w.recent
	l.OnFocusChanged(l, nil)
	_, y := l.Pos()
	if w.ScrollY() <= 0 || y >= 300 {
		t.Fatal("Tab focus left list offscreen")
	}
	w.newButton.OnFocusChanged(w.newButton, l)
	_, y = w.newButton.Pos()
	if y < 0 || y+w.btnNewRect[3] > 300 {
		t.Fatal("reverse Tab left button offscreen")
	}
	old := w.ScrollY()
	l.OnMouseWheel(20, 20, -1)
	if w.ScrollY() <= old {
		t.Fatal("non-scrolling list swallowed page wheel")
	}
	w.SetSize(900, 700)
	if w.ScrollY() != 0 || w.VertScrollBar().IsVisible() {
		t.Fatal("wide resize retained stale scroll")
	}
}
func TestWelcomeLocalizationElisionAndReadOnlyPaint(t *testing.T) {
	w := welcomeTest(t)
	w.SetSize(900, 600)
	hints := []WelcomeShortcut{{"Cmd+Shift+R", "Preview"}}
	w.SetShortcuts(hints)
	hints[0].Label = "Broken"
	if w.shortcuts[0].Label != "Preview" {
		t.Fatal("hint aliases caller")
	}
	w.SetActionLabels("New Form", "Open File")
	i18n.SetLocale("zh-CN")
	pix := paint.NewPixmap(900, 600)
	g := pix.NewPainter()
	defer paint.CloseSurface(pix)
	defer paint.ClosePainter(g)
	rec := &brandTextRecorder{Painter: g}
	w.Draw(rec)
	if w.newButton.Text() != "新建窗体" || w.openButton.Text() != "打开文件" || !slices.Contains(rec.text, "最近打开") {
		t.Fatalf("translation not refreshed: %v", rec.text)
	}
	fonts, page, rect := w.fonts, w.page, w.btnNewRect
	for i := 0; i < 50; i++ {
		w.Draw(g)
	}
	if w.fonts != fonts || w.page != page || w.btnNewRect != rect {
		t.Fatal("idle paint mutated layout/fonts")
	}
	text := strings.Repeat("目录🙂e\u0301/", 40)
	for _, width := range []float64{0, 1, 12, 30, 70, 140, 400} {
		out := welcomeElide(w.fonts.body, text, width)
		if !utf8.ValidString(out) || w.fonts.body.TextExtents(out).Width > width {
			t.Fatalf("invalid/overflow elision width=%g text=%q", width, out)
		}
	}
}

func TestWelcomeRecentRefreshCancelsPointerActivation(t *testing.T) {
	w := welcomeTest(t)
	w.SetSize(900, 600)
	w.SetRecentFiles([]string{"/tmp/old.go"})
	opened := ""
	w.SetOpenRecentCallback(func(s string) { opened = s })
	w.recent.OnLeftDown(30, 15)
	w.SetRecentFiles([]string{"/tmp/replacement.go"})
	w.recent.OnLeftUp(30, 15)
	if opened != "" {
		t.Fatalf("MRU refresh activated a file that was not pressed: %s", opened)
	}
}
