package ged

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func aboutTestEnvironment(t *testing.T) {
	t.Helper()
	mode, palette, font, translator := gui.CurrentThemeMode(), paint.CurrentIconPalette(), gui.Theme().Font, i18n.Default
	t.Cleanup(func() {
		gui.SetThemeMode(mode)
		gui.Theme().Font = font
		paint.SetIconPalette(palette)
		i18n.Default = translator
	})
	i18n.Default = i18n.NewTranslator()
	i18n.Default.AddMany("zh-CN", map[string]string{
		"About": "关于", "Close": "关闭", "App version": "应用版本", "SDK version": "SDK 版本",
		"A visual designer for Go interfaces.": "面向 Go 界面的可视化设计器。",
		"Drag, arrange and generate Go.":       "拖拽控件、调整布局并生成 Go 代码。",
		"Build":                                "构建", "Runtime": "运行时", "Platform": "平台", "Rendering": "渲染",
		"Development build": "开发构建", "Working tree modified": "工作区有修改",
		"Build information unavailable": "构建信息不可用",
	})
}

func aboutCloseButton(t *testing.T, widget gui.IWidget) *gui.Button {
	t.Helper()
	var found *gui.Button
	var visit func(gui.IWidget)
	visit = func(w gui.IWidget) {
		if b, ok := w.(*gui.Button); ok && b.Text() == i18n.T("Close") {
			found = b
		}
		for _, child := range w.Children() {
			visit(child)
		}
	}
	visit(widget)
	if found == nil {
		t.Fatal("missing actionable close button")
	}
	return found
}

func TestAboutDialogReportsHostAndActualRuntime(t *testing.T) {
	aboutTestEnvironment(t)
	for _, locale := range []string{"en-US", "zh-CN"} {
		i18n.SetLocale(locale)
		dlg := newAboutDialog(nil, AboutInfo{Name: "Silk IDE", Version: "0.1.3"})
		body := dlg.Content().(*aboutContent)
		if dlg.Title() != i18n.T("About")+" Silk IDE" || body.info.Name != "Silk IDE" {
			t.Fatal("host application identity was replaced by SDK branding")
		}
		rows := make(map[string]string)
		for _, row := range body.details() {
			rows[row.label] = row.value
		}
		for key, want := range map[string]string{
			"App version": "v0.1.3", "SDK version": "v" + core.Version,
			"Runtime":  "Go " + strings.TrimPrefix(runtime.Version(), "go"),
			"Platform": runtime.GOOS + "/" + runtime.GOARCH, "Rendering": aboutRenderer(runtime.GOOS),
		} {
			if rows[i18n.T(key)] != want {
				t.Errorf("%s=%q, want %q", key, rows[i18n.T(key)], want)
			}
		}
		lines, _ := body.textLayout(480)
		var all strings.Builder
		for _, line := range lines {
			all.WriteString(line.text)
		}
		for _, invalid := range []string{"62+", "86,000", "Qt Creator-level", "©", "Go go", "v2.3.0"} {
			if strings.Contains(all.String(), invalid) {
				t.Errorf("unverified or malformed About content: %q", invalid)
			}
		}
		if !aboutCloseButton(t, dlg).IsDefault() {
			t.Fatal("close must be the default keyboard action")
		}
	}
	standalone := newAboutDialog(nil, AboutInfo{}).Content().(*aboutContent)
	if standalone.info.Name != "Silk Designer" {
		t.Fatal("standalone designer branding changed")
	}
	for _, row := range standalone.details() {
		if row.label == i18n.T("App version") {
			t.Fatal("invented a host version when none was supplied")
		}
	}
}

func TestAboutBuildMetadataAndRendererAreNotMarketingConstants(t *testing.T) {
	aboutTestEnvironment(t)
	info := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "1234567890abcdef"}, {Key: "vcs.modified", Value: "true"},
	}}
	if got := aboutBuildDescription(info, true); got != "Development build · 1234567890ab · Working tree modified" {
		t.Fatalf("build provenance changed: %q", got)
	}
	if got := aboutBuildDescription(&debug.BuildInfo{Main: debug.Module{Version: "v9.8.7"}}, true); got != "v9.8.7" {
		t.Fatal("release build version ignored")
	}
	if aboutBuildDescription(nil, false) != "Build information unavailable" {
		t.Fatal("missing build metadata must not pretend to be a release")
	}
	for osName, want := range map[string]string{"darwin": "Cairo · OpenGL (GLFW)", "linux": "Cairo · OpenGL (GLFW)", "windows": "Cairo · Win32", "unknown": "Cairo"} {
		if got := aboutRenderer(osName); got != want {
			t.Errorf("%s renderer = %q, want %q", osName, got, want)
		}
	}
	if got := aboutGoVersion("devel go1.27-abcdef"); got != "Go devel go1.27-abcdef" {
		t.Fatal("development runtime metadata was truncated")
	}
}

type aboutDrawRecorder struct {
	paint.Painter
	brush paint.Color
	texts []string
	inks  []paint.Color
}

func (r *aboutDrawRecorder) SetBrush1(color paint.Color) {
	r.brush = color
	r.Painter.SetBrush1(color)
}
func (r *aboutDrawRecorder) DrawText1(x, y float64, text string) {
	r.texts = append(r.texts, text)
	r.inks = append(r.inks, r.brush)
	r.Painter.DrawText1(x, y, text)
}

func aboutContrast(a, b paint.Color) float64 {
	luminance := func(c paint.Color) float64 {
		linear := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func TestAboutDialogMeasuredLayoutAndRealThemeDraw(t *testing.T) {
	aboutTestEnvironment(t)
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		for _, locale := range []string{"en-US", "zh-CN"} {
			i18n.SetLocale(locale)
			for _, fontSize := range []int{14, 20, 28} {
				gui.Theme().Font = paint.NewFont(gui.Theme().Font.Family(), fontSize, false, false)
				preferred := newAboutDialog(nil, AboutInfo{Name: "Silk IDE", Version: "0.1.3"})
				pw, ph := preferred.Size()
				for _, size := range [][2]float64{{pw, ph}, {528, 500}, {380, 320}, {320, 240}} {
					t.Run(fmt.Sprintf("mode-%d/%s/font-%d/%gx%g", mode, locale, fontSize, size[0], size[1]), func(t *testing.T) {
						dlg := newAboutDialog(nil, AboutInfo{Name: "Silk IDE", Version: "0.1.3"})
						dlg.SetSize(size[0], size[1])
						dlg.Layout()
						body := dlg.Content().(*aboutContent)
						vw, vh := body.ViewportSizePx()
						lines, total := body.textLayout(vw - 4)
						for i, line := range lines {
							ext := line.font.TextExtents(line.text)
							if ext.Width > line.width+0.01 || line.x < 0 || line.y < 0 || line.x+ext.Width > vw+0.01 {
								t.Fatalf("text overflow at %q: %+v ink width=%g viewport=%g", line.text, line, ext.Width, vw)
							}
							for _, before := range lines[:i] {
								if line.x < before.x+before.width && before.x < line.x+line.width && line.y < before.y+before.height && before.y < line.y+line.height {
									t.Fatalf("overlapping text lines %q and %q", before.text, line.text)
								}
							}
						}
						if total > vh && !body.VertScrollBar().IsVisible() {
							t.Fatal("small/large-font dialog clips content without scrolling")
						}
						btn := aboutCloseButton(t, dlg)
						bx, by := btn.MapToWindow(0, 0)
						if btn.Width() < 88 || bx < size[0]/2 || bx+btn.Width() > size[0] || by < body.Y()+body.Height() || by+btn.Height() > size[1] {
							t.Fatalf("close target is too small, not right-aligned or overlaps body: (%g,%g,%g,%g) content bottom=%g", bx, by, btn.Width(), btn.Height(), body.Y()+body.Height())
						}
						pix := paint.NewPixmap(int(size[0])*2, int(size[1])*2)
						g := pix.NewPainter()
						g.Scale(2, 2)
						rec := &aboutDrawRecorder{Painter: g}
						gui.DrawWidgetAll(dlg, rec, 0, 0, 0, 0, size[0], size[1])
						if len(rec.texts) == 0 || rec.texts[0] != "Silk IDE" {
							t.Fatalf("real draw lost the title: %q", rec.texts)
						}
						img, err := pix.Image()
						if err != nil {
							t.Fatal(err)
						}
						bg := paint.ColorModel.Convert(img.At(2, 2)).(paint.Color)
						if bg != gui.Theme().ViewBGColor {
							t.Fatalf("body pixel=%v, want %v", bg, gui.Theme().ViewBGColor)
						}
						// The recorded DrawText call alone cannot prove Cairo put
						// visible title pixels on screen rather than clipping them.
						titleInk := 0
						for y := int(body.Y() * 2); y < int((body.Y()+lines[0].y+lines[0].height)*2); y++ {
							for x := int(body.X() * 2); x < int((body.X()+vw)*2); x++ {
								if paint.ColorModel.Convert(img.At(x, y)).(paint.Color) == gui.Theme().TextColor {
									titleInk++
								}
							}
						}
						if titleInk < 20 {
							t.Fatalf("title was recorded but not visibly rendered: %d pixels", titleInk)
						}
						for i, ink := range rec.inks {
							if ratio := aboutContrast(ink, bg); ratio < 4.5 {
								t.Fatalf("%q text contrast %.2f:1", rec.texts[i], ratio)
							}
						}
						if path := os.Getenv("SILK_ABOUT_QA_DIR"); path != "" && fontSize == 14 && size[0] == 528 {
							if err := os.MkdirAll(path, 0o700); err != nil {
								t.Fatal(err)
							}
							suffix := ""
							if size[1] == ph {
								suffix = "-preferred"
							}
							if err := pix.WritePNG(filepath.Join(path, fmt.Sprintf("about-%d-%s%s-2x.png", mode, locale, suffix))); err != nil {
								t.Fatal(err)
							}
						}
						body.SetScrollY(math.Max(0, total-vh))
						if body.ScrollY() < 0 || body.ScrollY() > math.Max(0, total-vh)+0.01 {
							t.Fatal("scroll escaped content bounds")
						}
						body.OnMouseWheel(0, 0, 1000)
						if body.ScrollY() != 0 {
							t.Fatal("wheel cannot return to title")
						}
					})
				}
			}
		}
	}
}

func TestAboutWrapPreservesUnicodeAndExplicitLineBreaks(t *testing.T) {
	aboutTestEnvironment(t)
	font := gui.Theme().Font
	for _, text := range []string{"中文多语言界面设计", "a very long translated explanation", "word\nsecond", strings.Repeat("W", 90)} {
		// Fontconfig may substitute a narrower face (the minimal Linux image
		// measures this CJK fixture at 72px, versus >90px on macOS). Construct
		// an actually over-wide line using the active renderer's font metrics,
		// rather than assuming every platform has PingFang/CJK fonts installed.
		inkWidth := font.TextExtents(strings.ReplaceAll(text, "\n", "")).Width
		if inkWidth <= 0 {
			t.Fatalf("font %s cannot measure %q", font.String(), text)
		}
		width := math.Min(90, inkWidth/2)
		lines := aboutWrapText(text, font, width)
		if len(lines) < 2 {
			t.Fatalf("text was not wrapped: %q (font=%s, ink width=%g, wrap width=%g)", text, font.String(), inkWidth, width)
		}
		for _, line := range lines {
			if strings.Contains(line, "\n") || font.TextExtents(line).Width > width {
				t.Fatalf("unrenderable line: %q", line)
			}
		}
		withoutSpace := func(s string) string { return strings.Join(strings.Fields(s), "") }
		if withoutSpace(strings.Join(lines, "")) != withoutSpace(text) {
			t.Fatalf("wrapping lost content: %q -> %q", text, lines)
		}
	}
}

func TestAboutDoesNotRestoreDetachedOwnerFocus(t *testing.T) {
	aboutTestEnvironment(t)
	owner := gui.NewForm()
	edit := gui.NewEdit()
	edit.SetParent(owner)
	edit.SetFocus()
	showAboutDialog(owner, AboutInfo{}, func(dlg *gui.Dialog) gui.DialogResult {
		dlg.SetFocus() // simulate native activation after owner capture
		edit.SetParent(nil)
		dlg.OnKeyDown(gui.KeyEsc, false)
		return dlg.Result()
	})
	if edit.HasFocus() {
		t.Fatal("modal exit restored a removed owner's widget")
	}
}

func TestAboutDialogCloseActionsRestoreOwnerFocus(t *testing.T) {
	aboutTestEnvironment(t)
	for _, action := range []string{"click", "enter", "escape", "button-enter", "button-space", "button-escape"} {
		t.Run(action, func(t *testing.T) {
			owner := gui.NewForm()
			edit := gui.NewEdit()
			edit.SetParent(owner)
			edit.SetFocus()
			result := showAboutDialog(owner, AboutInfo{Name: "Silk IDE"}, func(dlg *gui.Dialog) gui.DialogResult {
				if !edit.HasFocus() || dlg.HasFocus() {
					t.Fatal("About stole owner focus before presentation")
				}
				// The injected presenter simulates native activation after capture.
				dlg.SetFocus()
				// Prime the opposite result so a missing action cannot pass
				// merely because DialogResult's zero value happens to be OK.
				if !strings.HasSuffix(action, "escape") {
					dlg.OnKeyDown(gui.KeyEsc, false)
				}
				switch action {
				case "click":
					button := aboutCloseButton(t, dlg)
					button.Action().Trigger(button)
				case "enter":
					dlg.OnKeyDown(gui.KeyEnter, false)
				case "escape":
					dlg.OnKeyDown(gui.KeyEsc, false)
				case "button-enter", "button-space", "button-escape":
					button := aboutCloseButton(t, dlg)
					button.SetFocus()
					key := gui.KeyEnter
					if action == "button-space" {
						key = gui.KeySpace
					}
					if action == "button-escape" {
						key = gui.KeyEsc
					}
					button.OnKeyDown(key, false)
				}
				return dlg.Result()
			})
			want := gui.DialogOK
			if strings.HasSuffix(action, "escape") {
				want = gui.DialogCancel
			}
			if result != want || !edit.HasFocus() {
				t.Fatalf("result=%v want=%v restored focus=%v", result, want, edit.HasFocus())
			}
		})
	}
}

func TestAboutPresentationPreservesFocusOutsideParentSubtree(t *testing.T) {
	aboutTestEnvironment(t)
	frame := gui.NewForm()
	editor := gui.NewEdit()
	editor.SetParent(frame)
	editor.SetFocus()
	// The IDE opens About using its designer canvas even while editing a text
	// document in a sibling panel. Native ShowModal must capture that editor.
	canvas := new(gui.Widget)
	canvas.Init(canvas)
	canvas.SetParent(frame)
	canvas.SetVisible(false)
	showAboutDialog(canvas, AboutInfo{}, func(dlg *gui.Dialog) gui.DialogResult {
		if !editor.HasFocus() || dlg.HasFocus() {
			t.Fatal("About stole the sibling editor focus before native modal capture")
		}
		return gui.DialogCancel
	})
}

func TestAboutCloseDoesNotOverwriteNewFocus(t *testing.T) {
	aboutTestEnvironment(t)
	owner := gui.NewForm()
	editor := gui.NewEdit()
	editor.SetParent(owner)
	editor.SetFocus()
	other := gui.NewEdit()
	showAboutDialog(owner, AboutInfo{}, func(dlg *gui.Dialog) gui.DialogResult {
		dlg.SetFocus()
		// A callback deliberately moved focus, possibly into another Form.
		other.SetFocus()
		return gui.DialogCancel
	})
	if !other.HasFocus() {
		t.Fatal("About restored stale focus over an explicit new target")
	}
}
