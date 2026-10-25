//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsGoToNativeChildInput(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("zero\r\n中🙂xyz\r\nlast")
	t.Cleanup(e.HideGoToLine)
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original, clear [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&clear[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	e.ShowGoToLine()
	for _, r := range "2:3" {
		win32.SendMessage(e.Window().hWnd, win32.WM_CHAR, uintptr(r), 1)
	}
	if e.gotoBar.query.Text() != "2:3" || e.Text() != "zero\r\n中🙂xyz\r\nlast" {
		t.Fatal("native field input reached source")
	}
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(KeyEnter), 1)
	if e.CursorLine() != 1 || e.CursorCol() != 2 || e.GoToLineVisible() || !e.HasFocus() {
		t.Fatal("native Enter did not navigate")
	}
	e.SetReadOnly(true)
	e.ShowGoToLine()
	for _, r := range "999" {
		win32.SendMessage(e.Window().hWnd, win32.WM_CHAR, uintptr(r), 1)
	}
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(KeyEnter), 1)
	if !e.GoToLineVisible() || e.CursorLine() != 1 || e.gotoBar.accept.IsEnabled() {
		t.Fatal("native invalid input jumped")
	}
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(KeyEsc), 1)
	if e.GoToLineVisible() || !e.HasFocus() || e.TextRevision() != 1 {
		t.Fatal("native cancel lost focus or changed text")
	}
}
