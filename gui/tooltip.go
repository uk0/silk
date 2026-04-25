package gui

import (
	"silk/core"
	"silk/paint"
	"sync"
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
	text string
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

	// Drop shadow (offset 1px down-right, semi-transparent)
	roundedRect(g, 1, 1, w, h, radius)
	g.SetBrush1(paint.Color{0, 0, 0, 40})
	g.Fill()

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

	ext := tipFont.TextExtents(this.text)
	xt := padX - ext.XBearing
	yt := padY + (h-2*padY+ext.YBearing)*0.5 - ext.YBearing
	g.Translate(xt, yt)
	g.DrawText(this.text)
	g.Translate(-xt, -yt)
}

func (this *tooltipWindow) SizeHints() SizeHints {
	t := Theme()
	tipFont := paint.NewFont(t.Font.Family(), 12, false, false)
	ext := tipFont.TextExtents(this.text)
	fe := tipFont.FontExtents()
	padX := 8.0
	padY := 4.0
	w := ext.Width + padX*2 + 4 // +4 for border + shadow
	h := fe.Height + padY*2 + 4
	return SizeHints{Width: w, Height: h}
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

	tip := newTooltipWindow(text)
	tip.LazyAttachWindow(WtPopup)

	hints := tip.SizeHints()
	tip.SetSize(0, 0)
	tip.SetSize(hints.Width, hints.Height)

	// position near cursor, offset slightly below and to the right
	tx := xg + 12
	ty := yg + 18

	// ensure tooltip stays on screen
	dx, dy, dw, dh := DesktopArea()
	if tx+hints.Width > dx+dw {
		tx = dx + dw - hints.Width
	}
	if ty+hints.Height > dy+dh {
		ty = yg - hints.Height - 4
	}
	if tx < dx {
		tx = dx
	}
	if ty < dy {
		ty = dy
	}

	tip.SetPos(tx, ty)
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
		currentTooltip = nil
	}
}

// HideToolTip hides and destroys the current tooltip if any.
func HideToolTip() {
	tooltipMu.Lock()
	defer tooltipMu.Unlock()
	hideToolTipLocked()
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
