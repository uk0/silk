package gui

import (
	"github.com/uk0/silk/paint"
	"sort"
)

// updateViewport uses the same coordinates for painting, hit-testing and DND.
// Only the active cell must be wholly visible; every tab remains available in
// the chooser, including a single long title in an extremely narrow strip.
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
	b.scrollX = max(0, min(b.scrollX, b.contentW-b.viewportW))
	if b.activeTab >= 0 && b.activeTab < len(b.tabs) && b.viewportW > 0 {
		x := b.tabs[b.activeTab].x
		if x < b.scrollX {
			b.scrollX = x
		}
		if end := x + b.tabs[b.activeTab].width; end > b.scrollX+b.viewportW {
			b.scrollX = end - b.viewportW
		}
	}
	b.firstVisible = sort.Search(len(b.tabs), func(i int) bool { return b.tabs[i].x+b.tabs[i].width > b.scrollX })
	if b.scrollX != old {
		b.hoverTab = -1
		b.hoverCloseBtn = false
		b.Update()
	}
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
