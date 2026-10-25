package gui

import "testing"

type tabContextAPI interface {
	SetContextMenuCallback(func(*TabBar, *Menu, int))
	OnRightDown(float64, float64)
	OnRightUp(float64, float64)
}

func TestTabBarContextMenuAvailable(t *testing.T) {
	if _, ok := interface{}(NewTabBar()).(tabContextAPI); !ok {
		t.Fatal("TabBar has no owned context-menu/right-click API")
	}
}

func TestTabContextHitIdentityOwnershipAndRelease(t *testing.T) {
	b := overflowTabs(3)
	old := showTabBarPopup
	t.Cleanup(func() { showTabBarPopup = old; b.SetContextMenuCallback(nil) })
	var shown *Menu
	shows, built := 0, -1
	showTabBarPopup = func(m *Menu, _, _ float64) { shown = m; shows++ }
	b.SetContextMenuCallback(func(bar *TabBar, m *Menu, index int) {
		if bar != b {
			t.Fatal("wrong owner")
		}
		built = index
		button := m.AddButton1("action", nil)
		SetToolTip(button, "owned tooltip")
		AttachContextMenu(button, func(*Menu, float64, float64) {})
	})
	x := b.tabs[1].x + b.tabs[1].width/2
	b.OnRightUp(x, 15)
	if shows != 0 {
		t.Fatal("release without press opened menu")
	}
	b.OnRightDown(x, 15)
	b.OnRightUp(x, 15)
	if shown == nil || shown.Parent() != b || built != 1 || b.ActiveTab() != 0 {
		t.Fatal("target/owner/selection incorrect")
	}
	first := shown
	for i := 0; i < 20; i++ {
		b.OnRightDown(x, 15)
		b.OnRightUp(x, 15)
		if shown != first || len(shown.Items()) != 1 {
			t.Fatal("popup/entries accumulated")
		}
	}
	item := shown.Items()[0]
	shown.OnHide()
	if len(shown.Items()) != 0 || item.Parent() != nil || GetToolTip(item) != "" {
		t.Fatal("dismissed popup retained entries")
	}
	if _, ok := contextMenuRegistry[item]; ok {
		t.Fatal("registry retained context entry")
	}
	before := shows
	b.OnRightDown(x, 15)
	b.OnRightUp(0, 15)
	b.OnRightDown(b.viewportW+1, 15)
	b.OnRightUp(b.viewportW+1, 15)
	if shows != before {
		t.Fatal("off-target release opened menu")
	}
	b.OnRightDown(x, 15)
	b.RemoveTab(1)
	b.OnRightUp(x, 15)
	if shows != before {
		t.Fatal("removed press target retargeted replacement")
	}
	b.SetEnabled(false)
	b.OnRightDown(5, 15)
	b.OnRightUp(5, 15)
	if shows != before {
		t.Fatal("disabled tab strip opened menu")
	}
	b.SetContextMenuCallback(nil)
	if b.contextMenu != nil || first.Parent() != nil || len(first.Items()) != 0 {
		t.Fatal("callback disposal leaked menu")
	}
}

func TestTabContextEmptyAndSelfReplacingBuilder(t *testing.T) {
	b := overflowTabs(2)
	old := showTabBarPopup
	t.Cleanup(func() { showTabBarPopup = old; b.SetContextMenuCallback(nil) })
	shows := 0
	showTabBarPopup = func(*Menu, float64, float64) { shows++ }
	b.SetContextMenuCallback(func(*TabBar, *Menu, int) {})
	b.showTabContextMenu(0, 0, 0)
	if shows != 0 {
		t.Fatal("empty popup shown")
	}
	b.SetContextMenuCallback(func(b *TabBar, m *Menu, _ int) { m.AddButton1("old", nil); b.SetContextMenuCallback(nil) })
	b.showTabContextMenu(0, 0, 0)
	if shows != 0 || b.contextMenu != nil {
		t.Fatal("removed popup resurrected")
	}
}

func TestTabContextDismissBeforeActionStillExecutesOnce(t *testing.T) {
	b := overflowTabs(2)
	old := showTabBarPopup
	t.Cleanup(func() { showTabBarPopup = old; b.SetContextMenuCallback(nil) })
	var m *Menu
	showTabBarPopup = func(menu *Menu, _, _ float64) { m = menu }
	calls := 0
	b.SetContextMenuCallback(func(_ *TabBar, menu *Menu, _ int) {
		menu.AddButton1("Close", nil).Action().BindFunc0(func() {
			calls++
			if len(menu.Items()) != 0 {
				t.Fatal("action still retained by hidden popup")
			}
		})
	})
	b.showTabContextMenu(0, 0, 0)
	// Logical visibility only: no native window off the UI thread.
	m.NakedWidget().hidden = false
	m.setHighlight(0)
	m.OnKeyDown(KeyEnter, false)
	if calls != 1 || len(m.Items()) != 0 {
		t.Fatalf("calls=%d retained=%d", calls, len(m.Items()))
	}
}
