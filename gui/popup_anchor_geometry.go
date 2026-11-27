package gui

import "slices"

// An anchored popup is already represented in the capture stack. Reusing that
// lifetime avoids a second registry that could retain destroyed windows.
type popupAnchorButton interface {
	IWidget
	IButton
}

type buttonPopupPlacement struct {
	popup         IWidget
	anchor        popupAnchorButton
	window, owner *Window
}

// Widget geometry, capture, and this batch are owned by the UI thread.
var pendingButtonPopups []buttonPopupPlacement

func buttonPopupAnchor(popup IWidget) popupAnchorButton {
	if popup == nil || !popup.IsPopup() || !popup.IsVisible() {
		return nil
	}
	p, ok := popup.Parent().(popupAnchorButton)
	if !ok || p.SubPopup() != popup {
		return nil
	}
	return p
}

func affectsPopupAnchor(changed *Widget, anchor IWidget) bool {
	for p := anchor; p != nil; p = p.Parent() {
		if p.NakedWidget() == changed {
			return true
		}
		if p.Window() != nil {
			return false
		}
	}
	return false
}

func queueButtonPopupPlacement(changed *Widget) {
	for _, popup := range captureStack {
		anchor := buttonPopupAnchor(popup)
		if anchor == nil || !affectsPopupAnchor(changed, anchor) {
			continue
		}
		update := buttonPopupPlacement{popup, anchor, popup.Window(), anchor.OwnerWindow()}
		if slices.Contains(pendingButtonPopups, update) {
			continue
		}
		pendingButtonPopups = append(pendingButtonPopups, update)
		if len(pendingButtonPopups) == 1 {
			Post(placeQueuedButtonPopups)
		}
	}
}

func (p buttonPopupPlacement) current() bool {
	return buttonPopupAnchor(p.popup) == p.anchor && p.popup.Window() == p.window &&
		p.anchor.OwnerWindow() == p.owner && WidgetTreeVisible(p.anchor) &&
		slices.Contains(captureStack, p.popup)
}

func placeQueuedButtonPopups() {
	batch := pendingButtonPopups
	pendingButtonPopups = nil
	for _, update := range batch {
		if update.current() {
			layoutButtonPopup(update.anchor, update.popup, update.current)
		}
	}
	clear(batch)
	if len(pendingButtonPopups) == 0 && cap(batch) <= 16 {
		pendingButtonPopups = batch[:0]
	}
}

func layoutButtonPopup(anchor popupAnchorButton, popup IWidget, stillOpen func() bool) {
	x, y := anchor.MapToGlobal(0, 0)
	w, h := anchor.Size()
	vertical := !anchor.IsInPopupMenu()
	overlap := 3.0
	if vertical {
		overlap = 1
	}
	layoutPopup(popup, x, y, w, h, vertical, overlap, stillOpen)
}

// A mapped button popup can itself host a captured submenu. A move within
// that family should re-position the child, not retire it. All other native
// surfaces retain the existing geometry-change dismissal behavior.
func (win *Window) retirePopupsForGeometryChange() {
	if buttonPopupAnchor(win.widget) == nil || !slices.Contains(captureStack, win.widget) {
		win.dismissOwnedPopups()
	}
}

// The mapped screen origin of a popup can change while its local coordinates
// relative to the anchor stay constant. Still notify any nested anchors.
func (win *Window) queuePopupSurfacePlacement() {
	if win.wt == WtPopup && win.widget != nil {
		queueButtonPopupPlacement(win.widget.NakedWidget())
	}
}
