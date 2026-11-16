package gui

import "testing"

func TestCurrentFocusWidgetReportsIdentityWithoutChangingIt(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	focusWidget = nil
	if CurrentFocusWidget() != nil {
		t.Fatal("nil input owner")
	}
	a, b := new(Widget), new(Widget)
	a.Init(a)
	b.Init(b)
	a.SetFocus()
	if CurrentFocusWidget() != a || !a.HasFocus() {
		t.Fatal("first input owner")
	}
	for range 100 {
		if CurrentFocusWidget() != a {
			t.Fatal("reading changes input owner")
		}
	}
	b.SetFocus()
	if CurrentFocusWidget() != b || a.HasFocus() || !b.HasFocus() {
		t.Fatal("new input owner")
	}
}
