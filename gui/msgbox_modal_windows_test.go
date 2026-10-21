//go:build windows

package gui

import (
	"github.com/uk0/silk/win32"
	"testing"
)

func TestWindowsMessageBoxModalKeyboardAndShortcutIsolation(t *testing.T) {
	for _, key := range []int{KeyEnter, KeyEsc} {
		t.Run(map[int]string{KeyEnter: "Enter", KeyEsc: "Escape"}[key], func(t *testing.T) {
			editor := nativeModalOwner(t)
			box := NewMessageBox()
			box.SetParent(editor)
			box.SetMessage("[SILK-FS004] Destination already exists.\nChoose another name.")
			box.SetButtons([]string{"OK"})
			old := ShortcutHandler(0, KeyF6)
			t.Cleanup(func() { RegisterShortcut(0, KeyF6, old) })
			hostCalls := 0
			RegisterShortcut(0, KeyF6, func() { hostCalls++ })
			Post(func() {
				win := box.Window()
				if win == nil || !win.inModal {
					t.Error("message box did not enter native modal loop")
					return
				}
				if editor.Window().IsEnabled() || focusWidget != box.edit {
					t.Error("message box did not isolate owner/focus")
				}
				win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, win32.VK_F6, 0)
				if hostCalls != 0 {
					t.Error("modal key invoked application shortcut")
				}
				win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, uintptr(key), 0)
				want := ""
				if key == KeyEnter {
					want = "OK"
				}
				if win.modalRet != want {
					t.Errorf("key did not end modal with %q: %v", want, win.modalRet)
					win.EndModal("TEST-FALLBACK")
				}
			})
			result := box.ShowModal()
			want := ""
			if key == KeyEnter {
				want = "OK"
			}
			if result != want || box.LastError() != nil {
				t.Fatalf("modal result=%q, error=%v", result, box.LastError())
			}
			if !editor.Window().IsEnabled() || focusWidget != editor || box.Window() != nil {
				t.Fatal("message teardown did not release owner/window")
			}
			win32.SendMessage(editor.Window().hWnd, win32.WM_CHAR, 'x', 1)
			if editor.Text() != "x" {
				t.Fatal("editor input not restored")
			}
		})
	}
}
