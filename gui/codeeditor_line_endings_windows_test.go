//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsLineEndingNativeEnterAndBackspace(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("one\r\ntwo\r\n")
	e.cursorCol = 3
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
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(KeyEnter), 1)
	if e.Text() != "one\r\n\r\ntwo\r\n" {
		t.Fatal("native Enter", e.Text())
	}
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(KeyBackSpace), 1)
	if e.Text() != "one\r\ntwo\r\n" {
		t.Fatal("native Backspace", e.Text())
	}
}
