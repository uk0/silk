package gui

import "testing"

func TestInputBoxInitialKeyboardFocusAndReplacement(t *testing.T) {
	saved := focusWidget
	t.Cleanup(func() { focusWidget = saved })
	box := NewInputBox()
	box.SetMessage("旧名称.go")
	other := NewEdit()
	other.SetFocus()
	box.focusInput() // the callback passed to Window.ShowModal
	if !box.edit.HasFocus() || box.edit.SelectionText() != "旧名称.go" {
		t.Fatal("input dialog did not focus/select its editable field")
	}
	box.edit.OnTextInput("新名称.go")
	if box.edit.Text() != "新名称.go" {
		t.Fatal("typing did not replace the default name")
	}
	if box.edit.cbSubmit == nil {
		t.Fatal("Enter is not wired to accept")
	}
	// Safe before attachment: neither key action may close a foreign window.
	box.edit.OnKeyDown(KeyEnter, false)
	box.resolveCancel()
}
