//go:build windows

package gui

import (
	"runtime"
	"syscall"
	"testing"

	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/win32"
)

func dialogPolishNativeIcons(t *testing.T, w *Window) [2]win32.HICON {
	t.Helper()
	var icons [2]win32.HICON
	for _, kind := range []uintptr{win32.ICON_SMALL, win32.ICON_BIG} {
		icons[kind] = win32.HICON(win32.SendMessage(w.hWnd, win32.WM_GETICON, kind, 0))
		if icons[kind] == 0 || icons[kind] != w.ownedIcons[kind] || !windowsIconAlive(icons[kind]) {
			t.Fatal("window lacks a live individually owned native icon", kind)
		}
	}
	return icons
}
func TestDialogPolishNativeWindowsIconOwnership(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	isolateWindowsPopupState(t)
	old := appIcon
	t.Cleanup(func() { appIcon = old })
	app, parentIcon, explicit := paint.LoadIcon("document"), paint.LoadIcon("folder"), paint.LoadIcon("save")
	SetAppIcon(app)
	root := NewForm()
	root.SetVisible(false)
	root.AttachWindow(WtForm)
	owner := root.Window()
	defer owner.destroy()
	if owner.Icon() != app {
		t.Fatal("application icon missing on standalone form")
	}
	appHandles := dialogPolishNativeIcons(t, owner)
	root.SetIcon(parentIcon)
	if owner.Icon() != parentIcon {
		t.Fatal("live form icon did not update window")
	}
	for _, h := range appHandles {
		if windowsIconAlive(h) {
			t.Fatal("live replacement leaked old icon")
		}
	}
	ownerHandles := dialogPolishNativeIcons(t, owner)
	// Keep related HWNDs on this locked goroutine. t.Run starts a different
	// goroutine, which can migrate threads while its native owner is blocked.
	for _, kind := range []string{"form", "input", "message", "dialog", "nested"} {
		t.Logf("native icon case: %s", kind)
		func() {
			var child IWidget
			switch kind {
			case "input":
				child = NewInputBox()
			case "message":
				child = NewMessageBox()
			case "dialog":
				child = NewDialog("dialog", root)
			default:
				child = NewForm()
			}
			child.SetParent(root)
			child.SetVisible(false)
			child.AttachWindow(WtForm)
			win := child.Window()
			defer win.destroy()
			if win.Icon() != parentIcon || win.ParentWindow() != owner {
				t.Fatal("child lost owner identity")
			}
			handles := dialogPolishNativeIcons(t, win)
			for i, h := range handles {
				if h == ownerHandles[i] {
					t.Fatal("child borrowed owner native icon")
				}
			}
			setter := child.(interface{ SetIcon(paint.Icon) })
			setter.SetIcon(explicit)
			if win.Icon() != explicit {
				t.Fatal("explicit live override lost")
			}
			for _, h := range handles {
				if windowsIconAlive(h) {
					t.Fatal("override retained old native icon")
				}
			}
			if kind == "nested" {
				embedded := NewLabel("logical child")
				embedded.SetParent(child)
				nested := NewInputBox()
				nested.SetParent(embedded)
				nested.SetVisible(false)
				nested.AttachWindow(WtForm)
				nw := nested.Window()
				nh := dialogPolishNativeIcons(t, nw)
				if nw.Icon() != explicit || nw.ParentWindow() != win {
					t.Fatal("nested dialog did not inherit immediate native owner")
				}
				parentBefore := dialogPolishNativeIcons(t, win)
				nw.destroy()
				for _, h := range nh {
					if windowsIconAlive(h) {
						t.Fatal("nested window leaked icon")
					}
				}
				for _, h := range parentBefore {
					if !windowsIconAlive(h) {
						t.Fatal("nested close released parent icon")
					}
				}
			}
			setter.SetIcon(nil)
			if win.Icon() != parentIcon {
				t.Fatal("clearing explicit icon did not inherit owner")
			}
			handles = dialogPolishNativeIcons(t, win)
			win.destroy()
			if win.Icon() != nil {
				t.Fatal("destroyed window retained logical icon")
			}
			for _, h := range handles {
				if windowsIconAlive(h) {
					t.Fatal("destroyed child retained native icon")
				}
			}
			for _, h := range ownerHandles {
				if !windowsIconAlive(h) {
					t.Fatal("child destruction invalidated owner icon")
				}
			}
		}()
	}
	popup := NewLabel("popup")
	popup.SetParent(root)
	popup.SetVisible(false)
	popup.AttachWindow(WtPopup)
	if popup.Window().Icon() != nil || popup.Window().ownedIcons != [2]win32.HICON{} {
		t.Fatal("popup allocated title/taskbar icons")
	}
	popup.Window().destroy()
	SetAppIcon(nil)
	root.SetIcon(nil)
	if owner.Icon() != nil || owner.ownedIcons != [2]win32.HICON{} {
		t.Fatal("clearing a standalone icon retained explicit identity")
	}
	for _, kind := range []uintptr{win32.ICON_SMALL, win32.ICON_BIG} {
		if win32.SendMessage(owner.hWnd, win32.WM_GETICON, kind, 0) != 0 {
			t.Fatal("cleared form did not restore native class/default icon")
		}
	}
	for _, h := range ownerHandles {
		if windowsIconAlive(h) {
			t.Fatal("clearing form icon leaked owner handles")
		}
	}
}
func TestDialogPolishNativeWindowsRepeatedFormsDoNotLeakIcons(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	isolateWindowsPopupState(t)
	old := appIcon
	t.Cleanup(func() { appIcon = old })
	SetAppIcon(paint.LoadIcon("document"))
	get := syscall.NewLazyDLL("user32.dll").NewProc("GetGuiResources")
	count := func() uintptr { n, _, _ := get.Call(^uintptr(0), 0); return n }
	run := func() {
		w := NewInputBox()
		w.SetVisible(false)
		w.AttachWindow(WtForm)
		dialogPolishNativeIcons(t, w.Window())
		w.Window().destroy()
	}
	for i := 0; i < 5; i++ {
		run()
	}
	before := count()
	for i := 0; i < 50; i++ {
		run()
	}
	after := count()
	if after > before+4 {
		t.Fatalf("GDI icons leaked across form creation: %d -> %d", before, after)
	}
	t.Logf("50 inherited form icons GDI before=%d after=%d without forced GC", before, after)
}
