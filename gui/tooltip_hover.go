package gui

import (
	"math"
	"sync/atomic"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// Automatic and explicitly anchored hints are passive ink in their owner's
// client window. Neither creates a surface, affects focus/capture, joins
// layout, or participates in hit-testing.
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
	// The explicit hint is immutable after publication. Readers only consult
	// it for visibility; its damage notification still runs on the UI thread.
	currentTooltip            atomic.Pointer[hoverToolTipOverlay]
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
	if tip.win != nil && tip.win.IsValid() {
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

func dismissOwnedToolTipsIn(root IWidget) {
	if current := visibleHoverToolTip; current != nil && toolTipAncestorOf(root, current.owner) {
		dismissRegisteredToolTip(current.win)
	}
	dismissExplicitToolTip(func(tip *hoverToolTipOverlay) bool {
		for p := tip.owner; p != nil; p = p.Parent() {
			if p == root {
				return true
			}
		}
		return false
	})
}

func dismissOwnedToolTips(win *Window) {
	dismissRegisteredToolTip(win)
	dismissExplicitToolTip(func(tip *hoverToolTipOverlay) bool { return tip.win == win })
}

func replaceExplicitToolTip(next *hoverToolTipOverlay) {
	old := currentTooltip.Swap(next)
	if old != nil {
		old.invalidate()
	}
	if next != nil {
		next.invalidate()
	}
}

func dismissExplicitToolTip(match func(*hoverToolTipOverlay) bool) {
	if tip := currentTooltip.Load(); tip != nil && match(tip) && currentTooltip.CompareAndSwap(tip, nil) {
		tip.invalidate()
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
	// control hint, even when the pointer is over an unrelated registration.
	if currentTooltip.Load() != nil {
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

	current := newOwnedToolTip(anchor, owner, text)
	if current == nil {
		return
	}
	unpaintRegisteredToolTip()
	visibleHoverToolTip = current
	activeHoverToolTipOwner.Store(owner.NakedWidget())
	current.invalidate()
	// A worker could change the registration between lookup and publication.
	// Rechecking after Store closes that race without touching GUI state off-thread.
	if GetToolTip(owner) != text {
		queueRegisteredToolTipRefresh()
	}
}

func newOwnedToolTip(anchor hoverToolTipAnchor, owner IWidget, text string) *hoverToolTipOverlay {
	w, h := anchor.win.Widget().Size()
	if w <= 0 || h <= 0 || text == "" {
		return nil
	}
	label := newTooltipWindow(text)
	bounds := registeredToolTipBounds(w, h, anchor.x, anchor.y, label.SizeHints())
	if !validPopupArea(bounds) {
		return nil
	}
	label.NakedWidget().w, label.NakedWidget().h = bounds.Width, bounds.Height
	return &hoverToolTipOverlay{anchor, owner, label, bounds}
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

// Select the single visible hint without invoking widget code under the lock.
func ownedToolTipFor(win *Window) *hoverToolTipOverlay {
	tip := currentTooltip.Load()
	if tip == nil {
		tip = visibleHoverToolTip
	}
	if tip == nil || tip.win != win {
		return nil
	}
	return tip
}

// Called once in the native paint pass, after the widget tree. The damage
// rect is client-local, including on the partial Win32 back-buffer path.
func drawOwnedToolTip(win *Window, g paint.Painter, damage geom.Rect) {
	if tip := ownedToolTipFor(win); tip != nil {
		tip.draw(g, damage)
	}
}

func (tip *hoverToolTipOverlay) draw(g paint.Painter, damage geom.Rect) {
	if !tip.owner.IsAllAncentorsVisible() || tip.owner.OwnerWindow() != tip.win {
		return
	}
	r := tip.bounds
	x, y, w, h := intersectRect(r.X, r.Y, r.Width, r.Height, damage.X, damage.Y, damage.Width, damage.Height)
	if w > 0 && h > 0 {
		DrawWidgetAll(tip.label, g, r.X, r.Y, x, y, w, h)
	}
}
