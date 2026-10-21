//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"os"
	"strings"
	"testing"
)

// TestWin32KeyDownRunsTheWindowLayer guards a whole class of key handling that
// existed only in the GLFW backend: WM_KEYDOWN dispatched straight to the
// focused widget, so every gui.RegisterShortcut was inert on Windows — the
// IDE's Ctrl+S, Ctrl+Z and mode switches never reached their handlers — and Tab
// never moved focus. Both have to run ahead of focus routing, or a focused
// CodeEditor consumes the key for its own undo first.
//
// Keep this source-order guard alongside the actual HWND message test below.
// The window wrapper is required: direct dispatchShortcut bypasses modal and
// focused-control isolation.
func TestWin32KeyDownRunsTheWindowLayer(t *testing.T) {
	body := wndProcKeyDownCase(t)
	shortcut := strings.Index(body, "dispatchWindowShortcut(")
	tab := strings.Index(body, "VK_TAB")
	focus := strings.Index(body, "IEventKeyDown")

	if shortcut < 0 {
		t.Error("WM_KEYDOWN does not consult dispatchWindowShortcut; registered shortcuts are dead on Windows")
	}
	if tab < 0 {
		t.Error("WM_KEYDOWN does not handle Tab; focus cannot be moved from the keyboard on Windows")
	}
	if focus < 0 {
		t.Fatal("WM_KEYDOWN no longer forwards to the focused widget; update this test to match")
	}
	if shortcut > focus || tab > focus {
		t.Error("the window layer runs after focus routing; a focused editor will eat the key first")
	}
}

// wndProcKeyDownCase returns the text of the WM_KEYDOWN case in wndProcFunc.
func wndProcKeyDownCase(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	src := string(raw)
	start := strings.Index(src, "case win32.WM_KEYDOWN:")
	if start < 0 {
		t.Fatal("window_windows.go has no WM_KEYDOWN case")
	}
	rest := src[start:]
	end := strings.Index(rest, "\n\tcase win32.WM_KEYUP:")
	if end < 0 {
		t.Fatal("WM_KEYDOWN case is not terminated by WM_KEYUP")
	}
	return rest[:end]
}

func TestWindowsKeyLayerNativeShortcutRepeatAndFocus(t *testing.T) {
	editor := nativeModalOwner(t)
	editor.SetFocus()
	mods := currentMods()
	if mods != 0 {
		t.Fatalf("native fixture requires released modifiers, got %d", mods)
	}
	old := ShortcutHandler(0, KeyF6)
	t.Cleanup(func() { RegisterShortcut(0, KeyF6, old) })
	calls := 0
	RegisterShortcut(0, KeyF6, func() { calls++ })
	win32.SendMessage(editor.Window().hWnd, win32.WM_KEYDOWN, win32.VK_F6, 0)
	if calls != 1 {
		t.Fatal("real WM_KEYDOWN did not reach application shortcut")
	}
	win32.SendMessage(editor.Window().hWnd, win32.WM_KEYDOWN, win32.VK_F6, 1<<30)
	if calls != 1 {
		t.Fatal("key repeat re-executed application shortcut")
	}
	win32.SendMessage(editor.Window().hWnd, win32.WM_CHAR, 'x', 1)
	if editor.Text() != "x" || focusWidget != editor {
		t.Fatal("shortcut lost focused editor input")
	}
}
