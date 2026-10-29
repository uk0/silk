//go:build !windows

package gui

import "testing"

type focusReturnObservedMenu struct {
	Menu
	beforeHide func()
}

func (m *focusReturnObservedMenu) Hide() {
	if m.beforeHide != nil {
		m.beforeHide()
	}
	m.Menu.Hide()
}

func TestPopupButtonRetainsFocusUntilHideThenRunsCallback(t *testing.T) {
	for _, input := range []string{"mouse", "menu-keyboard"} {
		t.Run(input, func(t *testing.T) {
			isolateFocusReturns(t)
			preview, chart := focusReturnForm(t)
			menu := new(focusReturnObservedMenu)
			menu.Init(menu)
			menu.SetParent(chart)
			_, popup := testPopupWindow(menu, WtPopup, preview)
			popup.popupRestoreFocus = chart
			button := menu.AddButton1("outdent", nil)
			menu.SetFocus()
			hides, calls := 0, 0
			menu.beforeHide = func() {
				hides++
				if !widgetWithinPopups(focusWidget, map[IWidget]bool{menu: true}) {
					t.Fatal("button restored focus before popup Hide could capture native return")
				}
			}
			button.Action().BindFunc0(func() {
				calls++
				if menu.IsVisible() || focusWidget != chart {
					t.Fatal("callback ran before menu hiding and logical focus restoration")
				}
			})
			if input == "mouse" {
				mouseHoverWidget = button
				button.OnLeftDown(1, 1)
				button.OnLeftUp(1, 1)
			} else {
				menu.OnKeyDown(KeyEnd, false)
				menu.OnKeyDown(KeyEnter, false)
			}
			if hides != 1 || calls != 1 {
				t.Fatalf("hide/callback counts = %d/%d", hides, calls)
			}
		})
	}
}

func isolateFocusReturns(t *testing.T) {
	t.Helper()
	isolatePopupState(t)
	oldGeneration, oldDepth, oldQuit := transientFocusGeneration, transientTeardownDepth, shouldQuit
	oldQueue := pendingNativeDestruction
	transientFocusGeneration, transientTeardownDepth, shouldQuit = 0, 0, false
	pendingNativeDestruction = nil
	t.Cleanup(func() {
		transientFocusGeneration, transientTeardownDepth, shouldQuit = oldGeneration, oldDepth, oldQuit
		pendingNativeDestruction = oldQueue
	})
}

func focusReturnForm(t *testing.T) (*Window, IWidget) {
	t.Helper()
	root := NewForm()
	_, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	child := NewButton()
	child.SetParent(root)
	return win, child
}

func visibleFocusReturnWindow(w *Window) bool {
	return w != nil && w.widget != nil && w.widget.IsVisible()
}

func focusReturnKeyWindowFallback(focused bool) bool { return focused }

func TestPopupHideRejectsForeignSavedFocus(t *testing.T) {
	isolateFocusReturns(t)
	_, editor := focusReturnForm(t)
	preview, chart := focusReturnForm(t)
	menu := NewMenu(false)
	menu.SetParent(chart)
	_, popup := testPopupWindow(menu, WtPopup, preview)
	popup.popupRestoreFocus = editor // Regression: global focus still in IDE.
	focusWidget = menu
	menu.Hide()
	if focusWidget != preview.widget || focusWidget.OwnerWindow() != preview {
		t.Fatal("preview menu restored an unrelated IDE widget")
	}
	if got := focusTargetForOwner(preview, chart); got != chart {
		t.Fatal("valid owner-local focus was discarded")
	}
}

func TestTransientFocusReturnAfterNativeDestroyRestoresPreview(t *testing.T) {
	isolateFocusReturns(t)
	ide, _ := focusReturnForm(t)
	preview, chart := focusReturnForm(t)
	dialog := NewDialog("edit task", chart)
	_, modal := testPopupWindow(dialog, WtForm, nil)
	modal.enabled = true
	button := NewButton()
	button.SetParent(dialog)
	focusWidget = button
	r := prepareTransientFocusReturn(modal, preview, chart, func(w *Window) bool { return w == modal })
	if r == nil {
		t.Fatal("focused modal did not capture its actual preview owner")
	}
	order := []string{}
	pendingNativeDestruction = append(pendingNativeDestruction, func() {
		duringTransientTeardown(func() {
			modal.destroy()
			noteNativeFocusTransfer() // Cocoa may automatically select IDE.
			order = append(order, "destroy")
		})
	})
	pendingNativeDestruction = append(pendingNativeDestruction, func() {
		r.restore(visibleFocusReturnWindow, func(w *Window) bool { return w == ide }, focusReturnKeyWindowFallback, func(w *Window) {
			if w != preview || focusWidget != chart || modal.widget != nil {
				t.Fatal("activation ran before teardown or chose the wrong owner/focus")
			}
			order = append(order, "focus-preview")
		})
	})
	flushNativeDestruction()
	if len(order) != 2 || order[0] != "destroy" || order[1] != "focus-preview" {
		t.Fatalf("wrong teardown/restore order: %v", order)
	}
	if preview.keyTarget() != chart {
		t.Fatal("owner keyboard target did not return to the chart")
	}
}

func TestTransientFocusReturnDoesNotStealNewFocus(t *testing.T) {
	for _, scenario := range []string{"user-window-transfer", "other-app", "new-modal", "new-popup", "owner-hidden", "owner-disabled", "owner-minimized", "owner-destroyed", "target-moved", "target-hidden", "target-disabled-parent", "new-owner-focus", "quit"} {
		t.Run(scenario, func(t *testing.T) {
			isolateFocusReturns(t)
			ide, editor := focusReturnForm(t)
			preview, chart := focusReturnForm(t)
			modal, _ := focusReturnForm(t)
			r := prepareTransientFocusReturn(modal, preview, chart, func(w *Window) bool { return w == modal })
			focusWidget = nil
			focused := ide
			var preserve IWidget
			switch scenario {
			case "user-window-transfer":
				noteNativeFocusTransfer()
			case "other-app":
				focused = nil
			case "new-modal":
				modal.inModal = true
			case "new-popup":
				menu := NewMenu(false)
				menu.SetParent(chart)
				testPopupWindow(menu, WtPopup, preview)
				focusWidget = menu
			case "owner-hidden":
				preview.widget.NakedWidget().hidden = true
			case "owner-disabled":
				preview.enabled = false
			case "owner-minimized":
				preview.iconified = true
			case "owner-destroyed":
				preview.destroy()
			case "target-moved":
				chart.SetParent(ide.widget)
			case "target-hidden":
				chart.Hide()
			case "target-disabled-parent":
				container := NewLabel("disabled section")
				container.SetParent(preview.widget)
				chart.SetParent(container)
				container.SetEnabled(false)
			case "new-owner-focus":
				editor.SetParent(preview.widget)
				editor.SetFocus()
				preserve = editor
			case "quit":
				shouldQuit = true
			}
			activations := 0
			r.restore(visibleFocusReturnWindow, func(w *Window) bool { return w == focused }, focusReturnKeyWindowFallback, func(w *Window) { activations++ })
			wantActivation := scenario == "target-moved" || scenario == "target-hidden" || scenario == "target-disabled-parent" || scenario == "new-owner-focus"
			if (activations == 1) != wantActivation {
				t.Fatalf("native activation count = %d", activations)
			}
			if preserve != nil && focusWidget != preserve {
				t.Fatal("return overwrote a newly focused child of the owner")
			}
			if scenario == "target-moved" || scenario == "target-hidden" || scenario == "target-disabled-parent" {
				if focusWidget != preview.widget {
					t.Fatal("invalid saved child did not fall back to its actual owner")
				}
			}
		})
	}
}

func TestTransientFocusReturnRequiresOriginalNativeFamilyFocus(t *testing.T) {
	isolateFocusReturns(t)
	preview, chart := focusReturnForm(t)
	modal, _ := focusReturnForm(t)
	if r := prepareTransientFocusReturn(modal, preview, chart, func(*Window) bool { return false }); r != nil {
		t.Fatal("background transient scheduled application activation")
	}
	if r := prepareTransientFocusReturn(modal, preview, chart, func(w *Window) bool { return w == preview }); r == nil {
		t.Fatal("non-activating popup lost its natively focused owner")
	}
}

func TestTransientTeardownDoesNotSuppressLaterUserFocusTransfers(t *testing.T) {
	isolateFocusReturns(t)
	duringTransientTeardown(func() { noteNativeFocusTransfer() })
	if transientFocusGeneration != 0 || transientTeardownDepth != 0 {
		t.Fatal("synchronous teardown focus confused with user intent")
	}
	noteNativeFocusTransfer()
	if transientFocusGeneration != 1 {
		t.Fatal("later user focus transfer did not invalidate pending returns")
	}
}

func TestTransientFocusReturnDistinguishesActiveAppFromKeyWindowGap(t *testing.T) {
	for _, tc := range []struct {
		name        string
		appActive   bool
		keyFocused  bool
		newIntent   bool
		wantRestore bool
	}{
		{"active-app-without-key-window", true, false, false, true},
		{"inactive-app-without-key-window", false, false, false, false},
		{"inactive-app-stale-key-window", false, true, false, false},
		{"new-focus-intent-in-active-app", true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateFocusReturns(t)
			preview, chart := focusReturnForm(t)
			popup, _ := focusReturnForm(t)
			r := prepareTransientFocusReturn(popup, preview, chart, func(w *Window) bool { return w == popup })
			if tc.newIntent {
				noteNativeFocusTransfer()
			}
			focusWidget = chart // Existing logical focus is already correct.
			activations := 0
			r.restore(visibleFocusReturnWindow,
				func(w *Window) bool { return tc.keyFocused && w == preview },
				func(bool) bool { return tc.appActive },
				func(w *Window) {
					if w != preview {
						t.Fatal("activated a foreign owner")
					}
					activations++
				})
			if (activations == 1) != tc.wantRestore {
				t.Fatalf("native restore calls = %d, want restore %v", activations, tc.wantRestore)
			}
			if focusWidget != chart {
				t.Fatal("native focus repair changed already-correct logical focus")
			}
		})
	}
}

func TestPopupHideReturnsNativeFocusInsideModalWithoutOuterDrain(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "modal", true: "nested-modal"}[nested], func(t *testing.T) {
			isolateFocusReturns(t)
			outer, _ := focusReturnForm(t)
			owner, field := focusReturnForm(t)
			owner.inModal = true
			if nested {
				outer.inModal = true
				outer.enabled = false
				owner.widget.SetParent(outer.widget)
			}
			combo := NewComboBox()
			combo.SetParent(field)
			combo.SetEditable(false)
			_, popup := testPopupWindow(combo.sub, WtPopup, owner)
			key := popup
			focused := func(w *Window) bool { return w == key }
			r := prepareTransientFocusReturn(popup, owner, combo, focused)
			focusWidget = combo
			activated := 0
			ok := r.restorePopupOwnerBeforeHide(visibleFocusReturnWindow, focused, focusReturnKeyWindowFallback, func(w *Window) { key = w; activated++ })
			if !ok || activated != 1 || key != owner || focusWidget != combo || !owner.inModal || len(pendingNativeDestruction) != 0 {
				t.Fatal("popup return waited for outer loop, changed logical focus or ended modal")
			}
		})
	}
}

func TestPopupPreHideActivationNeverOverridesNewNativeIntent(t *testing.T) {
	for _, scenario := range []string{"other-app", "other-window", "new-generation", "owner-hidden", "owner-disabled", "owner-minimized", "owner-destroyed", "quit", "modal-not-popup", "activation-failed"} {
		t.Run(scenario, func(t *testing.T) {
			isolateFocusReturns(t)
			owner, field := focusReturnForm(t)
			other, _ := focusReturnForm(t)
			menu := NewMenu(false)
			menu.SetParent(field)
			_, popup := testPopupWindow(menu, WtPopup, owner)
			key := popup
			focused := func(w *Window) bool { return w == key }
			r := prepareTransientFocusReturn(popup, owner, field, focused)
			appActive := true
			switch scenario {
			case "other-app":
				appActive = false
			case "other-window":
				key = other
			case "new-generation":
				noteNativeFocusTransfer()
			case "owner-hidden":
				owner.widget.NakedWidget().hidden = true
			case "owner-disabled":
				owner.enabled = false
			case "owner-minimized":
				owner.iconified = true
			case "owner-destroyed":
				owner.destroy()
			case "quit":
				shouldQuit = true
			case "modal-not-popup":
				popup.wt = WtForm
			}
			calls := 0
			ok := r.restorePopupOwnerBeforeHide(visibleFocusReturnWindow, focused, func(bool) bool { return appActive }, func(*Window) { calls++ })
			expected := 0
			if scenario == "activation-failed" {
				expected = 1
			}
			if ok || calls != expected {
				t.Fatalf("unexpected restoration: ok=%v calls=%d", ok, calls)
			}
		})
	}
}

func TestNestedModalReturnsOwnerBeforeNativeRetirement(t *testing.T) {
	for _, scenario := range []string{"nested", "background", "new-intent", "disabled", "foreign-focus"} {
		t.Run(scenario, func(t *testing.T) {
			isolateFocusReturns(t)
			_, outside := focusReturnForm(t)
			owner, target := focusReturnForm(t)
			owner.inModal = true
			modal, input := focusReturnForm(t)
			modal.widget.SetParent(target)
			focusWidget = input
			focused := modal
			r := prepareTransientFocusReturn(modal, owner, target, func(w *Window) bool { return w == focused })
			if r == nil {
				t.Fatal("missing return")
			}
			switch scenario {
			case "background":
				focused = nil
			case "new-intent":
				noteNativeFocusTransfer()
			case "disabled":
				owner.enabled = false
			case "foreign-focus":
				focusWidget = outside
			}
			calls := 0
			ok := r.restoreModalOwnerBeforeHide(visibleFocusReturnWindow, func(w *Window) bool { return w == focused }, focusReturnKeyWindowFallback, func(w *Window) {
				calls++
				if w != owner || focusWidget != target || modal.widget == nil {
					t.Fatal("incorrect activation order or owner")
				}
				focused = w
			})
			if ok != (scenario == "nested") || (calls == 1) != ok {
				t.Fatalf("return=%v calls=%d", ok, calls)
			}
			if ok {
				modal.destroy()
				if focusWidget != target {
					t.Fatal("child teardown cleared owner focus")
				}
			}
		})
	}
}
