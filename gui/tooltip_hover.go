package gui

import (
	"math"
	"sync/atomic"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// Registered hints are passive ink in the window that received the pointer
// event. Unlike an explicit ShowToolTipFor popup, they don't create a surface,
// affect focus/capture, join layout, or participate in hit-testing.
type hoverToolTipAnchor struct {
	win  *Window
	x, y float64 // actual client-local coordinates, also on Wayland
}

type hoverToolTipOverlay struct {
	hoverToolTipAnchor
	owner  IWidget
	label  *tooltipWindow
	bounds geom.Rect
}

var (
	// Input, paint and lifetime state is owned by the UI thread. Only the
	// owner identity is published for off-thread tooltip registration.
	pendingHoverToolTip       hoverToolTipAnchor
	visibleHoverToolTip       *hoverToolTipOverlay
	activeHoverToolTipOwner   atomic.Pointer[Widget]
	hoverToolTipRefreshQueued atomic.Bool
)

func trackRegisteredToolTip(win *Window, hit IWidget, x, y float64) {
	if hit == nil || curCapture() != nil {
		dismissRegisteredToolTip(nil)
		return
	}
	anchor := hoverToolTipAnchor{win, x, y}
	if current := visibleHoverToolTip; current != nil {
		if current.win != win || !toolTipAncestorOf(current.owner, hit) {
			unpaintRegisteredToolTip()
		} else {
			current.hoverToolTipAnchor = anchor
		}
	}
	pendingHoverToolTip = anchor
}

func toolTipAncestorOf(ancestor, child IWidget) bool {
	for p := child; p != nil; p = p.Parent() {
		if p == ancestor {
			return true
		}
		if p.Window() != nil {
			break
		}
	}
	return false
}

func unpaintRegisteredToolTip() {
	if current := visibleHoverToolTip; current != nil {
		visibleHoverToolTip = nil
		activeHoverToolTipOwner.Store(nil)
		current.invalidate()
	}
}

func (tip *hoverToolTipOverlay) invalidate() {
	if tip.win.IsValid() {
		r := tip.bounds
		// Include a pixel on each side for antialiased strokes and the
		// conversion of fractional coordinates into a native damage region.
		tip.win.MarkDirtyRect(r.X-1, r.Y-1, r.Width+2, r.Height+2)
	}
}

// A nil window cancels both pending and visible hints across the UI. A late
// leave/focus callback from another window must not clear the new owner.
func dismissRegisteredToolTip(win *Window) {
	if win == nil || pendingHoverToolTip.win == win {
		pendingHoverToolTip = hoverToolTipAnchor{}
	}
	if visibleHoverToolTip != nil && (win == nil || visibleHoverToolTip.win == win) {
		unpaintRegisteredToolTip()
	}
}

func dismissRegisteredToolTipIn(root IWidget) {
	if current := visibleHoverToolTip; current != nil && toolTipAncestorOf(root, current.owner) {
		dismissRegisteredToolTip(current.win)
	}
}

func settleRegisteredToolTip() {
	anchor := pendingHoverToolTip
	pendingHoverToolTip = hoverToolTipAnchor{}
	if anchor.win == nil || curCapture() != nil || IsMouseLeftDown() || IsMouseRightDown() || mouseMiddleDown() {
		return
	}
	paintRegisteredToolTip(anchor)
}

func paintRegisteredToolTip(anchor hoverToolTipAnchor) {
	win := anchor.win
	if !win.IsValid() || !win.IsVisible() || !win.IsEnabled() || win.Widget() == nil || !pointerStillInTooltipWindow(anchor) {
		return
	}
	root := win.Widget()
	hit := root.FindWidgetAt(anchor.x, anchor.y)
	if hit == nil || hit != mouseHoverWidget || !hit.IsAllAncentorsVisible() {
		return
	}

	// Prefer explicit programmatic hints (e.g. LSP results) to an automatic
	// control hint. Their existing owner/placement API remains unchanged.
	tooltipMu.Lock()
	manualHintVisible := currentTooltip != nil
	tooltipMu.Unlock()
	if manualHintVisible {
		return
	}
	owner, text := registeredToolTip(hit)
	if owner == nil {
		unpaintRegisteredToolTip()
		return
	}
	if current := visibleHoverToolTip; current != nil &&
		current.win == win && current.owner == owner && current.label.text == text {
		return // motion inside one control never restarts a visible hint
	}

	w, h := root.Size()
	if w <= 0 || h <= 0 {
		return
	}
	label := newTooltipWindow(text)
	size := label.SizeHints()
	bounds := registeredToolTipBounds(w, h, anchor.x, anchor.y, size)
	if !validPopupArea(bounds) {
		return
	}
	label.NakedWidget().w, label.NakedWidget().h = bounds.Width, bounds.Height
	unpaintRegisteredToolTip()
	current := &hoverToolTipOverlay{anchor, owner, label, bounds}
	visibleHoverToolTip = current
	activeHoverToolTipOwner.Store(owner.NakedWidget())
	current.invalidate()
	// A worker could change the registration between lookup and publication.
	// Rechecking after Store closes that race without touching GUI state off-thread.
	if GetToolTip(owner) != text {
		queueRegisteredToolTipRefresh()
	}
}

func registeredToolTipBounds(width, height, x, y float64, hint SizeHints) geom.Rect {
	area := geom.Rect{Width: width, Height: height}
	w, h := math.Min(width, math.Ceil(hint.Width)), math.Min(height, math.Ceil(hint.Height))
	x, y = tooltipPositionInWorkArea(area, x, y, w, h)
	return geom.Rect{X: x, Y: y, Width: w, Height: h}
}

func queueRegisteredToolTipRefresh() {
	if hoverToolTipRefreshQueued.CompareAndSwap(false, true) {
		Post(refreshRegisteredToolTip)
	}
}

// SetToolTip is also a safe registration API for workers. Recheck the
// current owner on the UI queue; a late update cannot resurrect a retired hint.
func refreshRegisteredToolTip() {
	hoverToolTipRefreshQueued.Store(false)
	if current := visibleHoverToolTip; current != nil {
		if GetToolTip(current.owner) == current.label.text {
			return // retired or already-current UI does not need another frame
		}
		anchor := current.hoverToolTipAnchor
		unpaintRegisteredToolTip()
		paintRegisteredToolTip(anchor)
	}
}

// Called once in the native paint pass, after the widget tree. The damage
// rect is client-local, including on the partial Win32 back-buffer path.
func drawRegisteredToolTip(win *Window, g paint.Painter, damage geom.Rect) {
	current := visibleHoverToolTip
	if current == nil || current.win != win ||
		!current.owner.IsAllAncentorsVisible() || current.owner.OwnerWindow() != win {
		return
	}
	r := current.bounds
	x, y, w, h := intersectRect(r.X, r.Y, r.Width, r.Height, damage.X, damage.Y, damage.Width, damage.Height)
	if w > 0 && h > 0 {
		DrawWidgetAll(current.label, g, r.X, r.Y, x, y, w, h)
	}
}
