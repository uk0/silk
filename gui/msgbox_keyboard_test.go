package gui

import "testing"

func TestMessageBoxReadableKeyboardContract(t *testing.T) {
	box := NewMessageBox()
	box.SetMessage("[SILK-FS004] Destination already exists: C:/a/very/long/path/with/no/spaces/file.go\nChoose another name; existing files are not overwritten.")
	box.SetButtons([]string{"确定"})
	if !box.edit.Warp() {
		t.Error("long error text is clipped instead of wrapped")
	}
	if _, ok := any(box).(interface{ resolveCancel() }); !ok {
		t.Error("Escape cannot dismiss MessageBox")
	}
	if _, ok := any(box).(IEventKeyDown); !ok {
		t.Error("Enter cannot dismiss MessageBox")
	}
}

func TestMessageBoxSafeDefaultsAndReadOnlyDetails(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	box := NewMessageBox()
	box.SetButtons([]string{"删除", "取消"})
	box.focusMessage()
	if !box.edit.HasFocus() {
		t.Fatal("read-only details are not selectable on activation")
	}
	box.OnKeyDown(KeyEnter, false)
	if box.ret != "取消" {
		t.Fatal("Enter implicitly accepted a destructive action")
	}
	box.SetDefaultButton("删除")
	box.OnKeyDown(KeyEnter, true)
	if box.ret != "取消" {
		t.Fatal("key repeat submitted the dialog")
	}
	box.OnKeyDown(KeyEnter, false)
	if box.ret != "删除" {
		t.Fatal("explicit default button not honored")
	}
	box.resolveCancel()
	if box.ret != "取消" {
		t.Fatal("Escape did not return explicit cancel choice")
	}
	box.btnBox.btnMap["删除"].SetFocus()
	box.resolveDefault()
	if box.ret != "删除" {
		t.Fatal("Tab-focused button lost its action")
	}
	box.SetButtons([]string{"custom A", "custom B"})
	box.focusMessage()
	if box.resolveDefault() {
		t.Fatal("unknown multi-button dialog acquired implicit destructive default")
	}
	box.SetButtons([]string{"确定"})
	box.btnBox.btnMap["确定"].SetEnabled(false)
	if box.resolveDefault() {
		t.Fatal("disabled button was activated")
	}
	box.resolveCancel()
	if box.ret != "" {
		t.Fatal("no explicit Cancel must reject, not accept sole button")
	}
	box.SetMessage("[SILK-FS004] protected")
	box.edit.OnTextInput("replacement")
	if box.edit.Text() != "[SILK-FS004] protected" {
		t.Fatal("read-only details mutated")
	}
}

func TestMessageBoxWrapsLongPathWithoutLosingText(t *testing.T) {
	box := NewMessageBox()
	text := "C:/workspace/very-long-component/another-component/deeply-nested-project/renamed-file.go: [SILK-FS004] Destination already exists\nChoose another name; existing files are not overwritten."
	box.SetMessage(text)
	box.SetButtons([]string{"OK"})
	box.SetSize(620, 280)
	if box.edit.SoftRowsCount() < 3 || box.edit.Text() != text {
		t.Fatal("error path/code/hint were clipped or replaced")
	}
	box.SetSize(1, 1)
	if box.edit.Width() < 0 || box.edit.Height() < 0 || box.btnBox.Height() < 0 {
		t.Fatal("small dialog produced negative child geometry")
	}
}
