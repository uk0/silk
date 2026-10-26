//go:build windows

package gui

import (
	"os"
	"syscall"
	"testing"

	"github.com/uk0/silk/win32"
)

func TestWindowsModalRestoresNativeForegroundOwner(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_MODAL") != "1" {
		t.Skip("requires interactive desktop with SILK_TEST_NATIVE_MODAL=1")
	}
	editor := nativeModalOwner(t)
	owner := editor.Window()
	user := syscall.NewLazyDLL("user32.dll")
	foreground := user.NewProc("GetForegroundWindow")
	activate := user.NewProc("SetForegroundWindow")
	activate.Call(uintptr(owner.hWnd))
	if h, _, _ := foreground.Call(); win32.HWND(h) != owner.hWnd {
		t.Fatal("test owner never acquired foreground")
	}
	dlg := NewDialog("Native modal return", editor)
	dlg.SetContent(NewEdit())
	dlg.SetVisible(false)
	dlg.AttachWindow(WtForm)
	modal := dlg.Window()
	modal.ShowModal(func() {
		h, _, _ := foreground.Call()
		if win32.HWND(h) != modal.hWnd {
			t.Error("modal never acquired foreground")
		}
		modal.EndModal(DialogOK)
	})
	h, _, _ := foreground.Call()
	if win32.HWND(h) != owner.hWnd {
		t.Fatalf("native foreground escaped modal owner: got=%v owner=%v", h, owner.hWnd)
	}
}

func TestWindowsModalPreservesNativeIndependentForeground(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_MODAL") != "1" {
		t.Skip("requires interactive desktop with SILK_TEST_NATIVE_MODAL=1")
	}
	editor := nativeModalOwner(t)
	other := NewEdit()
	other.SetVisible(false)
	other.SetSize(240, 120)
	other.AttachWindow(WtForm)
	defer other.Window().destroy()
	dlg := NewDialog("Native modal background", editor)
	dlg.SetContent(NewEdit())
	dlg.SetVisible(false)
	dlg.AttachWindow(WtForm)
	modal := dlg.Window()
	modal.ShowModal(func() {
		other.Show()
		syscall.NewLazyDLL("user32.dll").NewProc("SetForegroundWindow").Call(uintptr(other.Window().hWnd))
		if win32.GetForegroundWindow() != other.Window().hWnd {
			t.Error("independent window never acquired foreground")
		}
		modal.EndModal(DialogCancel)
	})
	if win32.GetForegroundWindow() != other.Window().hWnd {
		t.Fatal("closing background modal stole native foreground")
	}
}

func TestWindowsNestedModalRestoresNativeImmediateOwner(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_MODAL") != "1" {
		t.Skip("requires interactive desktop with SILK_TEST_NATIVE_MODAL=1")
	}
	editor := nativeModalOwner(t)
	syscall.NewLazyDLL("user32.dll").NewProc("SetForegroundWindow").Call(uintptr(editor.Window().hWnd))
	outer := NewDialog("Native outer", editor)
	outer.SetContent(NewEdit())
	outer.SetVisible(false)
	outer.AttachWindow(WtForm)
	outerWin := outer.Window()
	outerWin.ShowModal(func() {
		inner := NewDialog("Native inner", outer)
		inner.SetContent(NewEdit())
		inner.SetVisible(false)
		inner.AttachWindow(WtForm)
		innerWin := inner.Window()
		innerWin.ShowModal(func() {
			if win32.GetForegroundWindow() != innerWin.hWnd {
				t.Error("inner never acquired foreground")
			}
			innerWin.EndModal(DialogCancel)
		})
		if win32.GetForegroundWindow() != outerWin.hWnd || !outerWin.IsEnabled() || editor.Window().IsEnabled() {
			t.Error("inner did not return native activation to immediate owner")
		}
		outerWin.EndModal(DialogOK)
	})
	if win32.GetForegroundWindow() != editor.Window().hWnd {
		t.Fatal("outer did not return native activation to main window")
	}
}
