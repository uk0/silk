package gui

import (
	"runtime"
	"testing"

	"github.com/uk0/silk/win32"
)

func isolateWindowsPopupState(t *testing.T) {
	t.Helper()
	// Go tests run outside the package-init UI thread. Each isolated fixture
	// owns its own locked native thread, never a migrated test goroutine.
	runtime.LockOSThread()
	thread := win32.GetCurrentThreadId()
	oldOwner := uiThreadOwner
	uiThreadOwner = func() bool { return win32.GetCurrentThreadId() == thread }
	t.Cleanup(func() { uiThreadOwner = oldOwner; runtime.UnlockOSThread() })
	oldMap, oldCapture, oldFocus := winMap, captureStack, focusWidget
	oldHover, oldMouse, oldTooltip, oldActive := mouseHoverWidget, lastMouseWidget, currentTooltip, activeForm
	winMap = make(map[win32.HWND]*Window)
	captureStack, focusWidget, mouseHoverWidget, lastMouseWidget, currentTooltip = nil, nil, nil, nil, nil
	t.Cleanup(func() {
		winMap, captureStack, focusWidget = oldMap, oldCapture, oldFocus
		mouseHoverWidget, lastMouseWidget, currentTooltip, activeForm = oldHover, oldMouse, oldTooltip, oldActive
	})
}

// A zero-handle attachment exercises widget ownership without creating or
// displaying a native HWND. Map keys are only registration identifiers.
func attachWindowsLifecycleFixture(widget IWidget, wt WindowType, owner *Window) *Window {
	win := &Window{widget: widget, wt: wt, popupOwner: owner}
	widget.NakedWidget().win, widget.NakedWidget().wt, widget.NakedWidget().hidden = win, wt, false
	winMap[win32.HWND(len(winMap)+1)] = win
	return win
}

func TestWindowsDestroyClearsOnlyItsOwnInputAndAttachment(t *testing.T) {
	isolateWindowsPopupState(t)
	owner := NewLabel("owner")
	attachWindowsLifecycleFixture(owner, WtForm, nil)
	outside := NewButton1("other window input", nil)
	outside.SetParent(owner)
	dialog := NewLabel("dialog")
	dialog.SetParent(owner)
	win := attachWindowsLifecycleFixture(dialog, WtForm, nil)
	button := NewButton1("dialog input", nil)
	button.SetParent(dialog)
	captureStack = []IWidget{outside, button}
	backing := captureStack[:cap(captureStack)]
	focusWidget, lastMouseWidget, mouseHoverWidget = button, button, button
	win.autoCaptured, win.toCapture, win.mouseEntered = true, true, true
	win.destroy()
	if len(captureStack) != 1 || captureStack[0] != outside || focusWidget != nil || lastMouseWidget != nil || mouseHoverWidget != nil {
		t.Fatal("destroy retained modal input or cleared an unrelated form's capture")
	}
	if dialog.IsVisible() || dialog.Window() != nil || dialog.Parent() != owner || win.widget != nil {
		t.Fatal("closed dialog has a stale native attachment, lost owner, or visible inline ghost")
	}
	if win.autoCaptured || win.toCapture || win.mouseEntered {
		t.Fatal("closed window retained an armed mouse gesture")
	}
	if backing[1] != nil {
		t.Fatal("capture stack retains a removed dialog in its backing array")
	}
	next := attachWindowsLifecycleFixture(dialog, WtForm, nil)
	win.destroy()
	if dialog.Window() != next || !dialog.IsVisible() {
		t.Fatal("old destruction damaged a replacement modal attachment")
	}
	// DetachWindow clears the attachment first, intentionally returning the
	// same visible widget to inline mode. Destruction must not hide it.
	dialog.NakedWidget().win, dialog.NakedWidget().wt = nil, WtInherit
	next.destroy()
	if !dialog.IsVisible() {
		t.Fatal("native teardown hid a widget already detached for inline reuse")
	}
}

func TestWindowsDestroyPreservesIndependentFormInput(t *testing.T) {
	isolateWindowsPopupState(t)
	owner := NewLabel("owner")
	win := attachWindowsLifecycleFixture(owner, WtForm, nil)
	preview := NewLabel("preview")
	preview.SetParent(owner)
	attachWindowsLifecycleFixture(preview, WtForm, nil)
	button := NewButton1("preview input", nil)
	button.SetParent(preview)
	captureStack = []IWidget{button}
	focusWidget, lastMouseWidget, mouseHoverWidget = button, button, button
	win.destroy()
	if len(captureStack) != 1 || captureStack[0] != button || focusWidget != button || lastMouseWidget != button || mouseHoverWidget != button {
		t.Fatal("destroy used the widget tree rather than independent native ownership")
	}
}

func TestWindowsOwnerDismissesOnlyItsReusablePopupTree(t *testing.T) {
	isolateWindowsPopupState(t)
	owner := NewLabel("owner")
	main := attachWindowsLifecycleFixture(owner, WtForm, nil)
	menu := NewLabel("menu")
	menu.SetParent(owner)
	menuWin := attachWindowsLifecycleFixture(menu, WtPopup, main)
	menuWin.popupRestoreFocus = owner
	sub := NewLabel("submenu")
	sub.SetParent(menu)
	subWin := attachWindowsLifecycleFixture(sub, WtPopup, menuWin)
	subWin.popupRestoreFocus = menu
	preview := NewLabel("preview")
	preview.SetParent(owner)
	previewWin := attachWindowsLifecycleFixture(preview, WtForm, nil)
	otherMenu := NewLabel("independent menu")
	otherMenu.SetParent(preview)
	attachWindowsLifecycleFixture(otherMenu, WtPopup, previewWin)
	captureStack = []IWidget{menu, sub}
	focusWidget, lastMouseWidget, mouseHoverWidget = sub, sub, sub
	main.dismissOwnedPopups()
	if menu.IsVisible() || sub.IsVisible() || menu.Window() != menuWin || sub.Window() != subWin {
		t.Fatal("dismiss either retained or destroyed a reusable popup")
	}
	if !preview.IsVisible() || !otherMenu.IsVisible() {
		t.Fatal("dismiss crossed an independent form boundary")
	}
	if len(captureStack) != 0 || lastMouseWidget != nil || mouseHoverWidget != nil || focusWidget != owner {
		t.Fatal("dismiss retained hidden popup input or failed to restore focus")
	}
	menu.SetParent(preview)
	menuWin.refreshPopupOwner()
	if menuWin.popupOwner != previewWin {
		t.Fatal("reusable popup retained its creation-time owner")
	}
	menu.SetParent(nil)
	menuWin.refreshPopupOwner()
	if menuWin.popupOwner != nil {
		t.Fatal("unparented popup retained an owner")
	}
}

func TestWindowsFirstClickAfterPopupRefreshesTargetAndHover(t *testing.T) {
	for _, stale := range []string{"same-widget-no-hover", "old-modal-widget"} {
		t.Run(stale, func(t *testing.T) {
			isolateWindowsPopupState(t)
			owner := NewLabel("owner")
			owner.SetSize(400, 300)
			win := attachWindowsLifecycleFixture(owner, WtForm, nil)
			button := NewButton1("menu", nil)
			button.SetBounds(40, 10, 40, 25)
			button.SetParent(owner)
			clicks := 0
			button.Action().BindFunc0(func() { clicks++ })
			lastMouseWidget = button
			if stale == "old-modal-widget" {
				lastMouseWidget = NewLabel("old modal content")
			}
			pos := uintptr(55 | 20<<16)
			for i := 0; i < 2; i++ {
				win.on_WM_LBUTTONDOWN(win32.WM_LBUTTONDOWN, 0, pos)
				win.on_WM_LBUTTONUP(win32.WM_LBUTTONUP, 0, pos)
			}
			if clicks != 2 {
				t.Fatalf("stationary clicks after popup = %d, want 2", clicks)
			}
		})
	}
}

func TestWindowsDirtyAPIAllowsUnattachedWindow(t *testing.T) {
	win := new(Window)
	win.MarkDirtyRect(0, 0, 40, 30)
	win.MarkDirtyRect(0, 0, 0, -1)
	win.MarkFullDirty()
	win.Update()
	win.UpdateRect(0, 0, 40, 30)
}

func TestWindowsKeyStateUsesNativeHighAndLowBits(t *testing.T) {
	for _, tc := range []struct {
		bits          uint16
		down, toggled bool
	}{
		{0, false, false}, {1, false, true},
		{0x8000, true, false}, {0x8001, true, true},
		{0x0100, false, false}, {0x0101, false, true},
	} {
		if down, toggled := decodeWin32KeyState(int16(tc.bits)); down != tc.down || toggled != tc.toggled {
			t.Fatalf("GetKeyState(%#x) = %t/%t, want %t/%t", tc.bits, down, toggled, tc.down, tc.toggled)
		}
	}
}

func TestWindowsPopupFocusChecksAreDeferredAndCoalesced(t *testing.T) {
	isolateWindowsPopupState(t)
	oldTasks, oldWake := uiTasks, uiWakeupFn
	uiTasks, uiWakeupFn = nil, nil
	t.Cleanup(func() { uiTasks, uiWakeupFn = oldTasks, oldWake })
	owner := NewLabel("owner")
	main := attachWindowsLifecycleFixture(owner, WtForm, nil)
	menu := NewLabel("menu")
	menu.SetParent(owner)
	popup := attachWindowsLifecycleFixture(menu, WtPopup, main)
	main.queuePopupFocusCheck()
	popup.queuePopupFocusCheck()
	main.queuePopupFocusCheck()
	if len(uiTasks) != 1 || !menu.IsVisible() || !main.popupFocusCheckPending {
		t.Fatal("blur must queue one owner-family check rather than dismiss inside the callback")
	}
	drainUITasks()
	if menu.IsVisible() || main.popupFocusCheckPending {
		t.Fatal("deferred check left an unfocused popup visible or prevented future checks")
	}
}
