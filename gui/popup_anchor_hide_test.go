package gui

import "testing"

// Use a logically attached popup without creating a native window in a unit
// test. Cross-platform backend surface state is exercised by the native probe.
type popupAnchorHideProbe struct {
	Widget
	hides  int
	hidden func()
}

func (p *popupAnchorHideProbe) OnHide() {
	p.hides++
	if p.hidden != nil {
		p.hidden()
	}
}

func popupUnder(owner IWidget) *popupAnchorHideProbe {
	p := &popupAnchorHideProbe{}
	p.Init(p)
	p.SetParent(owner)
	p.wt = WtPopup
	return p
}

func TestHidingInlineAnchorRetiresOpenPopupsAndRetainsIndependentForm(t *testing.T) {
	root, owner, inline := NewForm(), NewFrame(), NewFrame()
	owner.SetParent(root)
	inline.SetParent(owner)
	anchor := NewButton()
	anchor.SetParent(inline)
	popup := popupUnder(anchor)
	nested := popupUnder(popup)

	otherForm := NewForm()
	otherForm.SetParent(owner)
	otherForm.wt = WtForm
	independent := popupUnder(otherForm)

	owner.Hide()
	if popup.IsVisible() || nested.IsVisible() || popup.hides != 1 || nested.hides != 1 {
		t.Fatalf("hidden ancestor retained its open popup tree: popup=%v/%d nested=%v/%d",
			popup.IsVisible(), popup.hides, nested.IsVisible(), nested.hides)
	}
	if !otherForm.IsVisible() || !independent.IsVisible() || independent.hides != 0 {
		t.Fatal("inline hide crossed an independently owned form")
	}
	if popup.Parent() != anchor || nested.Parent() != popup {
		t.Fatal("reusable popups should hide without being detached or destroyed")
	}
	owner.Show()
	if popup.IsVisible() || nested.IsVisible() {
		t.Fatal("showing the old anchor unexpectedly reopened its popups")
	}
}

func TestHidingInlineAncestorDoesNotClosePopupMovedByCallback(t *testing.T) {
	root, oldHost, newHost := NewForm(), NewFrame(), NewFrame()
	oldHost.SetParent(root)
	newHost.SetParent(root)
	first := popupUnder(oldHost)
	movingAnchor := NewFrame()
	movingAnchor.SetParent(oldHost)
	moved := popupUnder(movingAnchor)
	// Reparent the inline anchor, not its popup: the latter would try to
	// create an actual NSWindow from the Go unit test goroutine.
	first.hidden = func() { movingAnchor.SetParent(newHost) }
	oldHost.Hide()
	if first.hides != 1 || first.IsVisible() {
		t.Fatal("original popup was not hidden exactly once")
	}
	if movingAnchor.Parent() != newHost || moved.Parent() != movingAnchor || moved.hides != 0 || !moved.IsVisible() {
		t.Fatal("a stale hide snapshot closed a popup after it moved to a visible host")
	}
}
