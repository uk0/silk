package gui

import "testing"

// newTabBar3 builds a laid-out three-tab strip, the same shape the designer's
// centre dock shows ("untitled | Welcome | 编辑器"), without needing a window.
func newTabBar3() *TabBar {
	tb := NewTabBar()
	tb.AddTab(&tabPageRef{title: "untitled"}, false)
	tb.AddTab(&tabPageRef{title: "Welcome"}, false)
	tb.AddTab(&tabPageRef{title: "编辑器"}, false)
	hints := tb.SizeHints()
	tb.SetBounds(0, 0, hints.Width, hints.Height)
	tb.Layout()
	return tb
}

// tabCellStart returns the x where tab idx's cell begins, the same running sum
// TabBar.Draw translates by and TabBar.HitTest walks.
func tabCellStart(tb *TabBar, idx int) float64 {
	x := 0.0
	for i := 0; i < idx; i++ {
		x += tb.tabs[i].width
	}
	return x
}

// paintedLabelSpan reproduces the ink span Theme.DrawTab paints the title into
// for tab idx, in tab-bar coordinates. DrawTab translates by xt-XBearing before
// drawing, so the glyph ink starts at the left margin (plus the icon column)
// and runs for TextExtents().Width.
func paintedLabelSpan(tb *TabBar, idx int) (x0, x1 float64) {
	t := Theme()
	tab := tb.tabs[idx]
	xt := t.TabMargin.L
	if tab.icon != nil {
		xt = t.TabMargin.L*2 + t.IconSize
	}
	x0 = tabCellStart(tb, idx) + xt
	x1 = x0 + t.Font.TextExtents(tab.label).Width
	return
}

// paintedCloseSpan reproduces the band Theme.DrawTab paints the close glyph
// into for tab idx, in tab-bar coordinates.
func paintedCloseSpan(tb *TabBar, idx int) (x0, x1 float64) {
	t := Theme()
	x0 = tabCellStart(tb, idx) + tb.tabs[idx].width - t.TabMargin.R - t.TabCloseSize
	return x0, x0 + t.TabCloseSize
}

// TestTabBarHitTestMatchesPaintedLabel pins every pixel of a tab's painted
// title to that tab's own hit rectangle, including the tail of the active
// tab's title. Tabs used to be sized without reserving room for the close
// button, so DrawTab painted the ✕ on top of the last TabCloseSize pixels of
// the title and HitTest reported that band as the close hot zone: pressing on
// the last two characters of the active tab's own label closed the tab.
func TestTabBarHitTestMatchesPaintedLabel(t *testing.T) {
	tb := newTabBar3()
	_, _, _, h := tb.Bounds()
	my := h * 0.5

	for active := 0; active < tb.Count(); active++ {
		tb.SetActiveTab(active)
		for i := 0; i < tb.Count(); i++ {
			lx0, lx1 := paintedLabelSpan(tb, i)
			for _, x := range []float64{lx0 + 0.5, (lx0 + lx1) * 0.5, lx1 - 0.5} {
				idx, closeBtn := tb.HitTest(x, my)
				if idx != i {
					t.Errorf("active=%d: HitTest(%.2f) on painted label of tab %d = %d",
						active, x, i, idx)
				}
				if closeBtn {
					t.Errorf("active=%d: HitTest(%.2f) on painted label of tab %d hit the close button",
						active, x, i)
				}
			}
		}
	}
}

// TestTabBarCloseButtonClearsLabel keeps the close band and the painted title
// disjoint, and keeps the whole close band inside its own tab's cell so it
// cannot reach into the neighbouring tab.
func TestTabBarCloseButtonClearsLabel(t *testing.T) {
	tb := newTabBar3()
	for i := 0; i < tb.Count(); i++ {
		_, lx1 := paintedLabelSpan(tb, i)
		cx0, cx1 := paintedCloseSpan(tb, i)
		if cx0 < lx1 {
			t.Errorf("tab %d: close band starts at %.2f, inside the title ending at %.2f", i, cx0, lx1)
		}
		cellEnd := tabCellStart(tb, i) + tb.tabs[i].width
		if cx1 > cellEnd {
			t.Errorf("tab %d: close band ends at %.2f, past the cell end %.2f", i, cx1, cellEnd)
		}
	}
}

// TestTabBarPressOutsideTabsKeepsActive covers a press that lands in the strip
// but on no tab. It used to run SetActiveTab(-1), which deactivates without
// firing the activate callback: the container kept showing a view no tab
// claimed, and the next Dock.Layout() then hid every view and blanked the pane.
func TestTabBarPressOutsideTabsKeepsActive(t *testing.T) {
	tb := newTabBar3()
	tb.SetActiveTab(2)
	_, _, w, h := tb.Bounds()

	past := tabCellStart(tb, tb.Count()-1) + tb.tabs[tb.Count()-1].width
	if past >= w {
		t.Fatalf("no empty strip to press: last cell ends at %.2f, bar is %.2f wide", past, w)
	}
	if idx, _ := tb.HitTest(past+0.5, h*0.5); idx != -1 {
		t.Fatalf("HitTest(%.2f) past the last tab = %d, want -1", past+0.5, idx)
	}

	tb.OnLeftDown(past+0.5, h*0.5)
	if got := tb.ActiveTab(); got != 2 {
		t.Errorf("press on empty strip changed active tab to %d, want 2", got)
	}
}

// TestTabBarMiddleClickClosesTab covers close-on-middle-click: a press and
// release on the same tab closes it, a release that slid onto another tab is
// abandoned, and a press on empty strip closes nothing.
func TestTabBarMiddleClickClosesTab(t *testing.T) {
	tb := newTabBar3()
	var closed []int
	tb.SetCloseCallback(func(_ *TabBar, idx int) bool {
		closed = append(closed, idx)
		return true
	})
	_, _, w, h := tb.Bounds()
	my := h * 0.5
	mid := func(i int) float64 { return tabCellStart(tb, i) + tb.tabs[i].width*0.5 }

	tb.OnMiddleDown(mid(1), my)
	tb.OnMiddleUp(mid(1), my)
	if len(closed) != 1 || closed[0] != 1 {
		t.Fatalf("middle click on tab 1 closed %v, want [1]", closed)
	}

	closed = nil
	tb.OnMiddleDown(mid(0), my)
	tb.OnMiddleUp(mid(2), my)
	if closed != nil {
		t.Errorf("middle press on tab 0 released on tab 2 closed %v, want nothing", closed)
	}

	past := tabCellStart(tb, tb.Count()-1) + tb.tabs[tb.Count()-1].width
	if past+0.5 < w {
		tb.OnMiddleDown(past+0.5, my)
		tb.OnMiddleUp(past+0.5, my)
		if closed != nil {
			t.Errorf("middle click on empty strip closed %v, want nothing", closed)
		}
	}
}

// TestTabBarMiddleUpWithoutPressClosesNothing covers a release the strip never
// armed. The middle button takes no capture, so a press that wanders off the
// strip is followed by a move that reassigns lastMouseWidget: the release goes
// to whatever is under the cursor and the strip never runs OnMiddleUp. With the
// candidate left armed, the *next* middle release that happens to land on the
// strip — from a press somewhere else entirely — closed the tab of the
// abandoned press.
func TestTabBarMiddleUpWithoutPressClosesNothing(t *testing.T) {
	tb := newTabBar3()
	var closed []int
	tb.SetCloseCallback(func(_ *TabBar, idx int) bool {
		closed = append(closed, idx)
		return true
	})
	_, _, _, h := tb.Bounds()
	my := h * 0.5
	mid := func(i int) float64 { return tabCellStart(tb, i) + tb.tabs[i].width*0.5 }

	tb.OnMiddleDown(mid(1), my)
	tb.OnMouseLeave() // pointer left the strip; the release lands elsewhere

	tb.OnMiddleUp(mid(1), my) // a later press's release, drifting back over tab 1
	if closed != nil {
		t.Fatalf("middle release with no press on the strip closed %v, want nothing", closed)
	}
}
