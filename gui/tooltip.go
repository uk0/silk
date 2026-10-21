package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"strings"
	"sync"
	"unicode/utf8"
)

// Global tooltip state
var (
	tooltipMu      sync.Mutex
	tooltipMap     = make(map[IWidget]string)
	currentTooltip *tooltipWindow
	tooltipTimer   Timer
)

func init() {
	core.RegisterFactory("gui.tooltipWindow", core.TypeOf((*tooltipWindow)(nil)))
}

// tooltipWindow is a small popup that displays tooltip text.
type tooltipWindow struct {
	Widget
	text  string
	owner IWidget
}

// A tooltip has an owner but is not a layout/serialized child of that widget.
func (this *tooltipWindow) Parent() IWidget { return this.owner }

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
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	if text == "" {
		delete(tooltipMap, w)
	} else {
		tooltipMap[w] = text
	}
}

// GetToolTip returns the tooltip text for a widget, or "".
func GetToolTip(w IWidget) string {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	return tooltipMap[w]
}

// ShowToolTip displays a tooltip at global coordinates (xg, yg).
func ShowToolTip(xg, yg float64, text string) {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	hideToolTipLocked()

	if text == "" {
		return
	}
	owner := FindWidgetGlobal(xg, yg)
	if owner == nil || owner.OwnerWindow() == nil || !owner.IsAllAncentorsVisible() {
		return
	}

	tip := newTooltipWindow(text)
	tip.owner = owner
	tip.SetVisible(false)

	hints := tip.SizeHints()
	tip.SetSize(0, 0)
	tip.SetSize(hints.Width, hints.Height)
	tip.AttachWindow(WtPopup)

	// Use the cursor's monitor, not the primary monitor or the offset tip.
	area := popupWorkAreaAt(xg, yg)
	tx, ty := tooltipPositionInWorkArea(area, xg, yg, hints.Width, hints.Height)
	setPopupGlobalPos(tip, tx, ty)
	tip.Show()

	currentTooltip = tip
}

// hideToolTipLocked hides and destroys the current tooltip.
// Caller must hold tooltipMu.
func hideToolTipLocked() {
	tooltipTimer.Stop()
	if currentTooltip != nil {
		currentTooltip.Hide()
		currentTooltip.DetachWindow()
		currentTooltip.Detach()
		currentTooltip.owner = nil
		currentTooltip = nil
	}
}

// HideToolTip hides and destroys the current tooltip if any.
func HideToolTip() {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	hideToolTipLocked()
}

// HideToolTipFor dismisses only this owner's hint; a background editor update
// must not remove a tooltip belonging to another editor or toolbar button.
func HideToolTipFor(owner IWidget) {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	if currentTooltip != nil && currentTooltip.owner == owner {
		hideToolTipLocked()
	}
}

// IsToolTipVisible returns true if a tooltip is currently shown.
func IsToolTipVisible() bool {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	return currentTooltip != nil
}

// CheckToolTip is intended to be called from a mouse idle/stop handler.
// It checks whether the widget under the mouse has a tooltip and shows it.
// xg, yg are global (screen) coordinates of the mouse cursor.
func CheckToolTip(xg, yg float64) {
	w := FindWidgetGlobal(xg, yg)
	if w == nil {
		HideToolTip()
		return
	}

	// walk up the widget tree to find a tooltip
	tooltipMu.Lock()
	text := ""
	for p := w; p != nil; p = p.Parent() {
		if t, ok := tooltipMap[p]; ok {
			text = t
			break
		}
	}
	tooltipMu.Unlock()

	if text == "" {
		HideToolTip()
		return
	}

	ShowToolTip(xg, yg, text)
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
