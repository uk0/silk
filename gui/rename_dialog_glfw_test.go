//go:build !windows

package gui

import "testing"

func TestRenameDialogInvalidEnterAndClickStayOpen(t *testing.T) {
	saved := focusWidget
	t.Cleanup(func() { focusWidget = saved })
	b := NewInputBox()
	b.SetValidator(GoIdentifierValidator())
	b.SetMessage("for")
	win := attachHeadlessDialogWindow(b)
	defer win.destroy()
	win.inModal = true
	b.focusInput()
	b.edit.OnKeyDown(KeyEnter, false)
	b.onBtnClick("@ok")
	if win.modalRet != nil || !b.edit.HasFocus() {
		t.Fatal("invalid dialog closed")
	}
	b.edit.SelectAll()
	b.edit.OnTextInput("validName")
	b.edit.OnKeyDown(KeyEnter, false)
	if win.modalRet != true {
		t.Fatal("valid dialog not accepted")
	}
	win.inModal = true
	win.modalRet = nil
	b.SetMessage("_")
	b.resolveCancel()
	if win.modalRet != false {
		t.Fatal("invalid draft blocked cancellation")
	}
}
