package gui

import "testing"

func TestTabReplaceKeepsOrderSelectionMetadataAndOwnedLifetime(t *testing.T) {
	tabs := NewTabWidget()
	a, b, c := NewLabel("a"), NewLabel("b"), NewLabel("c")
	tabs.AddTab(a, "A", nil)
	tabs.AddTab(b, "B", nil)
	tabs.SetCurrentIndex(1)
	calls := 0
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		calls++
		if idx != 1 || tabs.Stack().CurrentPage() != c || tabs.TabTitle(idx) != "C" || c.Parent() != tabs.Stack() {
			t.Error("replacement published intermediate identity")
		}
	})
	if !tabs.ReplaceTab(1, c, "C", nil) || calls != 1 || tabs.Count() != 2 || tabs.Stack().Page(0) != a || b.Parent() != nil || b.IsVisible() || !c.IsVisible() {
		t.Fatal("active replacement lost order/lifetime")
	}
	if !tabs.ReplaceTab(1, c, "C", nil) || calls != 1 {
		t.Fatal("same page spuriously notified")
	}
	d := NewLabel("d")
	if !tabs.ReplaceTab(0, d, "D", nil) || calls != 1 || d.IsVisible() || tabs.Stack().CurrentPage() != c {
		t.Fatal("inactive replacement changed active page")
	}
	if tabs.ReplaceTab(0, c, "bad", nil) || tabs.ReplaceTab(-1, NewLabel("bad"), "bad", nil) || tabs.ReplaceTab(0, tabs, "bad", nil) {
		t.Fatal("accepted invalid/owned page")
	}
}
func TestTabReplaceReentrantSelectionAndCloseOwnFinalState(t *testing.T) {
	tabs := NewTabWidget()
	a, b := NewLabel("a"), NewLabel("b")
	tabs.AddTab(a, "a", nil)
	tabs.AddTab(b, "b", nil)
	c := NewLabel("c")
	tabs.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		if idx == 0 && tabs.Stack().CurrentPage() == c {
			tabs.RemoveTab(0)
		}
	})
	if !tabs.ReplaceTab(0, c, "c", nil) || tabs.Count() != 1 || tabs.Stack().CurrentPage() != b || c.Parent() != nil {
		t.Fatal("reentrant close resurrected replacement")
	}
	parent := new(Widget)
	parent.Init(parent)
	tabs.SetParent(parent)
	if tabs.ReplaceTab(0, parent, "cycle", nil) {
		t.Fatal("accepted ancestor cycle")
	}
}

type replaceHidePage struct {
	Widget
	hide func()
}

func (p *replaceHidePage) OnHide() {
	if p.hide != nil {
		p.hide()
	}
}
func TestTabReplaceHonorsSelectionFromOldPageLifecycle(t *testing.T) {
	tabs := NewTabWidget()
	old := new(replaceHidePage)
	old.Init(old)
	b := NewLabel("b")
	tabs.AddTab(old, "old", nil)
	tabs.AddTab(b, "b", nil)
	old.hide = func() { tabs.SetCurrentIndex(1) }
	next := NewLabel("next")
	tabs.ReplaceTab(0, next, "next", nil)
	if tabs.CurrentIndex() != 1 || tabs.Stack().CurrentPage() != b || !b.IsVisible() || next.IsVisible() {
		t.Fatal("lifecycle selection left tab strip and stack inconsistent")
	}
}
