package gui

import "testing"

func TestDialogInitialFocusIsDeferredAndScoped(t *testing.T) {
	old := focusWidget
	defer func() { focusWidget = old }()
	owner, editor := NewForm(), NewEdit()
	editor.SetParent(owner)
	editor.SetFocus()
	dlg := NewDialog("Find", owner)
	input := NewEdit()
	dlg.SetContent(input)
	dlg.SetInitialFocus(input)
	if focusWidget != editor {
		t.Fatal("preparing a dialog stole the editor focus")
	}
	dlg.activateInitialFocus()
	if focusWidget != input {
		t.Fatal("initial child did not receive focus on activation")
	}
	editor.SetFocus()
	dlg.SetInitialFocus(editor)
	dlg.activateInitialFocus()
	if focusWidget != editor {
		t.Fatal("dialog activated a foreign widget")
	}
	dlg.SetInitialFocus(input)
	input.SetEnabled(false)
	dlg.activateInitialFocus()
	if focusWidget != editor {
		t.Fatal("disabled initial target got focus")
	}
}
