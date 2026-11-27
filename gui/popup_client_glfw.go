//go:build !windows

package gui

import (
	"slices"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// clientPopupHost is always a native window, while popupOwner is the immediate
// logical owner so nested menus keep the usual focus and capture hierarchy.
func (win *Window) configureClientPopup(owner *Window) error {
	if owner == nil {
		// A context menu can have no widget parent. Select its actual
		// active window, not an arbitrary window from the registry.
		if focusWidget != nil {
			owner = focusWidget.OwnerWindow()
			if owner == win || owner == nil || !owner.IsVisible() {
				owner = nil
			}
		}
		if owner == nil && pointerWindow != nil && pointerWindow.IsVisible() {
			owner = pointerWindow
		}
		if owner == nil {
			owner = activeForm
		}
	}
	if owner == nil || owner == win {
		return core.StrErr("gui: popup requires a visible Wayland window")
	}
	host := owner.clientPopupHost
	if host == nil {
		host = owner
	}
	if host == win || host.glfwWin == nil {
		return core.StrErr("gui: popup requires a native Wayland owner")
	}
	win.wt, win.enabled = WtPopup, true
	win.clientPopupHost, win.popupOwner = host, owner
	win.popupRestoreFocus = focusTargetForOwner(owner, focusWidget)
	return nil
}

func (win *Window) refreshClientPopupOwner() {
	if parent := win.widget.Parent(); parent != nil {
		owner := parent.OwnerWindow()
		if owner != nil && owner != win && owner != win.popupOwner {
			oldHost := win.clientPopupHost
			win.unlinkClientPopup()
			if err := win.configureClientPopup(owner); err != nil {
				win.clientPopupHost, win.popupOwner = oldHost, nil
			}
		}
	}
}

// Used by shared layout so menus/lists fit the client area, not the global
// monitor. Wayland does not give a global coordinate origin to the client.
func (win *Window) clientPopupWorkArea() (geom.Rect, bool) {
	if win.clientPopupHost == nil || win.widget == nil {
		return geom.Rect{}, false
	}
	win.refreshClientPopupOwner()
	host := win.clientPopupHost
	if host.glfwWin == nil {
		return geom.Rect{}, false
	}
	w, h := host.glfwWin.GetSize()
	area := geom.Rect{Width: float64(w), Height: float64(h)}
	return area, validPopupArea(area)
}

func (win *Window) setClientPopupPosition(x, y float64) {
	win.refreshClientPopupOwner()
	if !finite(x) || !finite(y) {
		return
	}
	moved := win.clientPopupX != x || win.clientPopupY != y
	win.clientPopupX, win.clientPopupY = x, y
	if parent := win.widget.Parent(); parent != nil {
		x, y = parent.MapFromGlobal(x, y)
	}
	win.widget.NakedWidget().setPos(x, y)
	if moved {
		win.queuePopupSurfacePlacement()
	}
	if win.clientPopupVisible {
		win.clientPopupHost.MarkFullDirty()
	}
}

func (win *Window) setClientPopupVisible(show bool) {
	if show {
		win.refreshClientPopupOwner()
		host := win.clientPopupHost
		if win.clientPopupVisible || host == nil || !host.IsVisible() ||
			win.popupOwner == nil || !win.popupOwner.IsVisible() || !host.IsEnabled() {
			return
		}
		win.popupRestoreFocus = focusTargetForOwner(win.popupOwner, focusWidget)
		win.clientPopupVisible = true
		host.clientPopupLayers = append(host.clientPopupLayers, win)
		host.MarkFullDirty()
		return
	}
	if !win.clientPopupVisible {
		return
	}
	win.dismissOwnedPopups()
	win.unlinkClientPopup()
	if win.widget != nil {
		p := map[IWidget]bool{win.widget: true}
		clearPopupInput(p, focusTargetForOwner(win.popupOwner, win.popupRestoreFocus), widgetWithinPopups(focusWidget, p))
	}
	if win.closeOnHide {
		win.destroy()
	}
}

func (win *Window) unlinkClientPopup() {
	if host := win.clientPopupHost; host != nil && win.clientPopupVisible {
		host.clientPopupLayers = slices.DeleteFunc(host.clientPopupLayers, func(candidate *Window) bool {
			return candidate == win
		})
		win.clientPopupVisible = false
		host.MarkFullDirty()
	}
}

func (win *Window) drawClientPopups(g paint.Painter, width, height float64) {
	if len(win.clientPopupLayers) == 0 {
		return
	}
	// A custom drawing callback can hide or replace another popup. Keep
	// iteration stable, and check attachment again before every draw.
	for _, layer := range slices.Clone(win.clientPopupLayers) {
		if layer.clientPopupHost != win || !layer.clientPopupVisible || layer.widget == nil || !layer.widget.IsVisible() {
			continue
		}
		x, y := layer.clientPopupX, layer.clientPopupY
		pw, ph := layer.widget.Size()
		cx, cy, cw, ch := intersectRect(0, 0, width, height, x, y, pw, ph)
		// DrawWidgetAll clips before applying the layer translation.
		DrawWidgetAll(layer.widget, g, x, y, cx, cy, cw, ch)
		if tip := ownedToolTipFor(layer); tip != nil {
			g.Save()
			g.Translate(x, y)
			tip.draw(g, geom.Rect{X: cx - x, Y: cy - y, Width: cw, Height: ch})
			g.Restore()
		}
	}
}

// Return a popup for a client-local hit only when it shares this form.
// Tooltip overlays never enter this interactive layer list.
func (win *Window) clientPopupAt(x, y float64) *Window {
	for i := len(win.clientPopupLayers) - 1; i >= 0; i-- {
		p := win.clientPopupLayers[i]
		if p.clientPopupHost != win || !p.clientPopupVisible || p.widget == nil || !p.widget.IsVisible() || !p.IsEnabled() {
			continue
		}
		px, py := x-p.clientPopupX, y-p.clientPopupY
		if pw, ph := p.widget.Size(); px >= 0 && py >= 0 && px < pw && py < ph {
			return p
		}
	}
	return nil
}

func (win *Window) clientPopupHit(x, y float64) IWidget {
	if popup := win.clientPopupAt(x, y); popup != nil {
		return popup.widget.FindWidgetAt(x-popup.clientPopupX, y-popup.clientPopupY)
	}
	return nil
}
