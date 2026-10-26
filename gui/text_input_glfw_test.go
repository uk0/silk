//go:build !windows

package gui

import (
	"testing"
	"unicode/utf8"
)

// Exercise the registered character callback, not Edit.OnTextInput directly.
func TestGLFWCharacterRoutingRejectsStaleFocus(t *testing.T) {
	for _, scenario := range []string{"foreign-form", "no-focus", "hidden", "hidden-parent", "hidden-grandparent", "disabled", "disabled-parent", "disabled-grandparent", "detached"} {
		t.Run(scenario, func(t *testing.T) {
			isolatePopupState(t)
			root := NewEdit()
			native, win := testPopupWindow(root, WtForm, nil)
			win.enabled = true
			grandparent, parent := NewLabel("grandparent"), NewLabel("parent")
			grandparent.SetParent(root)
			parent.SetParent(grandparent)
			field := NewEdit()
			field.SetParent(parent)
			field.SetFocus()
			switch scenario {
			case "foreign-form":
				_, other := testPopupWindow(grandparent, WtForm, nil)
				other.enabled = true
			case "no-focus":
				focusWidget = nil
			case "hidden":
				field.NakedWidget().hidden = true
			case "hidden-parent":
				parent.NakedWidget().hidden = true
			case "hidden-grandparent":
				grandparent.NakedWidget().hidden = true
			case "disabled":
				field.NakedWidget().disabled = true
			case "disabled-parent":
				parent.NakedWidget().disabled = true
			case "disabled-grandparent":
				grandparent.NakedWidget().disabled = true
			case "detached":
				field.SetParent(nil)
			}
			onChar(native, 'x')
			if field.Text() != "" || root.Text() != "" {
				t.Fatalf("character escaped focus scope: field=%q root=%q", field.Text(), root.Text())
			}
		})
	}
}

func TestGLFWCharacterRoutingUnicodeAndOwnedPopups(t *testing.T) {
	isolatePopupState(t)
	root := NewEdit()
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	field := NewEdit()
	field.SetParent(root)
	field.SetFocus()
	for _, r := range "qa_beta.go:4:6 中🙂" {
		onChar(native, r)
	}
	if field.Text() != "qa_beta.go:4:6 中🙂" {
		t.Fatalf("text = %q", field.Text())
	}
	for _, r := range []rune{0, '\t', '\n', '\r', 0x7f, -1, 0xD800, utf8.MaxRune + 1} {
		onChar(native, r)
	}
	if field.Text() != "qa_beta.go:4:6 中🙂" {
		t.Fatalf("invalid/control input inserted: %q", field.Text())
	}
	popup := NewEdit()
	popup.SetParent(root)
	popupNative, popupWin := testPopupWindow(popup, WtPopup, win)
	popupWin.enabled = true
	nested := NewEdit()
	nested.SetParent(popup)
	_, nestedWin := testPopupWindow(nested, WtPopup, popupWin)
	nestedWin.enabled = true
	nested.SetFocus()
	onChar(native, 'a')
	onChar(popupNative, 'b')
	if nested.Text() != "ab" || root.Text() != "" {
		t.Fatalf("nonactivating popup routing = %q/%q", nested.Text(), root.Text())
	}
	win.enabled = false
	onChar(native, 'c')
	if nested.Text() != "ab" {
		t.Fatal("disabled native owner accepted input")
	}
}
