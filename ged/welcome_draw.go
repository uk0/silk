package ged

import (
	"strings"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Draw only refreshes layout when presentation inputs change, never on every
// frame. Buttons/list are real children and are painted by the widget tree.
func (w *WelcomeScreen) Draw(g paint.Painter) {
	f := gui.Theme().Font
	if w.fontFamily != f.Family() || w.fontSize != f.Size() || w.locale != i18n.Locale() {
		w.Layout()
	}
	width, height := w.Size()
	p := w.page
	pal := currentWelcomePalette()
	s := p.scale
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, width, height)
	g.SetBrush1(pal.background)
	g.Fill()
	g.Rectangle(0, 0, width, height)
	g.Clip()
	g.Translate(p.x, p.y-w.ScrollY())
	name, version, footer := "Silk Designer", aboutAppVersion, "Silk UI Framework"
	desc := "A visual designer for Go interfaces."
	if strings.TrimSpace(w.branding.Name) != "" {
		name = w.branding.Name
		footer = name
		desc = "Go editing and visual interface design."
	}
	if strings.TrimSpace(w.branding.Version) != "" {
		version = "v" + strings.TrimPrefix(w.branding.Version, "v")
	}
	tx := 0.0
	if w.branding.Logo != nil && !w.branding.Logo.IsAir() && p.leftWidth >= 220*s {
		g.DrawIcon1(w.branding.Logo, 0, p.titleY-44*s, 56*s, false)
		tx = 68 * s
	}
	welcomeText(g, w.fonts.title, pal.title, tx, p.titleY, p.leftWidth-tx, name)
	welcomeText(g, w.fonts.subtitle, pal.accent, 0, p.titleY+22*s, p.leftWidth, version+"  ·  Go UI Framework")
	welcomeText(g, w.fonts.small, pal.muted, 0, p.titleY+44*s, p.leftWidth, i18n.T(desc))
	welcomeText(g, w.fonts.small, pal.muted, 0, p.titleY+60*s, p.leftWidth, i18n.T("Drag, arrange and generate Go."))
	g.SetPen1(pal.accent, 2*s)
	g.MoveTo(0, p.titleY+78*s)
	g.LineTo(min(64*s, p.leftWidth), p.titleY+78*s)
	g.Stroke()
	if len(w.shortcuts) > 0 {
		welcomeText(g, w.fonts.subtitle, pal.text, 0, p.tipsY, p.leftWidth, i18n.T("Keyboard shortcuts"))
		kw := 0.0
		for _, hint := range w.shortcuts {
			kw = max(kw, w.fonts.small.TextExtents(hint.Keys).Width)
		}
		kw = min(kw, p.leftWidth*.55)
		for i, hint := range w.shortcuts {
			y := p.tipsY + float64(i+1)*20*s
			welcomeText(g, w.fonts.small, pal.muted, 0, y, kw, hint.Keys)
			welcomeText(g, w.fonts.small, pal.text, kw+12*s, y, p.leftWidth-kw-12*s, i18n.T(hint.Label))
		}
	}
	r := p.cardRect
	g.SetBrush1(pal.surface)
	welcomeRoundRect(g, r[0], r[1], r[2], r[3], 8*s)
	g.Fill()
	g.SetPen1(pal.border, 1)
	welcomeRoundRect(g, r[0]+.5, r[1]+.5, max(0, r[2]-1), max(0, r[3]-1), 8*s)
	g.Stroke()
	welcomeText(g, w.fonts.section, pal.text, r[0]+16*s, r[1]+30*s, r[2]-32*s, i18n.T("Recent files"))
	if len(w.recentFiles) == 0 {
		welcomeText(g, w.fonts.body, pal.muted, r[0]+16*s, r[1]+76*s, r[2]-32*s, i18n.T("No recent files yet"))
		welcomeText(g, w.fonts.small, pal.muted, r[0]+16*s, r[1]+100*s, r[2]-32*s, i18n.T("Open a file to get started."))
	}
	welcomeText(g, w.fonts.small, pal.muted, 0, p.footerY, p.width, footer+"  ·  "+version+"  ·  "+aboutCopyright)
}
