//go:build !windows

package gui

import "testing"

func TestInputBoxEnterAcceptsAndEscapeCancelsOwnModal(t *testing.T) {
	saved := focusWidget
	t.Cleanup(func() { focusWidget = saved })
	box := NewInputBox()
	box.SetMessage("name.go")
	win := attachHeadlessDialogWindow(box)
	win.inModal = true
	box.focusInput()
	box.edit.OnKeyDown(KeyEnter, false)
	if win.modalRet != true {
		t.Fatal("Enter did not accept input")
	}
	win.inModal = true
	win.modalRet = nil
	if !win.dispatchEscape(KeyEsc, false) || win.modalRet != false {
		t.Fatal("Escape did not cancel the input dialog")
	}
	win.destroy()
}
