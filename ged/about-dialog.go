package ged

import (
	"fmt"
	"runtime"
	"silk/gui"
	"silk/paint"
)

// Version metadata for the About dialog.
const (
	aboutAppName      = "Silk UI Framework"
	aboutAppVersion   = "v2.3.0"
	aboutAppTagline   = "Cross-platform Go UI framework with\nvisual designer and integrated IDE"
	aboutAppHighlight = "62+ widgets  \u00b7  Qt Creator-level designer  \u00b7  86,000+ lines of Go"
	aboutCopyright    = "\u00a9 2024-2026 Silk Framework Contributors"
)

// ShowAboutDialog presents a polished About dialog for Silk Designer.
// It is theme-aware and uses a vertical layout with a title, version,
// description, runtime information, and an OK button.
func ShowAboutDialog(parent gui.IWidget) {
	if parent == nil {
		parent = gui.DefaultFrame()
	}

	dlg := gui.NewDialog("\u5173\u4e8e Silk Designer", parent)

	content := gui.NewVBox()
	content.SetSpacing(8)
	content.SetPadding(gui.Padding{L: 32, R: 32, T: 28, B: 18})

	fontFamily := gui.Theme().Font.Family()
	accent := gui.Theme().HighLightColor
	mutedText := paint.Color{R: 120, G: 120, B: 140, A: 255}
	if gui.CurrentThemeMode() == gui.ThemeDark {
		mutedText = paint.Color{R: 170, G: 170, B: 185, A: 255}
	}

	// App title
	title := gui.NewLabel(aboutAppName)
	title.SetFont(paint.NewFont(fontFamily, 22, true, false))
	title.SetAlign(gui.AlignCenter)
	content.AddWidget(title)

	// Version line (accent color)
	version := gui.NewLabel(aboutAppVersion)
	version.SetFont(paint.NewFont(fontFamily, 13, true, false))
	version.SetTextColor(accent)
	version.SetAlign(gui.AlignCenter)
	content.AddWidget(version)

	// Small vertical gap
	content.AddWidget(newAboutSpacer(8))

	// Primary description
	desc := gui.NewLabel(aboutAppTagline)
	desc.SetFont(paint.NewFont(fontFamily, 12, false, false))
	desc.SetAlign(gui.AlignCenter)
	desc.SetWrap(true)
	content.AddWidget(desc)

	// Highlight / feature summary line
	highlight := gui.NewLabel(aboutAppHighlight)
	highlight.SetFont(paint.NewFont(fontFamily, 11, false, false))
	highlight.SetTextColor(mutedText)
	highlight.SetAlign(gui.AlignCenter)
	highlight.SetWrap(true)
	content.AddWidget(highlight)

	// Divider spacer
	content.AddWidget(newAboutSpacer(6))
	content.AddWidget(newAboutDivider())
	content.AddWidget(newAboutSpacer(4))

	// Runtime stats
	stats := fmt.Sprintf(
		"Go %s  \u00b7  %s/%s\nCairo + OpenGL rendering",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
	statsLabel := gui.NewLabel(stats)
	statsLabel.SetFont(paint.NewFont(fontFamily, 11, false, false))
	statsLabel.SetTextColor(mutedText)
	statsLabel.SetAlign(gui.AlignCenter)
	statsLabel.SetWrap(true)
	content.AddWidget(statsLabel)

	content.AddWidget(newAboutSpacer(6))

	// Copyright footer
	footer := gui.NewLabel(aboutCopyright)
	footer.SetFont(paint.NewFont(fontFamily, 10, false, false))
	footer.SetTextColor(mutedText)
	footer.SetAlign(gui.AlignCenter)
	content.AddWidget(footer)

	dlg.SetContent(content)
	dlg.AddButton("\u786e\u5b9a", gui.DialogOK)
	dlg.ShowModal()
}

// aboutSpacer is an empty widget that reserves vertical space inside the
// About dialog's VBox layout. It does not draw anything.
type aboutSpacer struct {
	gui.Widget
	height float64
}

func newAboutSpacer(height float64) *aboutSpacer {
	s := new(aboutSpacer)
	s.Init(s)
	s.height = height
	return s
}

func (this *aboutSpacer) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 0, Height: this.height, Policy: gui.GrowHorizontal}
}

func (this *aboutSpacer) Draw(g paint.Painter) {}

// aboutDivider draws a thin horizontal theme-aware line across its bounds.
type aboutDivider struct {
	gui.Widget
}

func newAboutDivider() *aboutDivider {
	d := new(aboutDivider)
	d.Init(d)
	return d
}

func (this *aboutDivider) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 0, Height: 1, Policy: gui.GrowHorizontal | gui.ExpandHorizontal}
}

func (this *aboutDivider) Draw(g paint.Painter) {
	w, h := this.Self().Size()
	y := h * 0.5
	g.SetPen1(gui.Theme().BorderColor, 1)
	g.MoveTo(0, y)
	g.LineTo(w, y)
	g.Stroke()
}
