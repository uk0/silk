package gui

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/win32"
)

// GetIconInfo returns copies of both bitmaps, which the caller must delete.
func windowsIconAlive(icon win32.HICON) bool {
	if icon == 0 {
		return false
	}
	var info win32.ICONINFO
	getInfo := syscall.NewLazyDLL("user32.dll").NewProc("GetIconInfo")
	ok, _, _ := getInfo.Call(uintptr(icon), uintptr(unsafe.Pointer(&info)))
	if info.HbmMask != 0 {
		win32.DeleteObject(win32.HGDIOBJ(info.HbmMask))
	}
	if info.HbmColor != 0 {
		win32.DeleteObject(win32.HGDIOBJ(info.HbmColor))
	}
	return ok != 0
}

func TestWindowsWindowReleasesOwnedIconsOnDestroy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	isolateWindowsPopupState(t)
	widget := NewLabel("icon lifetime")
	widget.SetVisible(false)
	widget.AttachWindow(WtForm)
	win := widget.Window()
	if win == nil || win.hWnd == 0 {
		t.Fatal("native hidden window not created")
	}
	t.Cleanup(win.destroy)
	win.SetIcon(paint.LoadIcon("document"))
	small := win32.HICON(win32.SendMessage(win.hWnd, win32.WM_GETICON, win32.ICON_SMALL, 0))
	large := win32.HICON(win32.SendMessage(win.hWnd, win32.WM_GETICON, win32.ICON_BIG, 0))
	if !windowsIconAlive(small) || !windowsIconAlive(large) {
		t.Fatal("new window icons are not valid native objects")
	}
	win.SetIcon(paint.LoadIcon("folder"))
	if windowsIconAlive(small) || windowsIconAlive(large) {
		t.Fatal("icon replacement retained the previous owned handles")
	}
	small, large = win.ownedIcons[win32.ICON_SMALL], win.ownedIcons[win32.ICON_BIG]
	if !windowsIconAlive(small) || !windowsIconAlive(large) {
		t.Fatal("replacement icon handles are invalid")
	}
	win.destroy()
	if windowsIconAlive(small) || windowsIconAlive(large) {
		t.Fatal("destroyed window retained its generated icon handles")
	}
	win.SetIcon(paint.LoadIcon("document"))
	if win.ownedIcons != [2]win32.HICON{} {
		t.Fatal("setting an icon on a destroyed window allocated native handles")
	}
}

func TestWindowsWindowIconRetirementIsBounded(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	isolateWindowsPopupState(t)
	getResources := syscall.NewLazyDLL("user32.dll").NewProc("GetGuiResources")
	count := func() uintptr {
		n, _, _ := getResources.Call(^uintptr(0), 0)
		return n
	}
	icon := paint.LoadIcon("document")
	run := func() {
		widget := NewLabel("hidden icon resource probe")
		widget.SetVisible(false)
		widget.AttachWindow(WtPopup)
		win := widget.Window()
		if win == nil || win.hWnd == 0 {
			t.Fatal("native hidden probe window not created")
		}
		win.SetIcon(icon)
		win.destroy()
	}
	for i := 0; i < 5; i++ {
		run()
	}
	baseline := count()
	for i := 0; i < 50; i++ {
		run()
	}
	if after := count(); after > baseline+4 {
		t.Fatalf("hidden icon windows grew GDI objects: before=%d after=%d", baseline, after)
	}
	t.Logf("50 icon windows GDI before=%d after=%d (no forced GC)", baseline, count())
}
