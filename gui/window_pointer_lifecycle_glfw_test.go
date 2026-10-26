//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestFirstClickAfterPopupDoesNotRequireMouseMovement(t *testing.T) {
	for _, stale := range []string{"same-button-no-hover", "old-widget"} {
		t.Run(stale, func(t *testing.T) {
			isolatePopupState(t)
			root := NewLabel("owner")
			root.SetSize(400, 300)
			native, win := testPopupWindow(root, WtForm, nil)
			win.enabled = true
			win.pointer = pointerEventState{x: 55, y: 20, valid: true}
			button := NewButton1("menu", nil)
			button.SetBounds(40, 10, 40, 25)
			button.SetParent(root)
			clicks := 0
			button.Action().BindFunc0(func() { clicks++ })
			lastMouseWidget = button
			if stale == "old-widget" {
				lastMouseWidget = NewLabel("stale modal content")
			}
			mouseHoverWidget = nil
			// AX/CUA and real users can click a stationary cursor after a
			// popup closes: no intervening GLFW cursor callback is required.
			for i := 0; i < 2; i++ {
				onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
				onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
			}
			if clicks != 2 {
				t.Fatalf("stationary clicks after popup close=%d, want 2", clicks)
			}
		})
	}
}

func TestDestroyClearsModalPointerAndDescendantCapture(t *testing.T) {
	isolatePopupState(t)
	owner := NewLabel("owner")
	testPopupWindow(owner, WtForm, nil)
	outside := NewButton1("unrelated capture", nil)
	outside.SetParent(owner)
	dialog := NewLabel("dialog")
	dialog.SetParent(owner)
	_, win := testPopupWindow(dialog, WtForm, nil)
	button := NewButton1("modal child", nil)
	button.SetParent(dialog)
	captureStack = []IWidget{outside, button}
	focusWidget, lastMouseWidget, mouseHoverWidget = button, button, button
	pointerWindow = win
	win.autoCaptured, win.toCapture, win.mouseEntered = true, true, true
	win.pointer = pointerEventState{x: 10, y: 20, valid: true}
	win.destroy()
	if len(captureStack) != 1 || captureStack[0] != outside {
		t.Fatal("modal destroy kept descendant capture or discarded unrelated capture")
	}
	if focusWidget != nil || lastMouseWidget != nil || mouseHoverWidget != nil || pointerWindow != nil {
		t.Fatal("modal destroy retained focus/hover/pointer references to hidden descendants")
	}
	if win.autoCaptured || win.toCapture || win.mouseEntered || win.pointer.valid {
		t.Fatal("destroyed window retained an armed mouse gesture")
	}
}

func TestClickRetargetingPreservesAnActiveCapture(t *testing.T) {
	isolatePopupState(t)
	root := NewLabel("owner")
	root.SetSize(400, 300)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	win.pointer = pointerEventState{x: 55, y: 20, valid: true}
	captured := newCapturedButtonProbe(root, "captured")
	captured.SetBounds(0, 0, 20, 20)
	captured.SetParent(root)
	underPointer := NewButton1("under pointer", nil)
	underPointer.SetBounds(40, 10, 40, 25)
	underPointer.SetParent(root)
	otherClicks := 0
	underPointer.Action().BindFunc0(func() { otherClicks++ })
	captureStack = []IWidget{captured}
	lastMouseWidget, mouseHoverWidget = captured, captured
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
	if captured.downs != 1 || captured.ups != 1 || captured.clicks != 0 || otherClicks != 0 {
		t.Fatal("capture must retain event delivery without activating outside its bounds")
	}
}

func TestDestroyDoesNotClearIndependentPreviewInput(t *testing.T) {
	isolatePopupState(t)
	owner := NewLabel("owner")
	_, main := testPopupWindow(owner, WtForm, nil)
	preview := NewLabel("preview")
	preview.SetParent(owner)
	_, previewWin := testPopupWindow(preview, WtForm, nil)
	button := NewButton1("preview input", nil)
	button.SetParent(preview)
	captureStack = []IWidget{button}
	focusWidget, lastMouseWidget, mouseHoverWidget = button, button, button
	pointerWindow = previewWin
	main.destroy()
	if len(captureStack) != 1 || captureStack[0] != button || focusWidget != button || lastMouseWidget != button || mouseHoverWidget != button || pointerWindow != previewWin {
		t.Fatal("destroy cleared an independent preview's input state")
	}
}

func TestMenuCommandModalCloseAllowsSamePointOpenerClick(t *testing.T) {
	isolatePopupState(t)
	root := NewLabel("owner")
	root.SetSize(500, 400)
	native, owner := testPopupWindow(root, WtForm, nil)
	owner.enabled = true
	owner.pointer = pointerEventState{x: 55, y: 20, valid: true}
	opener := NewButton1("menu", nil)
	opener.SetParent(root)
	opener.SetBounds(40, 10, 40, 25)
	var menu *Menu
	var menuWin, dialogWin *Window
	var dialog *Dialog
	var dialogNative *glfw.Window
	opens := 0
	opener.Action().BindFunc0(func() {
		opens++
		menu = NewPopupMenu()
		menu.SetParent(opener)
		_, menuWin = testPopupWindow(menu, WtPopup, owner)
		menuWin.popupRestoreFocus = opener
		menu.setVisible(true)
		menu.AddButton1("about", nil).Action().BindFunc0(func() {
			menuWin.destroy() // autoClose teardown occurs before the callback
			dialog = NewDialog("about", root)
			close := dialog.AddButton("Close", DialogOK)
			dialog.SetSize(400, 250)
			dialogNative, dialogWin = testPopupWindow(dialog, WtForm, nil)
			dialogWin.enabled, dialogWin.inModal = true, true
			x, y := close.MapToWindow(5, 5)
			dialogWin.pointer = pointerEventState{x: x, y: y, valid: true}
			dialog.SetFocus()
		})
		menu.PushCapture()
		menu.SetFocus()
	})
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
	onKey(native, glfw.KeyEnd, 0, glfw.Press, 0)
	onKey(native, glfw.KeyEnter, 0, glfw.Press, 0)
	if dialog == nil {
		t.Fatal("menu command did not create its dialog")
	}
	onMouseButton(dialogNative, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(dialogNative, glfw.MouseButtonLeft, glfw.Release, 0)
	if !dialog.closed {
		t.Fatal("modal Close pointer gesture did not execute")
	}
	dialogWin.destroy()
	opener.SetFocus()
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
	if opens != 2 || !menu.IsVisible() {
		t.Fatalf("same-point reopen after modal close: opens=%d, capture=%T, hover=%T", opens, curCapture(), mouseHoverWidget)
	}
}
