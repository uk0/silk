//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsNativeCtrlTabNavigatesFocusedPage(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	isolateWindowsPopupState(t)
	tabs := NewTabWidget()
	tabs.SetVisible(false)
	tabs.SetSize(600, 350)
	a, b := NewEdit(), NewEdit()
	tabs.AddTab(a, "A", nil)
	tabs.AddTab(b, "B", nil)
	tabs.AttachWindow(WtForm)
	win := tabs.Window()
	if win == nil || win.hWnd == 0 {
		t.Fatal("missing native tab window")
	}
	t.Cleanup(win.destroy)
	// Logical visibility is sufficient for dispatch; no test desktop is needed.
	tabs.NakedWidget().hidden = false
	a.SetFocus()
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original, state [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	state[KeyCtrl] = 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, win32.VK_TAB, 1)
	if tabs.CurrentIndex() != 1 || focusWidget != b {
		t.Fatalf("native Ctrl+Tab: index=%d focus=%T", tabs.CurrentIndex(), focusWidget)
	}
	state[KeyShift] = 0x80
	set.Call(uintptr(unsafe.Pointer(&state[0])))
	win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, win32.VK_TAB, 1)
	if tabs.CurrentIndex() != 0 || focusWidget != a {
		t.Fatal("native reverse lost focus")
	}
	state[KeyShift] = 0
	set.Call(uintptr(unsafe.Pointer(&state[0])))
	win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, uintptr(KeyPageDown), 1<<30)
	if tabs.CurrentIndex() != 1 || focusWidget != b {
		t.Fatal("native repeat/PageDown failed")
	}
	win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, uintptr(KeyPageUp), 1)
	if tabs.CurrentIndex() != 0 || focusWidget != a {
		t.Fatal("native PageUp failed")
	}
}
