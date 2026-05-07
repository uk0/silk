package gui

import (
	"silk/paint"
	"math"
	"sync"
)

// ToastLevel represents the severity level of a toast notification.
type ToastLevel int

const (
	ToastInfo    ToastLevel = iota // Blue
	ToastSuccess                   // Green
	ToastWarning                   // Amber
	ToastError                     // Red
)

// toastEntry represents a single active toast notification.
type toastEntry struct {
	Widget
	message    string
	level      ToastLevel
	durationMs uint32
	timer      Timer
	parent     IWidget
}

// toastColors returns background and text colors for the given level.
func toastColors(level ToastLevel) (bg paint.Color, text paint.Color) {
	text = paint.Color{255, 255, 255, 255}
	switch level {
	case ToastInfo:
		bg = paint.Color{33, 150, 243, 240} // blue
	case ToastSuccess:
		bg = paint.Color{76, 175, 80, 240} // green
	case ToastWarning:
		bg = paint.Color{255, 152, 0, 240} // amber
		text = paint.Color{50, 50, 50, 255}
	case ToastError:
		bg = paint.Color{244, 67, 54, 240} // red
	default:
		bg = paint.Color{66, 66, 66, 240}
	}
	return
}

// ToastManager manages active toast notifications. It is a global singleton.
type ToastManager struct {
	mu     sync.Mutex
	toasts []*toastEntry
}

var globalToastManager = &ToastManager{}

// GetToastManager returns the global toast manager singleton.
func GetToastManager() *ToastManager {
	return globalToastManager
}

// ShowToast displays a temporary toast notification message at the top center
// of the parent widget's window. The toast auto-dismisses after the given
// number of milliseconds.
func ShowToast(parent IWidget, message string, durationMs uint32, level ToastLevel) {
	mgr := GetToastManager()
	mgr.show(parent, message, durationMs, level)
}

func (mgr *ToastManager) show(parent IWidget, message string, durationMs uint32, level ToastLevel) {
	entry := new(toastEntry)
	entry.Init(entry)
	entry.message = message
	entry.level = level
	entry.durationMs = durationMs
	entry.parent = parent

	mgr.mu.Lock()
	mgr.toasts = append(mgr.toasts, entry)
	mgr.mu.Unlock()

	entry.SetParent(parent)
	mgr.layoutToasts(parent)
	entry.AttachWindow(WtPopup)
	entry.SetVisible(true)

	// Use a Timer to dismiss on the UI thread
	entry.timer.Start(durationMs, func() {
		entry.timer.Stop()
		mgr.dismiss(entry)
	})
}

func (mgr *ToastManager) dismiss(entry *toastEntry) {
	mgr.mu.Lock()
	found := false
	for i, t := range mgr.toasts {
		if t == entry {
			mgr.toasts = append(mgr.toasts[:i], mgr.toasts[i+1:]...)
			found = true
			break
		}
	}
	parent := entry.parent
	mgr.mu.Unlock()

	if found {
		entry.timer.Stop()
		entry.SetVisible(false)
		entry.DetachWindow()
		entry.SetParent(nil)
	}

	// Re-layout remaining toasts
	if parent != nil {
		mgr.layoutToasts(parent)
	}
}

func (mgr *ToastManager) layoutToasts(parent IWidget) {
	mgr.mu.Lock()
	toasts := make([]*toastEntry, len(mgr.toasts))
	copy(toasts, mgr.toasts)
	mgr.mu.Unlock()

	if len(toasts) == 0 {
		return
	}

	// Position toasts at the top-center of the owner window
	win := parent.OwnerWindow()
	if win == nil {
		return
	}
	winW, _ := win.Widget().Size()
	toastW := 320.0
	startX := (winW - toastW) * 0.5
	startY := 12.0

	for _, t := range toasts {
		gx, gy := win.Widget().MapToGlobal(startX, startY)
		t.SetPos(gx, gy)
		t.SetSize(toastW, 48)
		startY += 56
	}
}

// toastAccentColor returns a brighter accent color for the left bar.
func toastAccentColor(level ToastLevel) paint.Color {
	switch level {
	case ToastInfo:
		return paint.Color{66, 165, 245, 255} // lighter blue
	case ToastSuccess:
		return paint.Color{102, 187, 106, 255} // lighter green
	case ToastWarning:
		return paint.Color{255, 183, 77, 255} // lighter amber
	case ToastError:
		return paint.Color{239, 83, 80, 255} // lighter red
	default:
		return paint.Color{120, 120, 120, 255}
	}
}

// --- Toast Drawing ---

func (this *toastEntry) Draw(g paint.Painter) {
	w, h := this.Size()
	bg, textColor := toastColors(this.level)
	accent := toastAccentColor(this.level)

	r := 6.0

	paint.DrawShadowRect(g, 0, 0, w, h, r, 5, paint.Color{0, 0, 0, 100})

	// Rounded rectangle background
	roundedRect(g, 0, 0, w, h, r)
	g.SetBrush1(bg)
	g.Fill()

	// Colored left accent bar (4px wide, with rounded left corners)
	barW := 4.0
	roundedRect(g, 0, 0, barW+r, h, r)
	g.SetBrush1(accent)
	g.Fill()
	// Fill the right part of the bar area with main bg to keep only left rounded
	g.Rectangle(barW, 0, r, h)
	g.SetBrush1(bg)
	g.Fill()

	// Level icon (simple shapes)
	iconX := barW + 14.0
	iconY := h * 0.5
	switch this.level {
	case ToastInfo:
		// "i" circle
		g.Arc(iconX, iconY, 8, 0, 2*math.Pi)
		g.SetBrush1(paint.Color{255, 255, 255, 60})
		g.Fill()
		// letter "i"
		g.Arc(iconX, iconY-3, 1.5, 0, 2*math.Pi)
		g.SetBrush1(textColor)
		g.Fill()
		g.MoveTo(iconX, iconY)
		g.LineTo(iconX, iconY+4)
		g.SetPen1(textColor, 2)
		g.Stroke()
	case ToastSuccess:
		// checkmark
		g.MoveTo(iconX-5, iconY)
		g.LineTo(iconX-1, iconY+4)
		g.LineTo(iconX+6, iconY-4)
		g.SetPen1(textColor, 2.5)
		g.Stroke()
	case ToastWarning:
		// triangle with exclamation
		g.MoveTo(iconX, iconY-6)
		g.LineTo(iconX+7, iconY+5)
		g.LineTo(iconX-7, iconY+5)
		g.LineTo(iconX, iconY-6)
		g.SetPen1(textColor, 1.5)
		g.Stroke()
		// exclamation dot
		g.MoveTo(iconX, iconY-2)
		g.LineTo(iconX, iconY+1)
		g.SetPen1(textColor, 2)
		g.Stroke()
		g.Arc(iconX, iconY+3, 1, 0, 2*math.Pi)
		g.SetBrush1(textColor)
		g.Fill()
	case ToastError:
		// circle with X
		g.Arc(iconX, iconY, 8, 0, 2*math.Pi)
		g.SetBrush1(paint.Color{255, 255, 255, 40})
		g.Fill()
		g.MoveTo(iconX-4, iconY-4)
		g.LineTo(iconX+4, iconY+4)
		g.MoveTo(iconX+4, iconY-4)
		g.LineTo(iconX-4, iconY+4)
		g.SetPen1(textColor, 2)
		g.Stroke()
	}

	// Message text
	t := Theme()
	msgFont := paint.NewFont(t.Font.Family(), 13, false, false)
	g.SetFont(msgFont)
	g.SetBrush1(textColor)
	ext := msgFont.TextExtents(this.message)
	tx := barW + 32.0 - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx, ty)
	g.DrawText(this.message)
	g.Translate(-tx, -ty)

	// Close button (rounded X) at right side
	closeX := w - 20.0
	closeY := h * 0.5
	// Hover circle background for close btn
	g.Arc(closeX, closeY, 10, 0, 2*math.Pi)
	g.SetBrush1(paint.Color{255, 255, 255, 25})
	g.Fill()
	g.MoveTo(closeX-3.5, closeY-3.5)
	g.LineTo(closeX+3.5, closeY+3.5)
	g.MoveTo(closeX+3.5, closeY-3.5)
	g.LineTo(closeX-3.5, closeY+3.5)
	g.SetPen1(textColor, 1.5)
	g.Stroke()
}

func (this *toastEntry) OnLeftDown(x, y float64) {
	w, h := this.Size()
	// Check if close button was clicked
	closeX := w - 20.0
	closeY := h * 0.5
	if x >= closeX-10 && x <= closeX+10 && y >= closeY-10 && y <= closeY+10 {
		globalToastManager.dismiss(this)
		return
	}
	_ = h
}

func (this *toastEntry) SizeHints() SizeHints {
	return SizeHints{Width: 320, Height: 48, Policy: 0}
}
