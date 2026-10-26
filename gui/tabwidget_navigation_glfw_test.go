//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"runtime"
	"testing"
)

func TestWindowCtrlTabNavigatesFocusedPage(t *testing.T) {
	isolatePopupState(t)
	root := NewForm()
	tabs := NewTabWidget()
	tabs.SetParent(root)
	tabs.SetSize(800, 500)
	a, b := NewCodeEditor(), NewCodeEditor()
	a.SetFilePath("a.txt")
	b.SetFilePath("b.txt")
	a.SetText("alpha")
	b.SetText("beta")
	tabs.AddTab(a, "A", nil)
	tabs.AddTab(b, "B", nil)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	a.SetFocus()
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if tabs.CurrentIndex() != 1 || focusWidget != b {
		t.Fatalf("Ctrl+Tab failed: index=%d focus=%T", tabs.CurrentIndex(), focusWidget)
	}
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl|glfw.ModShift)
	if tabs.CurrentIndex() != 0 || focusWidget != a {
		t.Fatal("Ctrl+Shift+Tab did not return typing focus")
	}
	if a.Text() != "alpha" || b.Text() != "beta" {
		t.Fatal("navigation modified document")
	}
}

func TestWindowTabNavigationOwnershipAndModifiers(t *testing.T) {
	isolatePopupState(t)
	old := shortcutHandlers
	shortcutHandlers = make(map[shortcutKey]func())
	t.Cleanup(func() { shortcutHandlers = old })
	root := NewForm()
	outer := NewTabWidget()
	outer.SetParent(root)
	outer.SetSize(800, 500)
	inner := NewTabWidget()
	a, b := NewEdit(), NewEdit()
	inner.AddTab(a, "A", nil)
	inner.AddTab(b, "B", nil)
	outer.AddTab(inner, "Inner", nil)
	outer.AddTab(NewEdit(), "Outer", nil)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	a.SetFocus()
	onKey(native, glfw.KeyPageDown, 0, glfw.Press, glfw.ModControl)
	if inner.CurrentIndex() != 1 || outer.CurrentIndex() != 0 || focusWidget != b {
		t.Fatal("nearest group not navigated")
	}
	onKey(native, glfw.KeyPageUp, 0, glfw.Repeat, glfw.ModControl)
	if inner.CurrentIndex() != 0 || focusWidget != a {
		t.Fatal("repeat or PageUp lost")
	}
	for _, mods := range []glfw.ModifierKey{0, glfw.ModControl | glfw.ModAlt, glfw.ModControl | glfw.ModSuper, glfw.ModControl | glfw.ModShift} {
		onKey(native, glfw.KeyPageDown, 0, glfw.Press, mods)
		if inner.CurrentIndex() != 0 {
			t.Fatal("unowned modifier chord consumed")
		}
	}
	inner.RemoveTab(1)
	a.SetFocus()
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if outer.CurrentIndex() != 0 || focusWidget != a {
		t.Fatal("single inner tab escaped into outer group")
	}
	inner.AddTab(b, "B", nil)
	win.inModal = true
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if inner.CurrentIndex() != 1 {
		t.Fatal("local modal tabs blocked")
	}
	win.inModal = false
	popup := NewPopupMenu()
	popup.SetParent(b)
	_, popupWin := testPopupWindow(popup, WtPopup, win)
	popupWin.enabled = true
	// NewPopupMenu starts hidden; this fixture represents an opened popup.
	popup.NakedWidget().hidden = false
	popup.SetFocus()
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if inner.CurrentIndex() != 1 || focusWidget != popup {
		t.Fatal("popup allowed background tab switch")
	}
	popup.Hide()
	a.SetFocus()
	inner.SetCurrentIndex(0)
	other := NewForm()
	otherNative, otherWin := testPopupWindow(other, WtForm, nil)
	otherWin.enabled = true
	onKey(otherNative, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if inner.CurrentIndex() != 0 {
		t.Fatal("unrelated native window navigated foreign tabs")
	}
	a.SetFocus()
	root.SetEnabled(false)
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if inner.CurrentIndex() != 0 {
		t.Fatal("disabled ancestor navigated")
	}
	root.SetEnabled(true)
	a.SetFocus()
	calls := 0
	mods := uint8(0)
	if runtime.GOOS != "darwin" {
		mods = ModAction
	}
	RegisterShortcut(mods, KeyTab, func() { calls++ })
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	onKey(native, glfw.KeyTab, 0, glfw.Repeat, glfw.ModControl)
	if calls != 1 || inner.CurrentIndex() != 0 {
		t.Fatal("host shortcut or repeat priority lost")
	}
}

func TestWindowTabContextKeyboardShortcutAndRepeat(t *testing.T) {
	isolatePopupState(t)
	root := NewForm()
	b := overflowTabs(3)
	b.SetParent(root)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	old := showTabBarPopup
	t.Cleanup(func() { showTabBarPopup = old; b.SetContextMenuCallback(nil) })
	count, index := 0, -1
	showTabBarPopup = func(*Menu, float64, float64) { count++ }
	b.SetContextMenuCallback(func(_ *TabBar, m *Menu, i int) { index = i; m.AddButton1("Action", nil) })
	b.SetActiveTab(2)
	b.SetFocus()
	onKey(native, glfw.KeyF10, 0, glfw.Press, glfw.ModShift)
	onKey(native, glfw.KeyF10, 0, glfw.Repeat, glfw.ModShift)
	if count != 1 || index != 2 {
		t.Fatal("keyboard context/repeat", count, index)
	}
	for _, mods := range []glfw.ModifierKey{0, glfw.ModShift | glfw.ModAlt, glfw.ModShift | glfw.ModControl, glfw.ModShift | glfw.ModSuper} {
		onKey(native, glfw.KeyF10, 0, glfw.Press, mods)
	}
	if count != 1 {
		t.Fatal("unowned modifiers opened context menu")
	}
}
