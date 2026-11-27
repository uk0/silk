package gui

import "testing"

func TestSubPopupSkipsHiddenAnchorTree(t *testing.T) {
	root, host, wrapper := NewForm(), NewFrame(), NewFrame()
	host.SetParent(root)
	wrapper.SetParent(host)
	anchor := NewButton()
	anchor.SetParent(wrapper)
	opened := 0
	anchor.SetSubPopupCallback(func(button IButton) {
		opened++
		// No native window is needed: observe the real pre-open callback and
		// stop inside it before native creation so headless tests are safe.
		anchor.SetSubPopup(nil)
	})
	for _, tc := range []struct {
		name string
		hide func()
		show func()
	}{
		{"outer container", host.Hide, host.Show},
		{"anchor itself", anchor.Hide, anchor.Show},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.hide()
			t.Cleanup(tc.show)
			opened = 0
			anchor.SetSubPopup(NewMenu(true))
			anchor.ShowSubPopup()
			if opened != 0 {
				t.Errorf("hidden anchor requested submenu population %d time(s)", opened)
			}
		})
	}
	if !WidgetTreeVisible(anchor) {
		t.Fatal("visible control did not rejoin its mounted tree")
	}
	opened = 0
	anchor.SetSubPopup(NewMenu(true))
	anchor.ShowSubPopup()
	if opened != 1 {
		t.Fatalf("visible anchor callback count = %d, want 1", opened)
	}
}

type submenuAttachProbe struct {
	Widget
	attaches int
}

func (p *submenuAttachProbe) AttachWindow(WindowType) { p.attaches++ }

func TestSubPopupRechecksAnchorTreeAfterPopulation(t *testing.T) {
	outer, wrapper := NewFrame(), NewFrame()
	wrapper.SetParent(outer)
	anchor := NewButton()
	anchor.SetParent(wrapper)
	panel := &submenuAttachProbe{}
	panel.Init(panel)
	anchor.SetSubPopup(panel)
	populated := false
	anchor.SetSubPopupCallback(func(IButton) {
		populated = true
		outer.Hide()
	})
	anchor.ShowSubPopup()
	if !populated {
		t.Fatal("visible anchor did not invoke popup population")
	}
	if panel.attaches != 0 || panel.Parent() == anchor {
		t.Fatalf("popup attached after population hid owner: attempts=%d anchorParent=%v",
			panel.attaches, panel.Parent() == anchor)
	}
}
