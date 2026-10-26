package gui

import (
	"github.com/uk0/silk/paint"
	"sort"
)

// updateViewport uses the same coordinates for painting, hit-testing and DND.
// The leading cell starts on a tab boundary. A trailing cell is measured for
// its visible width, never painted as a clipped title/icon fragment. All tabs
// remain available in the chooser, including strips too narrow for a label.
func (b *TabBar) updateViewport() {
	w, h := b.Size()
	w, h = max(0, w), max(0, h)
	natural := 0.0
	for _, tab := range b.tabs {
		natural += tab.SizeHints().Width
	}
	b.viewportW = w
	if natural > w {
		button := b.allTabsButton()
		bw := min(w, max(24, h))
		b.viewportW = max(0, w-bw)
		button.SetBounds(b.viewportW, 0, bw, h)
		button.SetVisible(w > 0 && h > 0)
	} else if b.overflowBtn != nil {
		b.overflowBtn.SetVisible(false)
	}
	b.contentW = 0
	t := Theme()
	for _, tab := range b.tabs {
		tab.x = b.contentW
		tab.width = min(tab.SizeHints().Width, b.viewportW)
		tab.paintWidth = tab.width
		textW := tab.width - t.TabMargin.L - t.TabMargin.R - t.TabCloseSize
		if tab.icon != nil {
			textW -= t.TabMargin.L + t.IconSize
		}
		tab.paintLabel = elideToWidth(t.Font, tab.label, max(0, textW))
		b.contentW += tab.width
	}
	b.ensureActiveVisible()
}

func (b *TabBar) ensureActiveVisible() {
	old := b.scrollX
	b.firstVisible, b.lastVisible = 0, 0
	if len(b.tabs) == 0 || b.viewportW <= 0 {
		b.scrollX = 0
		return
	}
	// Round the scroll origin up to an actual cell boundary, rather than
	// exposing the suffix of the preceding title when revealing a later tab.
	start := max(0, min(b.scrollX, b.contentW-b.viewportW))
	first := sort.Search(len(b.tabs), func(i int) bool { return b.tabs[i].x >= start })
	first = min(first, len(b.tabs)-1)
	if b.activeTab >= 0 && b.activeTab < len(b.tabs) {
		active := b.tabs[b.activeTab]
		if b.activeTab < first {
			first = b.activeTab
		} else if active.x+active.width > b.tabs[first].x+b.viewportW {
			start = active.x + active.width - b.viewportW
			first = sort.Search(len(b.tabs), func(i int) bool { return b.tabs[i].x >= start })
			first = min(first, b.activeTab)
		}
	}
	b.firstVisible, b.scrollX = first, b.tabs[first].x
	b.updateVisibleTabs()
	if b.scrollX != old {
		b.hoverTab = -1
		b.hoverCloseBtn = false
		b.Update()
	}
}

// Only visible cells are revisited when selection scrolls. Font measurements
// are cached until a cell's presentation width changes; Draw never elides.
func (b *TabBar) updateVisibleTabs() {
	b.lastVisible = b.firstVisible
	t := Theme()
	for i := b.firstVisible; i < len(b.tabs); i++ {
		tab := b.tabs[i]
		width := min(tab.width, b.viewportW-(tab.x-b.scrollX))
		if width <= 0 {
			break
		}
		padding := t.TabMargin.L + t.TabMargin.R + t.TabCloseSize
		if tab.icon != nil {
			padding += t.TabMargin.L + t.IconSize
		}
		// A tiny remainder is blank, not an anonymous fragment with a hit target.
		if width < tab.width && width < padding+t.Font.TextExtents(elideMark).Width {
			break
		}
		if tab.paintWidth != width {
			tab.paintWidth = width
			tab.paintLabel = elideToWidth(t.Font, tab.label, max(0, width-padding))
		}
		b.lastVisible = i + 1
	}
}

// Close painting and hit-testing share this guard. A close glyph must fit in
// full, without colliding with the tab icon, even in a one-pixel viewport.
func tabCloseFits(t *defaultTheme, icon paint.Icon, width, height float64) bool {
	left := t.TabMargin.L
	if icon != nil {
		left += t.IconSize + t.TabMargin.L
	}
	return width >= left+t.TabCloseSize+t.TabMargin.R && height >= t.TabMargin.T+t.TabCloseSize+t.TabMargin.B
}

func (b *TabBar) allTabsButton() *Button {
	if b.overflowBtn == nil {
		button := NewButton1("All tabs", paint.LoadIcon("toolbar-overflow"))
		button.SetVisible(false)
		button.SetTextVisible(false)
		button.SetFocusPolicy(TabFocus)
		button.SetParent(b)
		button.Action().BindFunc0(b.showAllTabs)
		b.overflowBtn = button
	}
	return b.overflowBtn
}

// ShowAsPopup is nonblocking: the chooser must retain its logical parent
// until it closes. Reuse one owned menu instead of accumulating child menus;
// neither it nor its entries are registered in the global tooltip map.
var showTabBarPopup = func(menu *Menu, x, y float64) { menu.ShowAsPopup(x, y, true) }

func (b *TabBar) showAllTabs() {
	if len(b.tabs) == 0 {
		return
	}
	if b.chooserMenu == nil {
		b.chooserMenu = NewPopupMenu()
		b.chooserMenu.SetParent(b)
	}
	menu := b.chooserMenu
	menu.Hide()
	menu.Clear()
	for index, tab := range b.tabs {
		entry := menu.AddButton1(tab.realText(), tab.realIcon())
		entry.Action().SetChecked(index == b.activeTab)
		entry.Action().BindFunc0(func() {
			// Reordering/removing views while a popup is open must not activate
			// the unrelated view that later inherits an old numeric index.
			for i, current := range b.tabs {
				if current == tab {
					b.SetActiveTab(i)
					// The popup restored focus to its opener before this action.
					// Selecting an editor must hand input to the selected page,
					// not leave typing on the overflow button. Revalidate after
					// activation callbacks, which can remove or replace the tab.
					if b.activeTab >= 0 && b.activeTab < len(b.tabs) && b.tabs[b.activeTab] == tab {
						var view IWidget
						if page, ok := tab.data.(*tabPageRef); ok {
							view = page.page
						} else {
							view, _ = tab.data.(IWidget)
						}
						if view != nil && view.IsVisible() && view.IsEnabled() && view.IsAllAncentorsVisible() {
							view.SetFocus()
						}
					}
					return
				}
			}
		})
	}
	x, y := b.MapToGlobal(b.Width(), b.Height())
	if b.overflowBtn != nil && b.overflowBtn.IsVisible() {
		x, y = b.overflowBtn.MapToGlobal(b.overflowBtn.Width(), b.overflowBtn.Height())
	}
	showTabBarPopup(menu, x-menu.SizeHints().Width, y)
}

func (b *TabBar) OnMouseWheel(x, y, z float64) {
	if len(b.tabs) == 0 || z == 0 {
		return
	}
	step := 1
	if z > 0 {
		step = -1
	}
	b.SetActiveTab(max(0, min(len(b.tabs)-1, b.activeTab+step)))
}

func (b *TabBar) OnKeyDown(key int, repeat bool) {
	if b.contextMenuKey(key, repeat) {
		return
	}
	if len(b.tabs) == 0 {
		return
	}
	switch key {
	case KeyTab:
		if IsKeyDown(KeyCtrl) {
			step := 1
			if IsKeyDown(KeyShift) {
				step = -1
			}
			b.SetActiveTab((b.activeTab + step + len(b.tabs)) % len(b.tabs))
		}
	case KeyLeft:
		b.SetActiveTab(max(0, b.activeTab-1))
	case KeyRight:
		b.SetActiveTab(min(len(b.tabs)-1, b.activeTab+1))
	case KeyHome:
		b.SetActiveTab(0)
	case KeyEnd:
		b.SetActiveTab(len(b.tabs) - 1)
	case KeyDown:
		b.showAllTabs()
	}
}
