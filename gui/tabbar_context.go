package gui

// SetContextMenuCallback configures an owned, reused popup for a right-clicked
// tab or Shift+F10 on the focused strip. Opening does not select the tab.
// The index is current only while building: actions must revalidate their
// target's identity, since tabs can move/close while a popup is open.
// UI thread only. Passing nil removes the popup and the callback.
func (b *TabBar) SetContextMenuCallback(build func(*TabBar, *Menu, int)) {
	b.contextDown = nil
	if m := b.contextMenu; m != nil {
		m.Hide()
		m.clearOwnedContextItems()
		m.DetachWindow()
		m.Detach()
		b.contextMenu = nil
	}
	b.cbContextMenu = build
}

func (b *TabBar) OnRightDown(x, y float64) {
	b.contextDown = nil
	if b.cbContextMenu != nil && tabNavigationAvailable(b) {
		if index, _ := b.HitTest(x, y); index >= 0 {
			b.contextDown = b.tabs[index]
		}
	}
}

func (b *TabBar) OnRightUp(x, y float64) {
	target := b.contextDown
	b.contextDown = nil
	if target == nil {
		return
	}
	if index, _ := b.HitTest(x, y); index >= 0 && b.tabs[index] == target {
		b.showTabContextMenu(index, x, y)
	}
}

func (b *TabBar) showTabContextMenu(index int, x, y float64) {
	if b.cbContextMenu == nil || index < 0 || index >= len(b.tabs) || !tabNavigationAvailable(b) {
		return
	}
	if b.contextMenu == nil {
		b.contextMenu = NewPopupMenu()
		b.contextMenu.clearOnHide = true
		b.contextMenu.SetParent(b)
	}
	menu := b.contextMenu
	menu.Hide()
	menu.clearOwnedContextItems()
	b.cbContextMenu(b, menu, index)
	// A builder may disable/replace itself. Do not resurrect its detached popup.
	if b.contextMenu != menu || b.cbContextMenu == nil || len(menu.Items()) == 0 {
		return
	}
	gx, gy := b.MapToGlobal(x, y)
	showTabBarPopup(menu, gx, gy)
}

func (b *TabBar) contextMenuKey(key int, repeat bool) bool {
	if key != KeyF10 || !IsKeyDown(KeyShift) || IsKeyDown(KeyCtrl) || IsKeyDown(KeyMenu) || IsKeyDown(KeyLWin) || IsKeyDown(KeyRWin) || b.cbContextMenu == nil {
		return false
	}
	if !repeat && b.activeTab >= 0 && b.activeTab < len(b.tabs) {
		tab := b.tabs[b.activeTab]
		b.showTabContextMenu(b.activeTab, max(0, tab.x-b.scrollX), b.Height())
	}
	return true
}

// Context entries belong to this popup, not to a global registry. Release
// captures even on Escape, not only when the next popup replaces them.
func (m *Menu) clearOwnedContextItems() {
	for _, item := range m.Items() {
		if b, ok := item.(*Button); ok {
			if sub, ok := b.SubPopup().(*Menu); ok {
				sub.Hide()
				sub.clearOwnedContextItems()
				sub.DetachWindow()
				sub.Detach()
			}
		}
		SetToolTip(item, "")
		DetachContextMenu(item)
	}
	m.Clear()
}

// Windows dispatches F10 through WM_SYSKEYDOWN, even without Alt. Claim only
// our Shift+F10 chord; unhandled system keys retain native/host behavior.
func (b *TabBar) OnSystemKeyDown(key int, repeat bool) bool {
	return b.contextMenuKey(key, repeat)
}
