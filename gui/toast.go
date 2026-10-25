package gui

import (
	"github.com/uk0/silk/paint"
	"math"
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
	manager    *ToastManager
	position   ToastPosition
	onActivate func()
}

// Notifications belong to the owner for coordinate mapping and repainting,
// but are deliberately absent from Children(): layouts, focus traversal and
// design serialization must not treat an ephemeral banner as user content.
func (entry *toastEntry) Parent() IWidget { return entry.parent }

// toastColors returns background and text colors for the given level.
func toastColors(level ToastLevel) (bg paint.Color, text paint.Color) {
	return toastSurfaceColors(level)
}

// ToastManager manages active toast notifications. It is a global singleton.
type ToastManager struct {
	toasts []*toastEntry
}

var globalToastManager = &ToastManager{}

// GetToastManager returns the global toast manager singleton.
func GetToastManager() *ToastManager {
	return globalToastManager
}

// ShowToast displays a temporary toast notification message at the top center
// of the parent widget's window. The toast auto-dismisses after the given
// number of milliseconds. Like other widget operations, call on the UI thread
// (use Post from workers). It never opens or focuses a native popup window.
func ShowToast(parent IWidget, message string, durationMs uint32, level ToastLevel) {
	mgr := GetToastManager()
	mgr.show(parent, message, durationMs, level)
}

func (mgr *ToastManager) show(parent IWidget, message string, durationMs uint32, level ToastLevel) {
	mgr.showWithOptions(parent, message, ToastOptions{DurationMs: durationMs, Level: level})
}

func (mgr *ToastManager) showWithOptions(parent IWidget, message string, options ToastOptions) *Toast {
	if parent == nil {
		return nil
	}
	owner := parent.RootWidget()
	if win := parent.OwnerWindow(); win != nil {
		owner = win.Widget()
	}
	if owner == nil || !owner.IsVisible() {
		return nil
	}
	if existing := owner.NakedWidget().toastManager; existing != nil && existing != mgr {
		return existing.showWithOptions(parent, message, options)
	}
	// Bounded, owner-isolated expiry work even for a burst of worker completions.
	count := 0
	var oldest *toastEntry
	for _, entry := range mgr.toasts {
		if entry.parent == owner {
			count++
			if oldest == nil {
				oldest = entry
			}
		}
	}
	if count >= 8 {
		mgr.dismiss(oldest)
	}
	owner.NakedWidget().toastManager = mgr
	entry := new(toastEntry)
	entry.Init(entry)
	entry.SetFocusPolicy(NoFocus)
	entry.message, entry.level, entry.durationMs = message, options.Level, options.DurationMs
	entry.position, entry.onActivate = options.Position, options.OnActivate
	entry.parent, entry.manager = owner, mgr
	mgr.toasts = append(mgr.toasts, entry)
	mgr.layoutToasts(owner)
	entry.timer.Start(options.DurationMs, func() { mgr.dismiss(entry) })
	return &Toast{entry: entry}
}

func (mgr *ToastManager) dismiss(entry *toastEntry) {
	if entry == nil || entry.manager != mgr {
		return
	}
	for i, t := range mgr.toasts {
		if t == entry {
			copy(mgr.toasts[i:], mgr.toasts[i+1:])
			mgr.toasts[len(mgr.toasts)-1] = nil
			mgr.toasts = mgr.toasts[:len(mgr.toasts)-1]
			break
		}
	}
	parent := entry.parent
	entry.timer.Stop()
	entry.SetVisible(false)
	entry.parent = nil
	entry.manager = nil
	entry.onActivate = nil
	entry.message = ""
	if parent != nil {
		mgr.layoutToasts(parent)
	}
}

func (mgr *ToastManager) layoutToasts(parent IWidget) {
	if parent == nil {
		return
	}
	w, h := parent.Size()
	left, top, right, bottom := 0.0, 0.0, math.Max(0, w), math.Max(0, h)
	if frame, ok := parent.(*Frame); ok && frame.root_ != nil {
		// Exclude menu/toolbars, sidebar and status bar for both placements.
		x, y, cw, ch := frame.root_.Bounds()
		left, top = math.Max(0, x), math.Max(0, y)
		right, bottom = math.Min(right, x+math.Max(0, cw)), math.Min(bottom, y+math.Max(0, ch))
	}
	lowerHalf := top + (bottom-top)*.5
	width := math.Max(0, right-left)
	margin := math.Min(12, width*.05)
	toastW := math.Min(320, math.Max(0, width-2*margin))
	top, bottom = top+12, bottom-12
	count := 0
	// Newest first. Opposite placements share the available strip so that
	// mixed top/bottom stacks cannot overlap each other in a small owner.
	for i := len(mgr.toasts) - 1; i >= 0; i-- {
		t := mgr.toasts[i]
		if t.parent != parent {
			continue
		}
		count++
		fits := toastW >= 24 && top+48 <= bottom
		if t.position == ToastBottomRight {
			fits = fits && bottom-48 >= lowerHalf
		}
		t.SetVisible(fits)
		if !fits {
			continue
		}
		x, y := left+(width-toastW)*.5, top
		if t.position == ToastBottomRight {
			x, y = right-margin-toastW, bottom-48
			bottom = y - 8
		} else {
			top += 56
		}
		t.SetBounds(x, y, toastW, 48)
	}

	if count == 0 {
		parent.NakedWidget().toastManager = nil
	}
	parent.Update()
}

// toastAccentColor uses the same precise semantic colors as the IDE icons.
func toastAccentColor(level ToastLevel) paint.Color {
	return toastSemanticColor(level)
}

// --- Toast Drawing ---

func (this *toastEntry) Draw(g paint.Painter) {
	w, h := this.Size()
	bg, textColor := toastColors(this.level)
	accent := toastAccentColor(this.level)
	iconBG := accent
	iconBG.A = 28

	r := 6.0

	// Rounded rectangle background
	roundedRect(g, 0, 0, w, h, r)
	g.SetBrush1(bg)
	g.Fill()
	roundedRect(g, .5, .5, w-1, h-1, r)
	g.SetPen1(Theme().BorderColor, 1)
	g.Stroke()

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
	if w >= 76 {
		switch this.level {
		case ToastInfo:
			// "i" circle
			g.Arc(iconX, iconY, 8, 0, 2*math.Pi)
			g.SetBrush1(iconBG)
			g.Fill()
			// letter "i"
			g.Arc(iconX, iconY-3, 1.5, 0, 2*math.Pi)
			g.SetBrush1(accent)
			g.Fill()
			g.MoveTo(iconX, iconY)
			g.LineTo(iconX, iconY+4)
			g.SetPen1(accent, 2)
			g.Stroke()
		case ToastSuccess:
			// checkmark
			g.MoveTo(iconX-5, iconY)
			g.LineTo(iconX-1, iconY+4)
			g.LineTo(iconX+6, iconY-4)
			g.SetPen1(accent, 2.5)
			g.Stroke()
		case ToastWarning:
			// triangle with exclamation
			g.MoveTo(iconX, iconY-6)
			g.LineTo(iconX+7, iconY+5)
			g.LineTo(iconX-7, iconY+5)
			g.LineTo(iconX, iconY-6)
			g.SetPen1(accent, 1.5)
			g.Stroke()
			// exclamation dot
			g.MoveTo(iconX, iconY-2)
			g.LineTo(iconX, iconY+1)
			g.SetPen1(accent, 2)
			g.Stroke()
			g.Arc(iconX, iconY+3, 1, 0, 2*math.Pi)
			g.SetBrush1(accent)
			g.Fill()
		case ToastError:
			// circle with X
			g.Arc(iconX, iconY, 8, 0, 2*math.Pi)
			g.SetBrush1(iconBG)
			g.Fill()
			g.MoveTo(iconX-4, iconY-4)
			g.LineTo(iconX+4, iconY+4)
			g.MoveTo(iconX+4, iconY-4)
			g.LineTo(iconX-4, iconY+4)
			g.SetPen1(accent, 2)
			g.Stroke()
		}
	}

	// Message text
	t := Theme()
	msgFont := paint.NewFont(t.Font.Family(), 13, false, false)
	g.SetFont(msgFont)
	g.SetBrush1(textColor)
	// Cut the message before the close button: it is painted after the text at
	// w-20 and takes clicks from w-32 on, so an unelided message runs under the
	// ✕ and gets sliced mid-glyph by the widget clip.
	textLeft := barW + 32.0
	if w < 76 {
		textLeft = 10
	}
	msg := elideToWidth(msgFont, notificationPreview(this.message), w-40.0-textLeft)
	ext := msgFont.TextExtents(msg)
	tx := textLeft - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx, ty)
	g.DrawText(msg)
	g.Translate(-tx, -ty)

	// Close button (rounded X) at right side
	closeX := toastCloseX(w)
	closeY := h * 0.5
	// Hover circle background for close btn
	g.Arc(closeX, closeY, 10, 0, 2*math.Pi)
	closeBG := textColor
	closeBG.A = 20
	g.SetBrush1(closeBG)
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
	closeX := toastCloseX(w)
	closeY := h * 0.5
	if x >= 0 && x < w && y >= 0 && y < h &&
		x >= closeX-12 && x <= closeX+12 && y >= closeY-12 && y <= closeY+12 && this.manager != nil {
		this.manager.dismiss(this)
		return
	}
	if x >= 0 && x < w && y >= 0 && y < h && this.manager != nil && this.onActivate != nil {
		activate := this.onActivate
		this.manager.dismiss(this)
		activate()
	}
}

func toastCloseX(width float64) float64 { return math.Max(12, width-20) }

func (this *toastEntry) SizeHints() SizeHints {
	return SizeHints{Width: 320, Height: 48, Policy: 0}
}
