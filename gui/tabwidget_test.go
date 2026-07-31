package gui

import "testing"

// TestTabWidgetRemoveTabKeepsPageInSync removes a tab to the left of the
// current one. TabBar.RemoveTab re-activates a surviving tab from inside
// RemoveTab and that callback indexes into the stack, so removing the tab
// before the page made the callback address the stack with post-removal tab
// indices: the bar ended up on "three" while the stack showed "two".
//
// The follow-on click is the user-visible half of that bug. SetActiveTab
// returns early when the tab is already active, so once the two disagree,
// clicking the tab the bar already considers active fires no callback and the
// wrong page stays on screen no matter how often it is clicked.
func TestTabWidgetRemoveTabKeepsPageInSync(t *testing.T) {
	tw := newTabWidget3()
	tw.SetCurrentIndex(2)

	tw.RemoveTab(0)

	if got, want := tw.tabBar.ActiveTab(), 1; got != want {
		t.Fatalf("active tab = %d, want %d", got, want)
	}
	if got, want := tw.stack.CurrentIndex(), 1; got != want {
		t.Fatalf("stack page = %d, want %d (bar says %d)", got, want, tw.tabBar.ActiveTab())
	}
	if got, want := tw.refs[tw.stack.CurrentIndex()].title, "Three"; got != want {
		t.Fatalf("visible page = %q, want %q", got, want)
	}

	// Clicking the tab the bar already paints active must be a no-op that
	// leaves the pair in sync, not a click that silently does nothing while
	// the wrong page shows.
	tb := tw.tabBar
	x := 0.0
	for i := 0; i < 1; i++ {
		x += tb.tabs[i].width
	}
	x += tb.tabs[1].width * 0.5
	_, _, _, h := tb.Bounds()
	tb.OnLeftDown(x, h*0.5)
	tb.OnLeftUp(x, h*0.5)
	if tb.ActiveTab() != tw.stack.CurrentIndex() {
		t.Errorf("after clicking the active tab: bar = %d, stack = %d",
			tb.ActiveTab(), tw.stack.CurrentIndex())
	}
}
