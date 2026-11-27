//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func isolatePopupState(t *testing.T) {
	t.Helper()
	oldMap, oldCapture, oldFocus := winMap, captureStack, focusWidget
	oldHover, oldMouse, oldTooltip := mouseHoverWidget, lastMouseWidget, currentTooltip.Load()
	oldActive, oldPointer := activeForm, pointerWindow
	winMap = make(map[*glfw.Window]*Window)
	captureStack, focusWidget, mouseHoverWidget, lastMouseWidget = nil, nil, nil, nil
	currentTooltip.Store(nil)
	t.Cleanup(func() {
		winMap, captureStack, focusWidget = oldMap, oldCapture, oldFocus
		mouseHoverWidget, lastMouseWidget = oldHover, oldMouse
		currentTooltip.Store(oldTooltip)
		activeForm, pointerWindow = oldActive, oldPointer
	})
}

// Register a Window without a native handle: callbacks can be exercised on
// the real widget/ownership/capture paths without opening windows off-thread.
func testPopupWindow(widget IWidget, wt WindowType, owner *Window) (*glfw.Window, *Window) {
	native := new(glfw.Window)
	win := &Window{widget: widget, wt: wt, popupOwner: owner}
	widget.NakedWidget().win = win
	widget.NakedWidget().wt = wt
	winMap[native] = win
	return native, win
}

func TestOwnerLifecycleDismissesOnlyItsPopupTree(t *testing.T) {
	for _, event := range []string{"move", "resize", "set-pos", "set-size", "hide", "destroy", "minimize"} {
		t.Run(event, func(t *testing.T) {
			isolatePopupState(t)
			owner := NewLabel("main")
			owner.SetSize(400, 300)
			native, main := testPopupWindow(owner, WtForm, nil)
			editor := NewLabel("focus before popup")
			editor.SetParent(owner)
			menu := NewMenu(false)
			menu.SetParent(owner)
			_, menuWin := testPopupWindow(menu, WtPopup, main)
			menuWin.popupRestoreFocus = editor
			sub := NewMenu(false)
			sub.SetParent(menu)
			_, subWin := testPopupWindow(sub, WtPopup, menuWin)
			subWin.popupRestoreFocus = menu
			combo := NewLabel("combo popup")
			combo.SetParent(owner)
			_, comboWin := testPopupWindow(combo, WtPopup, main)
			// A true child Form (preview/modal) is an independent surface;
			// its own popup must not be dismissed along with the main menu.
			preview := NewLabel("preview")
			preview.SetParent(owner)
			_, previewWin := testPopupWindow(preview, WtForm, nil)
			otherMenu := NewMenu(false)
			otherMenu.SetParent(preview)
			_, otherWin := testPopupWindow(otherMenu, WtPopup, previewWin)
			captureStack = []IWidget{menu, sub}
			focusWidget, lastMouseWidget = sub, sub
			switch event {
			case "move":
				onWindowMove(native, 30, 40)
			case "resize":
				onWindowResize(native, 450, 350)
			case "set-pos":
				main.SetPos(30, 40)
			case "set-size":
				main.SetSize(450, 350)
			case "hide":
				owner.Hide()
			case "destroy":
				main.destroy()
			case "minimize":
				onWindowIconify(native, true)
			}
			for _, popup := range []IWidget{menu, sub, combo} {
				if popup.IsVisible() {
					t.Fatalf("%s retained popup %T", event, popup)
				}
			}
			if menu.Window() != menuWin || sub.Window() != subWin || combo.Window() != comboWin {
				t.Fatal("dismiss destroyed a reusable popup")
			}
			if !preview.IsVisible() || preview.Window() != previewWin || !otherMenu.IsVisible() || otherMenu.Window() != otherWin {
				t.Fatal("main lifecycle modified an independent Form or its popup")
			}
			if len(captureStack) != 0 || lastMouseWidget != nil {
				t.Fatal("dismiss left stale popup capture/pointer routing")
			}
			if event == "destroy" {
				if focusWidget != nil {
					t.Fatal("destroy retained keyboard focus in the closed owner")
				}
			} else if focusWidget != editor {
				t.Fatalf("dismiss did not restore pre-popup focus: %T", focusWidget)
			}
			// Ordinary Hide preserves the instance; reopening can reuse it.
			combo.Show()
			if !combo.IsVisible() || combo.Window() != comboWin {
				t.Fatal("dismiss made a reusable ComboBox popup unusable")
			}
		})
	}
}

func TestUnchangedOwnerGeometryDoesNotDismissPopup(t *testing.T) {
	isolatePopupState(t)
	owner := NewLabel("owner")
	owner.SetBounds(10, 20, 400, 300)
	native, main := testPopupWindow(owner, WtForm, nil)
	menu := NewMenu(false)
	menu.SetParent(owner)
	testPopupWindow(menu, WtPopup, main)
	onWindowMove(native, 10, 20)
	onWindowResize(native, 400, 300)
	onWindowIconify(native, false)
	if !menu.IsVisible() {
		t.Fatal("duplicate geometry/unminimize callbacks dismissed an unchanged popup")
	}
}

func TestOwnerDismissClearsTooltipReferenceWithoutLayoutChild(t *testing.T) {
	isolatePopupState(t)
	owner := NewLabel("owner")
	owner.SetSize(240, 100)
	_, main := testPopupWindow(owner, WtForm, nil)
	tip := newOwnedToolTip(hoverToolTipAnchor{main, 50, 50}, owner, "hint")
	if tip == nil {
		t.Fatal("missing owner-local tooltip")
	}
	replaceExplicitToolTip(tip)
	if len(owner.Children()) != 0 || tip.owner != owner || tip.label.Window() != nil || tip.label.Parent() != nil {
		t.Fatal("tooltip either lacks an owner or entered the owner's layout children")
	}
	main.dismissOwnedPopups()
	if currentTooltip.Load() != nil || IsToolTipVisible() {
		t.Fatal("owner dismissal retained a tooltip reference")
	}
}

func TestPopupHideRestoresFocusAndCaptureWithoutDestroy(t *testing.T) {
	isolatePopupState(t)
	owner := NewLabel("owner")
	_, main := testPopupWindow(owner, WtForm, nil)
	menu := NewMenu(false)
	menu.SetParent(owner)
	_, menuWin := testPopupWindow(menu, WtPopup, main)
	menuWin.popupRestoreFocus = owner
	focusWidget, lastMouseWidget = menu, menu
	captureStack = []IWidget{menu}
	menu.Hide()
	if focusWidget != owner || lastMouseWidget != nil || len(captureStack) != 0 {
		t.Fatal("normal popup close left keyboard or pointer input in its hidden window")
	}
	if menu.Window() != menuWin {
		t.Fatal("normal popup close destroyed a reusable window")
	}
}

func TestDeferredFocusCheckKeepsInternalPopupTransfer(t *testing.T) {
	for _, active := range []string{"owner", "menu", "submenu", "other-app"} {
		t.Run(active, func(t *testing.T) {
			isolatePopupState(t)
			owner := NewLabel("owner")
			_, main := testPopupWindow(owner, WtForm, nil)
			menu := NewMenu(false)
			menu.SetParent(owner)
			_, menuWin := testPopupWindow(menu, WtPopup, main)
			sub := NewMenu(false)
			sub.SetParent(menu)
			_, subWin := testPopupWindow(sub, WtPopup, menuWin)
			focused := map[string]*Window{"owner": main, "menu": menuWin, "submenu": subWin}[active]
			main.dismissPopupsIfUnfocused(func(win *Window) bool { return win == focused })
			wantVisible := active != "other-app"
			if menu.IsVisible() != wantVisible || sub.IsVisible() != wantVisible {
				t.Fatal("focus check either broke a normal menu click or left a floating popup over another app")
			}
		})
	}
}

func TestFocusLossDefersPopupDismissalPastNativeCallback(t *testing.T) {
	isolatePopupState(t)
	oldTasks, oldWake := uiTasks, uiWakeupFn
	uiTasks, uiWakeupFn = nil, nil
	t.Cleanup(func() { uiTasks, uiWakeupFn = oldTasks, oldWake })
	owner := NewLabel("owner")
	native, main := testPopupWindow(owner, WtForm, nil)
	menu := NewMenu(false)
	menu.SetParent(owner)
	testPopupWindow(menu, WtPopup, main)
	onWindowFocus(native, false)
	onWindowFocus(native, false)
	onWindowFocus(native, false)
	if !menu.IsVisible() || len(uiTasks) != 1 {
		t.Fatal("blur must coalesce to one deferred family check, not dismiss inside a native callback")
	}
	drainUITasks() // zero-native fixture: none of the family has OS focus
	if menu.IsVisible() {
		t.Fatal("deferred check retained an unfocused floating popup")
	}
	if main.popupFocusCheckPending {
		t.Fatal("completed focus check prevents future focus events")
	}
}

func TestReusablePopupRefreshesOwnerAfterReparenting(t *testing.T) {
	isolatePopupState(t)
	one, two := NewLabel("one"), NewLabel("two")
	_, first := testPopupWindow(one, WtForm, nil)
	_, second := testPopupWindow(two, WtForm, nil)
	menu := NewMenu(false)
	menu.SetParent(one)
	_, popup := testPopupWindow(menu, WtPopup, first)
	menu.Hide()
	menu.SetParent(two)
	// The real hidden-to-visible native path refreshes this before Show.
	popup.refreshPopupOwner()
	menu.Show()
	if popup.popupOwner != second {
		t.Fatal("reused popup retained its creation-time owner")
	}
	first.dismissOwnedPopups()
	if !menu.IsVisible() {
		t.Fatal("former owner dismissed a popup now belonging to another window")
	}
	second.dismissOwnedPopups()
	if menu.IsVisible() {
		t.Fatal("new owner failed to dismiss its reused popup")
	}
	menu.SetParent(nil)
	popup.refreshPopupOwner()
	if popup.popupOwner != nil {
		t.Fatal("unparented popup retained a stale owner reference")
	}
}
