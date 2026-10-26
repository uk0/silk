//go:build !windows

package gui

import "testing"

func nestedFocusPopup(parent IWidget, owner *Window) (*Menu, *Window) {
	m := NewMenu(false)
	m.SetParent(parent)
	_, win := testPopupWindow(m, WtPopup, owner)
	win.enabled = true
	return m, win
}

func TestNestedPopupChainReturnsNativeFocusBeforeChildrenHide(t *testing.T) {
	for _, depth := range []int{1, 2, 4} {
		t.Run(string(rune('0'+depth)), func(t *testing.T) {
			isolateFocusReturns(t)
			owner, field := focusReturnForm(t)
			menu, root := nestedFocusPopup(field, owner)
			root.popupRestoreFocus = field
			leaf, key := menu, root
			for range depth {
				child, popup := nestedFocusPopup(leaf, key)
				popup.popupRestoreFocus = leaf
				leaf, key = child, popup
			}
			leaf.SetFocus()
			focused := func(w *Window) bool { return w == key }
			r := prepareTransientFocusReturn(root, owner, field, focused)
			if r == nil {
				t.Fatal("focused descendant did not preserve the root popup's native return")
			}
			calls := 0
			ok := r.restorePopupOwnerBeforeHide(visibleFocusReturnWindow, focused, focusReturnKeyWindowFallback, func(w *Window) {
				if w != owner || !leaf.IsVisible() || focusWidget != leaf {
					t.Fatal("return must precede descendant teardown without changing logical focus")
				}
				key = w
				calls++
				noteNativeFocusTransfer() // Real GLFW focus callbacks change the generation.
			})
			if !ok || calls != 1 {
				t.Fatal("nested popup did not directly activate its surviving owner")
			}
			menu.Hide()
			if menu.IsVisible() || leaf.IsVisible() || focusWidget != field || key != owner {
				t.Fatal("closing the popup tree stranded native or logical focus")
			}
		})
	}
}

func TestNestedPopupFocusReturnDoesNotCrossIndependentForm(t *testing.T) {
	isolateFocusReturns(t)
	owner, field := focusReturnForm(t)
	menu, root := nestedFocusPopup(field, owner)
	independent, independentField := focusReturnForm(t)
	independent.widget.SetParent(menu)
	_, foreign := nestedFocusPopup(independentField, independent)
	if r := prepareTransientFocusReturn(root, owner, field, func(w *Window) bool { return w == foreign }); r != nil {
		t.Fatal("an independent form's popup authorized activation of a different owner")
	}
}

func TestNestedPopupFocusReturnStillHonorsNewIntent(t *testing.T) {
	for _, scenario := range []string{"background", "foreign-window", "new-generation", "new-popup", "owner-disabled"} {
		t.Run(scenario, func(t *testing.T) {
			isolateFocusReturns(t)
			owner, field := focusReturnForm(t)
			menu, root := nestedFocusPopup(field, owner)
			_, leaf := nestedFocusPopup(menu, root)
			key, active := leaf, true
			focused := func(w *Window) bool { return w == key }
			r := prepareTransientFocusReturn(root, owner, field, focused)
			if r == nil {
				t.Fatal("nested focus return missing")
			}
			switch scenario {
			case "background":
				active = false
			case "foreign-window":
				key, _ = focusReturnForm(t)
			case "new-generation":
				noteNativeFocusTransfer()
			case "new-popup":
				_, key = nestedFocusPopup(field, owner)
			case "owner-disabled":
				owner.enabled = false
			}
			calls := 0
			if r.restorePopupOwnerBeforeHide(visibleFocusReturnWindow, focused, func(bool) bool { return active }, func(*Window) { calls++ }) || calls != 0 {
				t.Fatal("stale popup close stole native focus")
			}
		})
	}
}
