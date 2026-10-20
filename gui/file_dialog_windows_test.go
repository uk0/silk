//go:build windows

package gui

import "testing"

func TestFileDialogOwnerResolvesPopupChain(t *testing.T) {
	form := &Window{hWnd: 42, wt: WtForm}
	popup := &Window{hWnd: 43, wt: WtPopup, popupOwner: form}
	nested := &Window{hWnd: 44, wt: WtPopup, popupOwner: popup}
	for _, w := range []*Window{form, popup, nested} {
		if got := fileDialogOwnerWindow(w); got != 42 {
			t.Fatalf("owner=%d want 42", got)
		}
	}
	if fileDialogOwnerWindow(nil) != 0 || fileDialogOwnerWindow(&Window{wt: WtPopup}) != 0 {
		t.Fatal("missing owner manufactured an HWND")
	}
	form.hWnd = 0
	if fileDialogOwnerWindow(nested) != 0 {
		t.Fatal("destroyed form still owns chooser")
	}
}
