package gui

import "testing"

// A shown logical popup, without asking the window backend to create native
// resources. These tests execute real menu/button/action handlers.
func visibleActivationMenu() *Menu {
	m := NewPopupMenu()
	m.setVisible(true)
	return m
}

func TestDisabledActionTriggerCannotBypassButtonGuard(t *testing.T) {
	target := NewAction1("disabled command", nil)
	targetCalls, proxyCalls := 0, 0
	target.BindFunc0(func() { targetCalls++ })
	proxy := NewAction()
	proxy.BindAction(target)
	proxy.BindFunc0(func() { proxyCalls++ })
	target.SetEnabled(false)
	target.Trigger(nil)
	proxy.Trigger(nil)
	if targetCalls != 0 || proxyCalls != 0 {
		t.Fatalf("disabled action target/proxy callbacks = %d/%d", targetCalls, proxyCalls)
	}
	target.SetEnabled(true)
	proxy.Trigger(nil)
	if targetCalls != 1 || proxyCalls != 1 {
		t.Fatal("enabled bound action no longer forwards once")
	}
}

func TestHiddenMenuItemsCannotNavigateOrActivate(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	m := visibleActivationMenu()
	hidden := m.AddButton1("hidden", nil)
	visible := m.AddButton1("visible", nil)
	hidden.Hide()
	hiddenCalls, visibleCalls := 0, 0
	hidden.Action().BindFunc0(func() { hiddenCalls++ })
	visible.Action().BindFunc0(func() { visibleCalls++ })
	m.OnKeyDown(KeyDown, false)
	if m.highlight != 1 {
		t.Errorf("Down selected hidden item %d, want visible index 1", m.highlight)
	}
	m.highlight = 0 // visibility changed after an older highlight was recorded
	m.OnKeyDown(KeyEnter, false)
	if hiddenCalls != 0 {
		t.Errorf("hidden menu command ran %d times", hiddenCalls)
	}
	m.setVisible(false)
	m.highlight = 1
	m.OnKeyDown(KeyEnter, false)
	visible.OnKeyDown(KeySpace, false)
	if visibleCalls != 0 {
		t.Errorf("stale keyboard input ran a hidden popup command %d times", visibleCalls)
	}
}

func TestCommandButtonsIgnoreActivationKeyRepeat(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	b := NewButton1("toggle", nil)
	calls := 0
	b.Action().BindFunc1(func(action IAction) {
		calls++
		action.SetChecked(!action.IsChecked())
	})
	b.OnKeyDown(KeySpace, false)
	b.OnKeyDown(KeySpace, true)
	b.OnKeyDown(KeyEnter, true)
	if calls != 1 || !b.IsChecked() {
		t.Errorf("held activation key ran %d times, checked=%v", calls, b.IsChecked())
	}
	b.OnKeyDown(KeyEnter, false)
	if calls != 2 || b.IsChecked() {
		t.Fatal("a separate activation must run once and leave checked state to its action")
	}
	m := visibleActivationMenu()
	item := m.AddButton1("command", nil)
	menuCalls := 0
	item.Action().BindFunc0(func() { menuCalls++ })
	m.highlight = 0
	m.OnKeyDown(KeyEnter, true)
	if menuCalls != 0 || !m.IsVisible() {
		t.Fatal("repeated Enter activated or dismissed the menu")
	}
}

func TestMenuCallbackRunsOnceAfterEntirePopupChainIsHidden(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	root := visibleActivationMenu()
	parent := root.AddButton1("submenu", nil)
	sub := visibleActivationMenu()
	parent.SetSubPopup(sub)
	sub.SetParent(parent)
	sub.setVisible(true)
	leaf := sub.AddButton1("leaf", nil)
	calls := 0
	leaf.Action().BindFunc0(func() {
		calls++
		if root.IsVisible() || sub.IsVisible() {
			t.Error("callback entered before its popup chain was hidden")
		}
	})
	setMouseHoverWidget(leaf)
	leaf.OnLeftDown(5, 5)
	leaf.OnLeftUp(5, 5)
	leaf.OnLeftUp(5, 5) // A duplicate release must not repeat the command.
	leaf.OnKeyDown(KeyEnter, true)
	if calls != 1 {
		t.Fatalf("one activation produced %d callbacks", calls)
	}
}

func TestHiddenButtonCannotKeepAnArmedMousePress(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	button := NewButton1("command", nil)
	calls := 0
	button.Action().BindFunc0(func() { calls++ })
	setMouseHoverWidget(button)
	button.OnLeftDown(5, 5)
	button.Hide()
	button.Show()
	button.OnLeftUp(5, 5)
	if calls != 0 || button.IsPushed() {
		t.Fatal("a hide/show cycle resurrected an old mouse press")
	}
}

type actionDisablingMenu struct {
	Menu
	beforeHide func()
}

func (menu *actionDisablingMenu) OnHide() {
	menu.Menu.OnHide()
	menu.beforeHide()
}

func TestMenuHideCallbackCanDisablePendingAction(t *testing.T) {
	menu := new(actionDisablingMenu)
	menu.Menu.Init(menu)
	menu.wt = WtPopup
	button := menu.AddButton1("pending", nil)
	calls := 0
	button.Action().BindFunc0(func() { calls++ })
	menu.beforeHide = func() { button.Action().SetEnabled(false) }
	button.emit()
	if calls != 0 || menu.IsVisible() {
		t.Fatal("a command disabled while hiding its menu was still dispatched")
	}
}

func TestFocusedButtonEscapeCancelsDialogButPopupTakesPriority(t *testing.T) {
	oldHover, oldFocus := mouseHoverWidget, focusWidget
	t.Cleanup(func() { mouseHoverWidget, focusWidget = oldHover, oldFocus })
	dialog := NewDialog("owner", nil)
	button := dialog.AddButton("OK", DialogOK)
	button.SetFocus()
	button.OnKeyDown(KeyEsc, false)
	if !dialog.closed || dialog.result != DialogCancel {
		t.Fatal("a focused button swallowed its owning dialog's Escape")
	}
	dialog = NewDialog("owner", nil)
	menu := NewPopupMenu()
	menu.SetParent(dialog)
	menu.setVisible(true)
	item := menu.AddButton1("popup item", nil)
	item.SetFocus()
	item.OnKeyDown(KeyEsc, false)
	if menu.IsVisible() || dialog.closed {
		t.Fatal("Escape must close only the popup before reaching its owner dialog")
	}
}

func TestMouseMenuBarHeaderKeepsContentReturnFocus(t *testing.T) {
	oldFocus := focusWidget
	t.Cleanup(func() { focusWidget = oldFocus })
	content := NewEdit()
	content.SetFocus()
	bar := NewMenuBar()
	_, header := bar.AddSubMenu("Edit", nil, nil)
	header.OnLeftDown(0, 0)
	if focusWidget != content {
		t.Fatalf("mouse header stole return focus: %T", focusWidget)
	}
	// Keyboard focus remains possible; do not disable header accessibility.
	header.SetFocus()
	header.OnLeftDown(0, 0)
	if focusWidget != header {
		t.Fatal("explicit keyboard focus changed")
	}
	ordinary := NewButton1("ordinary", nil)
	ordinary.OnLeftDown(0, 0)
	if focusWidget != ordinary {
		t.Fatal("ordinary button lost mouse focus")
	}
	popup := visibleActivationMenu()
	_, nested := popup.AddSubMenu("Nested", nil, nil)
	nested.OnLeftDown(0, 0)
	if focusWidget != nested {
		t.Fatal("nested submenu focus contract changed")
	}
}
