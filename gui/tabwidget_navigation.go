package gui

// NavigateTab selects the next/previous tab in visual strip order with wrap,
// then focuses the new page's first enabled keyboard control (or its tab bar
// when the page is display-only). UI thread only; no disk/LSP work or timers.
// Empty/single-tab and hidden/disabled containers are no-ops. A callback may
// redirect/remove the selected page; that decision is never overwritten.
func (tabs *TabWidget) NavigateTab(previous bool) bool {
	if tabs.Count() < 2 || !tabNavigationAvailable(tabs) {
		return false
	}
	index := nextTabIndex(tabs.CurrentIndex(), tabs.Count(), !previous)
	page := tabs.stack.Page(index)
	oldFocus := focusWidget
	tabs.SetCurrentIndex(index)
	if tabs.stack.CurrentPage() != page || page.Parent() != tabs.stack || !tabNavigationAvailable(page) {
		return true
	}
	// Preserve a callback's explicit focus transfer, including another window.
	if focusWidget != oldFocus && focusWidget != nil {
		return true
	}
	target := firstTabPageFocus(page)
	if target == nil {
		target = tabs.tabBar
	}
	target.SetFocus()
	return true
}

func tabNavigationAvailable(w IWidget) bool {
	for ; w != nil; w = w.Parent() {
		if !w.IsVisible() || !w.IsEnabled() {
			return false
		}
	}
	return true
}

// Stop at the first eligible control instead of allocating/traversing a list
// of the whole page on every Ctrl+Tab. Disabled containers exclude descendants.
func firstTabPageFocus(w IWidget) IWidget {
	if w == nil || !w.IsVisible() || !w.IsEnabled() {
		return nil
	}
	if isTabFocusable(w) {
		return w
	}
	for _, child := range w.Children() {
		if found := firstTabPageFocus(child); found != nil {
			return found
		}
	}
	return nil
}

func (win *Window) dispatchTabNavigation(key int) bool {
	if !IsKeyDown(KeyCtrl) || IsKeyDown(KeyMenu) || IsKeyDown(KeyLWin) || IsKeyDown(KeyRWin) {
		return false
	}
	previous := IsKeyDown(KeyShift)
	switch key {
	case KeyTab:
	case KeyPageUp, KeyPageDown:
		if previous {
			return false
		}
		previous = key == KeyPageUp
	default:
		return false
	}
	// The host has first priority on press; repeats of a host-owned shortcut
	// must not unexpectedly fall through into our local default.
	if win.allowsApplicationShortcuts() && ShortcutHandler(currentMods(), key) != nil {
		return true
	}
	target := win.keyTarget()
	if target == nil {
		return false
	}
	// Non-activating menus retain their owner as the receiving native window.
	// Do not let modified Tab escape a popup into the document behind it.
	if win.wt == WtPopup || (target.OwnerWindow() != win && popupOwnedBy(target.OwnerWindow(), win)) {
		return true
	}
	if !tabNavigationAvailable(target) {
		return false
	}
	for w := target; w != nil && w.OwnerWindow() == win; w = w.Parent() {
		if tabs, ok := w.(*TabWidget); ok {
			tabs.NavigateTab(previous)
			return true // nearest tab group owns even its single-tab no-op
		}
	}
	return false
}
