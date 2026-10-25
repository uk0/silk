//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsReadOnlyNativeTextEditingAndSystemKeys(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("one\ntwo\n")
	e.DuplicateLines()
	before, rev := e.Text(), e.TextRevision()
	e.SetReadOnly(true)
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	for _, modifier := range []int{-1, KeyCtrl, KeyMenu} {
		var state [256]byte
		if modifier >= 0 {
			state[modifier] = 0x80
		}
		if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
			t.Fatal("SetKeyboardState")
		}
		if modifier == -1 {
			win32.SendMessage(e.Window().hWnd, win32.WM_CHAR, uintptr('X'), 1)
		}
		for _, key := range []int{KeyBackSpace, KeyDelete, KeyEnter, KeyTab, 'Z', 'Y', 'X', 'V', 'J', '/'} {
			win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr(key), 1)
		}
		if modifier == KeyMenu {
			win32.SendMessage(e.Window().hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1)
		}
	}
	if e.Text() != before || e.TextRevision() != rev || e.CanUndo() {
		t.Fatal("native edits escaped read-only", e.Text())
	}
	e.SetReadOnly(false)
	e.Undo()
	if e.Text() != "one\ntwo\n" {
		t.Fatal("native history lost")
	}
}
