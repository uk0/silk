package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// SetRetainClosedViews makes user-close hide tool tabs without disposing their
// configured instances. Document docks always retain normal save/close behavior.
// Explicit CloseView/CloseAllViews still dispose hidden and visible instances.
// Like all dock operations, call these methods only on the UI thread.
func (d *Dock) SetRetainClosedViews(on bool) { d.retainClosedViews = on }

// HasView includes temporarily hidden tool tabs owned by this dock.
func (d *Dock) HasView(view IWidget) bool {
	if view == nil {
		return false
	}
	if d.IndexOfView(view) >= 0 {
		return true
	}
	_, ok := d.hiddenViews[view]
	return ok
}

// HideView retains a tool's state, parent, callbacks and position until reveal.
// It is deliberately opt-in; documents must go through the save/close protocol.
func (d *Dock) HideView(view IWidget) bool {
	if !d.retainClosedViews || d.isMainDock {
		return false
	}
	idx := d.IndexOfView(view)
	if idx < 0 {
		_, ok := d.hiddenViews[view]
		return ok
	}
	if d.hiddenViews == nil {
		d.hiddenViews = make(map[IWidget]int)
	}
	d.hiddenViews[view] = idx
	view.SetVisible(false)
	d.tabbar.RemoveTab(idx)
	if d.ViewCount() == 0 {
		d.SetVisible(false)
	}
	d.Layout()
	d.focusVisibleView()
	if f := d.Frame(); f != nil {
		f.Layout()
	}
	return true
}

// RevealView selects an owned tab, restoring hidden/minimized panels and leaving
// a different panel's maximized layout. It never attaches an unrelated widget.
func (d *Dock) RevealView(view IWidget) bool {
	if !d.HasView(view) {
		return false
	}
	if idx, ok := d.hiddenViews[view]; ok {
		delete(d.hiddenViews, view)
		d.tabbar.InsertTab(min(idx, d.ViewCount()), view, true)
	}
	if f := d.Frame(); f != nil && f.maximizedDock != d {
		f.RestoreDockLayout()
	}
	d.isMin = false
	d.SetVisible(true)
	d.SetActiveIndex(d.IndexOfView(view))
	if f := d.Frame(); f != nil {
		f.Layout()
	}
	return true
}

func (d *Dock) registerToolInstance(view IWidget) {
	if f := d.Frame(); f != nil {
		f.syncToolViewDefs()
		if p := f.toolViews[core.FactoryNameOf(view)]; p != nil && p.Widget == nil {
			p.Widget = view
		}
	}
}

func (d *Dock) forgetToolInstance(view IWidget, idx int) {
	if f := d.Frame(); f != nil {
		for _, p := range f.toolViews {
			if p.Widget == view {
				p.Widget = nil
				p.DockPath = DockPath(d)
				p.ViewIdx = idx
				break
			}
		}
	}
}

func (d *Dock) releaseViewMenu(view IWidget) {
	if menu := d.menuCache[view]; menu != nil {
		menu.Hide()
		menu.Detach()
		if d.menu == menu {
			d.menu = nil
		}
	}
	delete(d.menuCache, view)
}

func (d *Dock) removeHiddenView(view IWidget) bool {
	idx, ok := d.hiddenViews[view]
	if !ok {
		return false
	}
	delete(d.hiddenViews, view)
	d.forgetToolInstance(view, idx)
	d.releaseViewMenu(view)
	view.SetParent(nil)
	d.DetachIfEmpty()
	return true
}

// IsMinimized reports an explicitly minimized panel, not one hidden by a mode.
func (d *Dock) IsMinimized() bool { return d.isMin }
func (d *Dock) SetMinimized(on bool) {
	if on {
		d.SetMaximized(false)
	}
	d.isMin = on
	d.SetVisible(!on)
	if on {
		d.focusVisibleView()
	}
	if f := d.Frame(); f != nil {
		f.Layout()
	}
}

func (d *Dock) IsMaximized() bool {
	f := d.Frame()
	return f != nil && f.maximizedDock == d
}

// SetMaximized uses the existing split tree. Other docks are effectively hidden,
// without overwriting their requested visibility or any split ratio.
func (d *Dock) SetMaximized(on bool) {
	f := d.Frame()
	if f == nil {
		return
	}
	if !on {
		if f.maximizedDock == d {
			f.RestoreDockLayout()
		}
		return
	}
	if f.maximizedDock == d {
		return
	}
	f.RestoreDockLayout()
	d.isMin = false
	d.Widget.SetVisible(true)
	f.maximizedDock = d
	for _, other := range f.AllDocks() {
		if other != d {
			cancelClipboardPastes(other)
			dismissOwnedToasts(other)
			other.PopCapture()
		}
	}
	d.syncWindowButtons()
	f.Layout()
	d.focusVisibleView()
}

func (d *Dock) SetVisible(on bool) {
	if !on {
		d.SetMaximized(false)
	}
	d.Widget.SetVisible(on)
}

func (d *Dock) syncWindowButtons() {
	icon, tip := "maximize", "Maximize Panel"
	if d.IsMaximized() {
		icon, tip = "restore", "Restore Panel"
	}
	d.maxBtn.SetIcon(LoadIcon(icon))
	SetToolTip(d.maxBtn, i18n.T(tip))
	SetToolTip(d.minBtn, i18n.T("Hide Panel"))
}

func (d *Dock) focusVisibleView() {
	if d.IsVisible() {
		if view := d.ActiveView(); view != nil {
			view.SetFocus()
			return
		}
	}
	if f := d.Frame(); f != nil {
		if main := f.MainDock(); main != nil && main.IsVisible() {
			if view := main.ActiveView(); view != nil {
				view.SetFocus()
				return
			}
		}
		f.SetFocus()
	}
}

// RestoreDockLayout ends temporary maximization without showing panels that the
// user had hidden or minimized. No pixel geometry snapshots or global maps remain.
func (f *Frame) RestoreDockLayout() {
	if d := f.maximizedDock; d != nil {
		f.maximizedDock = nil
		d.syncWindowButtons()
		f.Layout()
	}
}

// AvailableViews returns open and retained tool views for a host's panel chooser.
// Hidden views are unordered; hosts can sort by their localized title.
func (d *Dock) AvailableViews() []IWidget {
	views := d.AllViews()
	for view := range d.hiddenViews {
		views = append(views, view)
	}
	return views
}

// DockViewTitle and DockViewIcon match the tab strip, including host overrides
// and the registered tool definition when a widget has no Title method.
func DockViewTitle(view IWidget) string {
	if view == nil {
		return ""
	}
	return (&_Tab{data: view}).realText()
}
func DockViewIcon(view IWidget) paint.Icon {
	if view == nil {
		return nil
	}
	return (&_Tab{data: view}).realIcon()
}
