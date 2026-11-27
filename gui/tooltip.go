package gui

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// Protects only worker-readable registrations; the paint path never takes it.
var tooltipMu sync.Mutex

func init() {
	core.RegisterFactory("gui.tooltipWindow", core.TypeOf((*tooltipWindow)(nil)))
}

// tooltipWindow paints hint contents. The registered factory name is kept
// for compatibility; runtime hints never attach this widget to a window.
type tooltipWindow struct {
	Widget
	text      string
	signature *signatureTipLayout
}

func newTooltipWindow(text string) *tooltipWindow {
	p := new(tooltipWindow)
	p.Init(p)
	p.text = text
	return p
}

func (this *tooltipWindow) Draw(g paint.Painter) {
	w, h := this.Self().Size()

	padX := 8.0
	padY := 4.0
	radius := 4.0

	paint.DrawShadowRect(g, 0, 0, w, h, radius, 3, paint.Color{0, 0, 0, 120})

	// Dark background (modern tooltip style)
	bg := paint.Color{33, 37, 41, 245}
	roundedRect(g, 0, 0, w, h, radius)
	g.SetBrush1(bg)
	g.FillPreserve()

	// Subtle border
	g.SetPen1(paint.Color{55, 60, 66, 255}, 1)
	g.Stroke()

	if this.signature != nil {
		this.signature.draw(g)
		return
	}

	// White text on dark background, smaller font
	t := Theme()
	tipFont := paint.NewFont(t.Font.Family(), 12, false, false)
	g.SetFont(tipFont)
	g.SetBrush1(paint.Color{240, 240, 240, 255})

	fe := tipFont.FontExtents()
	for i, line := range tooltipLines(this.text) {
		g.DrawText1(padX, padY+fe.Ascent+float64(i)*(fe.Height+2), line)
	}

}

func (this *tooltipWindow) SizeHints() SizeHints {
	if this.signature != nil {
		return this.signature.size
	}
	t := Theme()
	tipFont := paint.NewFont(t.Font.Family(), 12, false, false)
	fe := tipFont.FontExtents()
	lines := tooltipLines(this.text)
	w := 0.0
	for _, line := range lines {
		if v := tipFont.TextExtents(line).XAdvance; v > w {
			w = v
		}
	}
	return SizeHints{Width: w + 20, Height: float64(len(lines))*(fe.Height+2) + 8}

}

// --- Public tooltip API ---

// SetToolTip associates tooltip text with a widget.
// Pass an empty string to remove the tooltip.
func SetToolTip(w IWidget, text string) {
	if w == nil {
		return
	}
	base := w.NakedWidget()
	tooltipMu.Lock()
	changed := base.registeredToolTip != text
	base.registeredToolTip = text
	tooltipMu.Unlock()
	if changed && activeHoverToolTipOwner.Load() == base {
		queueRegisteredToolTipRefresh()
	}
}

// GetToolTip returns the text registered directly on w, or "".
func GetToolTip(w IWidget) string {
	if w == nil {
		return ""
	}
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	return w.NakedWidget().registeredToolTip
}

// Resolve the nearest registration without crossing a native/logical window
// boundary. Registrations live on widgets rather than retaining dead forms in
// a process-wide widget map.
func registeredToolTip(w IWidget) (IWidget, string) {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	for p := w; p != nil; p = p.Parent() {
		if text := p.NakedWidget().registeredToolTip; text != "" {
			return p, text
		}
		if p.Window() != nil {
			break
		}
	}
	return nil, ""
}

// ShowToolTip displays a tooltip at global coordinates (xg, yg).
func ShowToolTip(xg, yg float64, text string) {
	ShowToolTipFor(nil, xg, yg, text)
}

// ShowToolTipFor gives asynchronous hints an explicit owner. A caret anchor at
// the viewport edge must not assign the hint to another panel. Screen
// coordinates are converted by the owner's current window and clipped to it.
// Call on the UI thread. A nil owner retains ShowToolTip's hit-tested behavior.
func ShowToolTipFor(owner IWidget, xg, yg float64, text string) {
	showToolTipFor(owner, xg, yg, text, nil)
}

func showToolTipFor(owner IWidget, xg, yg float64, text string, signature *signatureTipSpan) {
	dismissRegisteredToolTip(nil)
	if text == "" {
		replaceExplicitToolTip(nil)
		return
	}
	if owner == nil {
		owner = FindWidgetGlobal(xg, yg)
	}
	if owner == nil || !owner.IsAllAncentorsVisible() {
		replaceExplicitToolTip(nil)
		return
	}
	win := owner.OwnerWindow()
	if win == nil || !win.IsValid() || !win.IsVisible() || !win.IsEnabled() || win.Widget() == nil {
		replaceExplicitToolTip(nil)
		return
	}
	x, y := win.mapFromGlobal(xg, yg)
	label := newTooltipWindow(text)
	if signature != nil {
		w, h := win.Widget().Size()
		label.signature = layoutSignatureTip(text, *signature, w, h)
	}
	tip := newOwnedToolTipLabel(hoverToolTipAnchor{win, x, y}, owner, label)
	if tip != nil && signature != nil {
		tip.avoidCompletion = true
		tip.bounds = tip.signatureBounds()
	}
	replaceExplicitToolTip(tip)
}

// HideToolTip retires both explicit and automatic hints.
func HideToolTip() {
	dismissRegisteredToolTip(nil)
	replaceExplicitToolTip(nil)
}

// HideToolTipFor dismisses only this owner's hint; a background editor update
// must not remove a tooltip belonging to another editor or toolbar button.
func HideToolTipFor(owner IWidget) {
	if current := visibleHoverToolTip; current != nil && current.owner == owner {
		dismissRegisteredToolTip(current.win)
	}
	dismissExplicitToolTip(func(tip *hoverToolTipOverlay) bool { return tip.owner == owner })
}

// IsToolTipVisible returns true if a tooltip is currently shown.
func IsToolTipVisible() bool {
	return currentTooltip.Load() != nil || activeHoverToolTipOwner.Load() != nil
}

// CheckToolTip explicitly shows the nearest registered hint at screen
// coordinates. Native automatic hover uses the event's actual client window.
func CheckToolTip(xg, yg float64) {
	owner, text := registeredToolTip(FindWidgetGlobal(xg, yg))
	if owner == nil {
		HideToolTip()
		return
	}
	ShowToolTipFor(owner, xg, yg, text)
}

// OnMouseMoveToolTip should be called when the mouse moves to hide the tooltip.
func OnMouseMoveToolTip() {
	HideToolTip()
}

// Tooltips are multiline text, not one Cairo glyph run. Bound remote LSP
// documentation and expand tabs so control characters never become tofu.
func tooltipLines(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", "    ")
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		for utf8.RuneCountInString(line) > 100 {
			r := []rune(line)
			lines = append(lines, string(r[:100]))
			line = string(r[100:])
			if len(lines) >= 16 {
				return append(lines, "…")
			}
		}
		lines = append(lines, line)
		if len(lines) >= 16 {
			return append(lines, "…")
		}
	}
	return lines
}
