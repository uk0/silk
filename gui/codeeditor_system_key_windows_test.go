//go:build windows

package gui

import (
	"testing"

	"github.com/uk0/silk/win32"
)

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
