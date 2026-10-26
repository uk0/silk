package ged

import (
	"math"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

type welcomeFonts struct{ title, subtitle, body, small, section paint.Font }
type welcomePageLayout struct {
	x, y, width, height, leftWidth, scale float64
	titleY, tipsY, footerY                float64
	newRect, openRect, cardRect           [4]float64
}

func (w *WelcomeScreen) Layout() {
	if w.layouting || w.newButton == nil || w.openButton == nil || w.recent == nil {
		return
	}
	w.layouting = true
	defer func() { w.layouting = false }()
	f := gui.Theme().Font
	s := math.Max(1, float64(f.Size())/14)
	if w.fontFamily != f.Family() || w.fontSize != f.Size() || w.fonts.title == nil {
		w.fontFamily, w.fontSize = f.Family(), f.Size()
		font := func(size float64, bold bool) paint.Font {
			return paint.NewFont(f.Family(), int(math.Round(size*s)), bold, false)
		}
		w.fonts = welcomeFonts{font(28, true), font(13, true), font(12, false), font(11, false), font(15, true)}
	}
	w.locale = i18n.Locale()
	w.newButton.SetText(i18n.T(w.newLabel))
	w.openButton.SetText(i18n.T(w.openLabel))
	gui.SetToolTip(w.newButton, i18n.T(w.newLabel))
	gui.SetToolTip(w.openButton, i18n.T(w.openLabel))
	width, height := w.Size()
	width, height = math.Max(0, width), math.Max(0, height)
	margin := math.Min(20*s, width/4)
	makePage := func(available float64) welcomePageLayout {
		cw := math.Max(0, math.Min(720*s, available-2*margin))
		left := cw
		wide := cw >= 680*s
		if wide {
			left = cw * .44
		}
		bh := math.Max(36*s, gui.Theme().ItemHeight+12)
		nw := math.Min(left, math.Max(140*s, w.newButton.SizeHints().Width+16*s))
		ow := math.Min(left, math.Max(140*s, w.openButton.SizeHints().Width+16*s))
		by := 140 * s
		nr := [4]float64{0, by, nw, bh}
		or := [4]float64{nw + 12*s, by, ow, bh}
		bottom := by + bh
		if nw+12*s+ow > left {
			or[0] = 0
			or[1] = bottom + 10*s
			bottom = or[1] + bh
		}
		tips := bottom + 28*s
		intro := tips
		if len(w.shortcuts) > 0 {
			intro += float64(len(w.shortcuts))*20*s + 20*s
		}
		ch := math.Max(460*s, intro+28*s)
		card := [4]float64{left + 20*s, 30 * s, math.Max(0, cw-left-20*s), ch - 50*s}
		if !wide {
			card = [4]float64{0, intro + 24*s, cw, math.Max(160*s, math.Min(320*s, 64*s+float64(len(w.recentFiles))*44*s))}
			ch = card[1] + card[3] + 38*s
		}
		return welcomePageLayout{x: math.Max(0, (available-cw)/2), y: math.Max(20*s, (height-ch)/2), width: cw, height: ch, leftWidth: left, scale: s, titleY: 42 * s, tipsY: tips, footerY: ch - 6*s, newRect: nr, openRect: or, cardRect: card}
	}
	page := makePage(width)
	needScroll := page.height+40*s > height && width > gui.Theme().ScrollWidth+4
	if needScroll {
		page = makePage(math.Max(0, width-gui.Theme().ScrollWidth))
	}
	w.page = page
	vs := w.VertScrollBar()
	vs.SetFocusPolicy(gui.NoFocus)
	vs.SetRange(0, math.Max(0, page.height+40*s-height))
	vs.SetDelta(40*s, math.Max(40*s, height*.8))
	vs.SetVisible(needScroll)
	if !needScroll {
		w.SetScrollY(0)
	}
	w.recent.SetRowHeight(math.Max(44*s, 2*w.fonts.body.FontExtents().Height+12*s))
	w.recent.SetPadding(0, 0, 0, 0)
	w.applyGeometry()
	w.ScrollArea.Layout()
}
func (w *WelcomeScreen) applyGeometry() {
	p := w.page
	dy := p.y - w.ScrollY()
	rect := func(r [4]float64) [4]float64 { return [4]float64{p.x + r[0], dy + r[1], r[2], r[3]} }
	w.btnNewRect, w.btnOpenRect = rect(p.newRect), rect(p.openRect)
	set := func(c gui.IWidget, r [4]float64) { c.SetBounds(r[0], r[1], math.Max(0, r[2]), math.Max(0, r[3])) }
	set(w.newButton, w.btnNewRect)
	set(w.openButton, w.btnOpenRect)
	r := rect(p.cardRect)
	r[0] += 8 * p.scale
	r[1] += 48 * p.scale
	r[2] = math.Max(0, r[2]-16*p.scale)
	r[3] = math.Max(0, r[3]-56*p.scale)
	set(w.recent, r)
	w.recentRects = [][4]float64{r}
}
func (w *WelcomeScreen) OnVertScroll(gui.IWidget)     { w.applyGeometry(); w.Update() }
func (w *WelcomeScreen) OnMouseWheel(x, y, z float64) { w.SetScrollY(w.ScrollY() - z*40*w.page.scale) }

// Rune boundaries and measured advances keep long CJK/emoji paths within their
// column; byte-count truncation can split UTF-8 and still overflow visually.
func welcomeElide(font paint.Font, text string, width float64) string {
	if width <= 0 {
		return ""
	}
	if font.TextExtents(text).Width <= width {
		return text
	}
	const mark = "…"
	if font.TextExtents(mark).Width > width {
		return ""
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if font.TextExtents(string(runes[:mid])+mark).Width <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]) + mark
}
func welcomeText(g paint.Painter, font paint.Font, color paint.Color, x, y, width float64, text string) {
	g.SetFont(font)
	g.SetBrush1(color)
	g.DrawText1(x, y, welcomeElide(font, text, width))
}
func hitRect(x, y float64, r [4]float64) bool {
	return r[2] > 0 && r[3] > 0 && x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3]
}
