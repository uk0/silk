//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsLineCopyDeleteShortcutsKeepAtomicSelectionHistory(t *testing.T) {
	e := nativeTextInputEditor(t)
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original, state [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	state[KeyCtrl], state[KeyShift] = 0x80, 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	e.SetText("a\r\nb")
	e.cursorLine = 1
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr('D'), 1)
	if e.Text() != "a\r\nb\r\nb" {
		t.Fatal("Ctrl+Shift+D", e.Text())
	}
	e.Undo()
	e.SetText("a\nb\nc\nd\n")
	e.hasSelection = true
	e.selStartLine = 1
	e.selEndLine = 3
	e.cursorLine = 3
	win32.SendMessage(e.Window().hWnd, win32.WM_KEYDOWN, uintptr('K'), 1)
	if e.Text() != "a\nd\n" {
		t.Fatal("Ctrl+Shift+K", e.Text())
	}
	e.Undo()
	if e.Text() != "a\nb\nc\nd\n" || !e.hasSelection || e.selStartLine != 1 || e.selEndLine != 3 {
		t.Fatal("native deletion Undo")
	}
}
