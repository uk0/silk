//go:build windows

package gui

import (
	"os"
	"testing"

	"github.com/uk0/silk/win32"
)

func nativeModalOwner(t *testing.T) *CodeEditor {
	t.Helper()
	editor := nativeTextInputEditor(t)
	// The text-input fixture is intentionally hidden. Modal restoration must
	// instead start with a visible owner. A launcher STARTUPINFO can override
	// the first ShowWindow call (notably go test under Windows OpenSSH).
	editor.SetVisible(true)
	if !editor.Window().IsVisible() {
		editor.SetVisible(true)
	}
	if !editor.Window().IsVisible() && os.Getenv("SILK_TEST_NATIVE_MODAL") != "1" {
		t.Skip("visible modal owner unavailable: rerun on interactive desktop with SILK_TEST_NATIVE_MODAL=1")
	}
	if !editor.Window().IsVisible() || !editor.Window().IsEnabled() {
		t.Fatal("modal owner must be visible and enabled")
	}
	return editor
}

func TestWindowsNestedModalRestoresImmediateOwner(t *testing.T) {
	editor := nativeModalOwner(t)
	outer := NewDialog("Outer", editor)
	outerInput := NewEdit()
	outer.SetContent(outerInput)
	outer.SetInitialFocus(outerInput)
	outer.SetVisible(false)
	outer.AttachWindow(WtForm)
	outerWin := outer.Window()
	outerWin.ShowModal(func() {
		outer.activateInitialFocus()
		inner := NewDialog("Inner", outer)
		innerInput := NewEdit()
		inner.SetContent(innerInput)
		inner.SetInitialFocus(innerInput)
		inner.SetVisible(false)
		inner.AttachWindow(WtForm)
		innerWin := inner.Window()
		innerWin.ShowModal(func() {
			inner.activateInitialFocus()
			if outerWin.IsEnabled() || editor.Window().IsEnabled() {
				t.Error("nested modal left an ancestor enabled")
			}
			innerWin.EndModal(DialogCancel)
		})
		if focusWidget != outerInput || !outerWin.IsEnabled() || editor.Window().IsEnabled() {
			t.Error("inner teardown lost outer input or re-enabled the main window early")
		}
		outerWin.EndModal(DialogCancel)
	})
	if focusWidget != editor || !editor.Window().IsEnabled() {
		t.Fatal("outer teardown did not return to the main editor")
	}
}

func TestWindowsModalReturnsKeyboardToOwner(t *testing.T) {
	for _, result := range []DialogResult{DialogOK, DialogCancel} {
		t.Run(dialogResultKey(result), func(t *testing.T) {
			editor := nativeModalOwner(t)
			dlg := NewDialog("Modal focus", editor)
			input := NewEdit()
			dlg.SetContent(input)
			dlg.SetInitialFocus(input)
			dlg.SetVisible(false)
			dlg.AttachWindow(WtForm)
			win := dlg.Window()
			owner := editor.Window()
			ret := win.ShowModal(func() {
				dlg.activateInitialFocus()
				if owner.IsEnabled() {
					t.Error("modal did not disable its owner")
				}
				if focusWidget != input {
					t.Error("modal initial focus missing")
				}
				win.EndModal(result)
			})
			if ret != result || focusWidget != editor {
				t.Fatalf("modal result=%v; restored focus=%T; owner visible=%v enabled=%v hwnd=%v widget=%T", ret, focusWidget, owner.IsVisible(), owner.IsEnabled(), owner.hWnd, owner.widget)
			}
			win32.SendMessage(editor.Window().hWnd, win32.WM_CHAR, 'x', 1)
			if editor.Text() != "x" {
				t.Fatalf("owner text input lost after modal: %q", editor.Text())
			}
		})
	}
}

func TestWindowsModalPreservesExplicitFocusChange(t *testing.T) {
	editor := nativeModalOwner(t)
	destination := NewEdit()
	destination.SetParent(editor)
	dlg := NewDialog("Modal focus", editor)
	input := NewEdit()
	dlg.SetContent(input)
	dlg.SetVisible(false)
	dlg.AttachWindow(WtForm)
	win := dlg.Window()
	win.ShowModal(func() { input.SetFocus(); destination.SetFocus(); win.EndModal(DialogOK) })
	if focusWidget != destination {
		t.Fatal("modal teardown overwrote explicit focus")
	}
}

func TestWindowsModalDoesNotRestoreHiddenOwner(t *testing.T) {
	editor := nativeModalOwner(t)
	dlg := NewDialog("Modal focus", editor)
	input := NewEdit()
	dlg.SetContent(input)
	dlg.SetVisible(false)
	dlg.AttachWindow(WtForm)
	win := dlg.Window()
	win.ShowModal(func() {
		input.SetFocus()
		editor.SetVisible(false)
		win.EndModal(DialogCancel)
	})
	if focusWidget != nil || editor.Window().IsVisible() {
		t.Fatal("modal teardown restored hidden owner or its input")
	}
}
