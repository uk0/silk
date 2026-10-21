//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
)

func TestModalDoesNotInvokeApplicationShortcuts(t *testing.T) {
	isolatePopupState(t)
	old := shortcutHandlers
	shortcutHandlers = make(map[shortcutKey]func())
	t.Cleanup(func() { shortcutHandlers = old })
	box := NewMessageBox()
	box.SetButtons([]string{"OK"})
	native, win := testPopupWindow(box, WtForm, nil)
	win.enabled, win.inModal = true, true
	calls := 0
	RegisterShortcut(0, KeyF6, func() { calls++ })
	onKey(native, glfw.KeyF6, 0, glfw.Press, 0)
	if calls != 0 {
		t.Fatal("modal shortcut invoked the application behind the dialog")
	}
	win.inModal = false
	onKey(native, glfw.KeyF6, 0, glfw.Press, 0)
	if calls != 1 {
		t.Fatal("modeless application shortcut stopped working")
	}
}

func TestMessageBoxModalEnterAndEscapeFromText(t *testing.T) {
	isolatePopupState(t)
	box := NewMessageBox()
	box.SetButtons([]string{"OK"})
	native, win := testPopupWindow(box, WtForm, nil)
	win.enabled, win.inModal = true, true
	box.focusMessage()
	onKey(native, glfw.KeyEnter, 0, glfw.Press, 0)
	if win.modalRet != "OK" {
		t.Fatal("read-only text swallowed Enter")
	}
	win.inModal = true
	win.modalRet = nil
	box.focusMessage()
	onKey(native, glfw.KeyEscape, 0, glfw.Press, 0)
	if win.modalRet != "" {
		t.Fatal("read-only text swallowed Escape")
	}
}

func TestModalPopupCannotReenableApplicationShortcuts(t *testing.T) {
	isolatePopupState(t)
	box := NewMessageBox()
	_, owner := testPopupWindow(box, WtForm, nil)
	owner.enabled, owner.inModal = true, true
	popup := NewForm()
	native, win := testPopupWindow(popup, WtPopup, owner)
	win.enabled = true
	old := ShortcutHandler(0, KeyF6)
	t.Cleanup(func() { RegisterShortcut(0, KeyF6, old) })
	calls := 0
	RegisterShortcut(0, KeyF6, func() { calls++ })
	onKey(native, glfw.KeyF6, 0, glfw.Press, 0)
	if calls != 0 {
		t.Fatal("modal popup escaped the application shortcut boundary")
	}
}
