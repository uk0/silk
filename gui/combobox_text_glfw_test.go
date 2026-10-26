//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
)

func TestComboCharacterRoutingInsideModalShortcut(t *testing.T) {
	isolatePopupState(t)
	root := NewForm()
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	c := newTestCombo("Dark", "Light")
	c.SetEditable(false)
	c.SetParent(root)
	c.SetActiveIndex(0)
	c.SetFocus()
	// The dialog's event loop can still be nested in Cmd+Shift+P's callback.
	// Committed text must not inherit that outer callback's modifier snapshot.
	old := eventModifiers
	mods := glfw.ModSuper | glfw.ModControl | glfw.ModShift
	eventModifiers = &mods
	t.Cleanup(func() { eventModifiers = old })
	onChar(native, 'l')
	if c.ActiveIndex() != 1 {
		t.Fatal("committed text discarded by outer modal shortcut modifiers")
	}
	popupNative, popupWin := testPopupWindow(c.sub, WtPopup, win)
	popupWin.enabled = true
	c.sub.NakedWidget().hidden = false
	c.sub.SetFocus()
	onChar(popupNative, 'd')
	if c.sub.ActiveIndex() != 0 || c.ActiveIndex() != 1 {
		t.Fatal("owned popup text did not preview without committing")
	}
}

func TestComboNativeEscapeKeepsOwnerDialogOpen(t *testing.T) {
	isolatePopupState(t)
	d := NewDialog("Combo dialog", nil)
	win := attachHeadlessDialogWindow(d)
	win.inModal = true
	c := newTestCombo("Dark", "Light")
	c.SetEditable(false)
	c.SetParent(d)
	c.SetActiveIndex(0)
	c.SetFocus()
	popup := &Window{widget: c.sub, wt: WtPopup, popupOwner: win, popupRestoreFocus: c, enabled: true}
	c.sub.NakedWidget().win = popup
	c.sub.NakedWidget().wt = WtPopup
	c.sub.Show()
	c.sub.OnKeyDown(KeyEnd, false)
	if !win.dispatchEscape(KeyEsc, false) {
		t.Fatal("Escape was not handled")
	}
	if c.IsSubPopupVisible() || c.ActiveIndex() != 0 || !d.IsVisible() || !win.inModal || win.modalRet != nil || focusWidget != c {
		t.Fatalf("Escape crossed popup boundary: modal=%v result=%v focus=%T", win.inModal, win.modalRet, focusWidget)
	}
}

func TestComboPopupTabCancelsAndLeavesComposite(t *testing.T) {
	isolatePopupState(t)
	d := NewDialog("Combo dialog", nil)
	win := attachHeadlessDialogWindow(d)
	previous := NewEdit()
	previous.SetParent(d)
	c := newTestCombo("Dark", "Light")
	c.SetEditable(false)
	c.SetParent(d)
	c.SetActiveIndex(0)
	next := NewEdit()
	next.SetParent(d)
	for _, editable := range []bool{false, true} {
		c.SetEditable(editable)
		for _, reverse := range []bool{false, true} {
			c.SetFocus()
			c.sub.Show()
			c.sub.OnKeyDown(KeyEnd, false)
			if !c.sub.WantsTab(reverse) || c.IsSubPopupVisible() || c.ActiveIndex() != 0 {
				t.Fatal("Tab committed preview or retained popup")
			}
			want := IWidget(next)
			if reverse {
				want = previous
			}
			if focusWidget != want {
				t.Fatalf("reverse=%v focus=%T", reverse, focusWidget)
			}
			if c.OwnerWindow() != win {
				t.Fatal("lost native owner")
			}
		}
	}
}
