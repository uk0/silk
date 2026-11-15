//go:build windows

package gui

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestExtensionFileFilterPreservesLibraryAndLegacyFormats(t *testing.T) {
	if !reflect.DeepEqual(extensionFileFilterUTF16(""), silkUIFilterUTF16()) {
		t.Fatal("legacy form filter changed")
	}
	parts := strings.Split(string(utf16.Decode(extensionFileFilterUTF16("silkhmilibrary"))), "\x00")
	want := []string{"silkhmilibrary files (*.silkhmilibrary)", "*.silkhmilibrary", "All Files (*.*)", "*.*", "", ""}
	if !reflect.DeepEqual(parts, want) {
		t.Fatal("library filter", parts)
	}
}

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
