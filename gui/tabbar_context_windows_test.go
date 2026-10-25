//go:build windows

package gui

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/uk0/silk/win32"
)

func TestWindowsTabContextSystemF10AndRepeat(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	isolateWindowsPopupState(t)
	b := overflowTabs(3)
	b.SetVisible(false)
	b.AttachWindow(WtForm)
	win := b.Window()
	if win == nil || win.hWnd == 0 {
		t.Fatal("missing native HWND")
	}
	t.Cleanup(win.destroy)
	b.NakedWidget().hidden = false
	b.SetFocus()
	b.SetActiveTab(1)
	old := showTabBarPopup
	t.Cleanup(func() { showTabBarPopup = old; b.SetContextMenuCallback(nil) })
	calls, index := 0, -1
	showTabBarPopup = func(*Menu, float64, float64) { calls++ }
	b.SetContextMenuCallback(func(_ *TabBar, m *Menu, i int) { index = i; m.AddButton1("Action", nil) })
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original, state [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	state[KeyShift] = 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	win32.SendMessage(win.hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyF10), 1)
	win32.SendMessage(win.hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyF10), 1<<30)
	if calls != 1 || index != 1 {
		t.Fatalf("system F10 routed %d times to %d; want 1/1", calls, index)
	}
	state[KeyMenu] = 0x80
	set.Call(uintptr(unsafe.Pointer(&state[0])))
	win32.SendMessage(win.hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyF10), 1)
	if calls != 1 {
		t.Fatal("Alt chord consumed")
	}
}
