//go:build windows

package gui

import (
	"syscall"
	"testing"
	"unsafe"

	"github.com/uk0/silk/win32"
)

func TestWindowsAltArrowMovesLinesAndCtrlAltStillAddsCaret(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("one\ntwo\nthree\n")
	dll := syscall.NewLazyDLL("user32.dll")
	get, set := dll.NewProc("GetKeyboardState"), dll.NewProc("SetKeyboardState")
	var original, state [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	state[KeyMenu] = 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	h := e.Window().hWnd
	win32.SendMessage(h, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1)
	win32.SendMessage(h, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1<<30)
	if e.Text() != "two\nthree\none\n" || e.CursorLine() != 2 || e.CursorCount() != 1 {
		t.Fatal("Alt movement/repeat", e.Text(), e.CursorLine(), e.CursorCount())
	}
	e.Undo()
	e.Undo()
	if e.Text() != "one\ntwo\nthree\n" || e.CursorLine() != 0 {
		t.Fatal("native move Undo")
	}
	state[KeyCtrl] = 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState ctrl")
	}
	win32.SendMessage(h, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1)
	if e.Text() != "one\ntwo\nthree\n" || e.CursorCount() != 2 {
		t.Fatal("Ctrl+Alt stopped adding carets")
	}
	for _, key := range []int{KeyF4, KeySpace, 'Q'} {
		if e.OnSystemKeyDown(key, false) {
			t.Fatal("consumed native Alt key", key)
		}
	}
}

type systemKeyEditorSpy struct {
	*CodeEditor
	keys    []int
	repeats []bool
}

func (s *systemKeyEditorSpy) OnSystemKeyDown(key int, repeat bool) bool {
	s.keys = append(s.keys, key)
	s.repeats = append(s.repeats, repeat)
	return true
}
func TestWindowsSystemKeyReachesOptInWidget(t *testing.T) {
	e := nativeTextInputEditor(t)
	w := e.Window()
	old := w.widget
	spy := &systemKeyEditorSpy{CodeEditor: e}
	// Replacing the root alone does not change logical focus: native fixture
	// focus is still the embedded editor, which correctly wins key routing.
	oldFocus := focusWidget
	w.widget, focusWidget = spy, spy
	t.Cleanup(func() { w.widget, focusWidget = old, oldFocus })
	if w.keyTarget() != spy || winMap[w.hWnd] != w {
		t.Fatal("native system-key fixture is not the receiving focus target")
	}
	win32.SendMessage(w.hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1)
	win32.SendMessage(w.hWnd, win32.WM_SYSKEYDOWN, uintptr(KeyDown), 1<<30)
	if len(spy.keys) != 2 || spy.keys[0] != KeyDown || spy.repeats[0] || !spy.repeats[1] {
		t.Fatalf("system-key routing: keys=%v repeat=%v", spy.keys, spy.repeats)
	}
}
