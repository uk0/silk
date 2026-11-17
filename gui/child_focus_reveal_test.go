package gui

import "testing"

type focusRevealHost struct {
	Widget
	targets  []IWidget
	callback func(IWidget)
}

func newFocusRevealHost() *focusRevealHost {
	h := new(focusRevealHost)
	h.Init(h)
	return h
}

func (h *focusRevealHost) RevealChildFocus(target IWidget) {
	h.targets = append(h.targets, target)
	if h.callback != nil {
		h.callback(target)
	}
}

func TestFocusRevealsNestedChild(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	h := newFocusRevealHost()
	inner := new(Widget)
	inner.Init(inner)
	inner.SetParent(h)
	e := NewEdit()
	e.SetParent(inner)
	e.SetFocus()
	if len(h.targets) != 1 || h.targets[0] != e {
		t.Fatal("nested keyboard focus was never offered to viewport", len(h.targets))
	}
}

func TestFocusRevealStopsAtNativeOwner(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	for _, wt := range []WindowType{WtForm, WtPopup} {
		outer, surface := newFocusRevealHost(), newFocusRevealHost()
		surface.SetParent(outer)
		e := NewEdit()
		e.SetParent(surface)
		surface.wt = wt // Set AFTER wiring; never instantiate a native window in this test.
		e.SetFocus()
		if len(surface.targets) != 1 || len(outer.targets) != 0 {
			t.Fatal("focus crossed independent native ownership", wt)
		}
		surface.wt = WtInherit
	}
}

func TestFocusRevealCallbackRetirement(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	for _, mode := range []string{"transfer", "disable", "reparent-leaf", "reparent-branch"} {
		t.Run(mode, func(t *testing.T) {
			outer, inner, elsewhere := newFocusRevealHost(), newFocusRevealHost(), newFocusRevealHost()
			inner.SetParent(outer)
			branch := new(Widget)
			branch.Init(branch)
			branch.SetParent(inner)
			e, other := NewEdit(), NewEdit()
			e.SetParent(branch)
			inner.callback = func(IWidget) {
				switch mode {
				case "transfer":
					other.SetFocus()
				case "disable":
					e.SetEnabled(false)
				case "reparent-leaf":
					e.SetParent(elsewhere)
				case "reparent-branch":
					branch.SetParent(elsewhere)
				}
			}
			e.SetFocus()
			if len(inner.targets) != 1 || len(outer.targets) != 0 || len(elsewhere.targets) != 0 {
				t.Fatal("callback notified a stale or unsolicited focus owner")
			}
		})
	}
}

type redirectFocusEdit struct {
	Edit
	calls     int
	onChanged func(IWidget, IWidget)
}

func newRedirectFocusEdit() *redirectFocusEdit {
	e := new(redirectFocusEdit)
	e.Init(e)
	return e
}

func (e *redirectFocusEdit) OnFocusChanged(next, previous IWidget) {
	e.calls++
	if e.onChanged != nil {
		e.onChanged(next, previous)
	}
}

func TestFocusTransferDuringOldNotificationIsNotDeliveredTwice(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	a, requested, redirected := newRedirectFocusEdit(), newRedirectFocusEdit(), newRedirectFocusEdit()
	a.SetFocus()
	a.onChanged = func(next, previous IWidget) {
		if previous == a && next == requested {
			redirected.SetFocus()
		}
	}
	requested.SetFocus()
	if !redirected.HasFocus() || redirected.calls != 1 || requested.calls != 1 {
		// requested receives exactly its loss notification from the nested
		// transfer, but must not receive a stale gain after it has lost focus.
		t.Fatal("reentrant transition duplicated or revived focus", redirected.calls, requested.calls)
	}
}

type orderedFocusHost struct {
	Widget
	order []IWidget
}

func (h *orderedFocusHost) ChildFocusOrder() []IWidget { return h.order }

func TestLazyChildrenUseVisualFocusOrder(t *testing.T) {
	h := new(orderedFocusHost)
	h.Init(h)
	last, first := NewEdit(), NewEdit()
	last.SetParent(h) // Materialized by a filter before earlier rows.
	first.SetParent(h)
	h.order = []IWidget{first, last}
	if firstFocusable(h) != first || nextFocusable(h, first, true) != last || nextFocusable(h, last, false) != first {
		t.Fatal("Tab followed creation order instead of visual order")
	}
}
