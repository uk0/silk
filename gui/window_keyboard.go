package gui

// Keyboard focus is logical and global: non-activating popups intentionally
// keep receiving their owner's native keys. A stale focus in another Form,
// however, must not consume this window's input.
func (win *Window) keyTarget() IWidget {
	if focusWidget != nil && focusWidget.IsAllAncentorsVisible() {
		owner := focusWidget.OwnerWindow()
		if owner == win || popupOwnedBy(owner, win) {
			return focusWidget
		}
	}
	return win.widget
}

// Escape belongs to the receiving popup/dialog, not to whichever child the
// Tab chain selected. IEventKeyDown has no handled result, so routing it after
// a child handler cannot distinguish consumption from a silently ignored key.
// Enter and ordinary keys deliberately remain owned by the focused child.
func (win *Window) dispatchEscape(key int, repeat bool) bool {
	if key != KeyEsc || win.widget == nil || !win.widget.IsVisible() {
		return false
	}
	popup := win
	if target := win.keyTarget(); target != nil && !win.widget.IsPopup() {
		popup = target.OwnerWindow()
	}
	if popup != nil && popup.wt == WtPopup {
		// Close the popup chain only, never cross an independent Form.
		for popup.popupOwner != nil && popup.popupOwner != popup && popup.popupOwner.wt == WtPopup {
			popup = popup.popupOwner
		}
		if !repeat && popup.widget != nil {
			popup.widget.Hide()
		}
		return true
	}
	if dialog, ok := win.widget.(interface{ resolveCancel() }); ok {
		if !repeat {
			dialog.resolveCancel()
		}
		return true
	}
	return false
}

// Modal dialogs and their popups use their own focused controls. Application
// bindings (Open, Save, Undo, Quit) must not mutate a disabled editor behind them.
func (win *Window) allowsApplicationShortcuts() bool {
	for current := win; current != nil; {
		if current.inModal {
			return false
		}
		if current.wt == WtPopup {
			current = current.popupOwner
		} else if current.wt == WtChild {
			current = current.ParentWindow()
		} else {
			break
		}
	}
	return true
}
func (win *Window) dispatchWindowShortcut(key int) bool {
	if win.dispatchShortcutOverride(key) {
		return true
	}
	return win.allowsApplicationShortcuts() && dispatchShortcut(key)
}

// A MessageBox contains read-only selectable text, not an editable multi-line
// field. Enter acknowledges it even when that text has focus; real Dialog/Edit
// multi-line fields keep their existing newline behavior.
func (win *Window) dispatchMessageBoxEnter(key int, repeat bool) bool {
	box, ok := win.widget.(*MessageBox)
	if !ok || key != KeyEnter || !box.IsVisible() || currentMods() != 0 {
		return false
	}
	if !repeat {
		box.resolveDefault()
	}
	return true
}
