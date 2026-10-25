//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestGLFWDialogActivationRoutesFocusedChoice(t *testing.T) {
	for _, tc := range []struct {
		name string
		tabs int
		key  glfw.Key
		want DialogResult
	}{
		{"save", 0, glfw.KeyEnter, DialogOK},
		{"discard", 1, glfw.KeyEnter, DialogNo},
		{"cancel", 2, glfw.KeyEnter, DialogCancel},
		{"space", 1, glfw.KeySpace, DialogNo},
		{"escape", 1, glfw.KeyEscape, DialogCancel},
		{"wrap", 3, glfw.KeyEnter, DialogOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatePopupState(t)
			d := NewDialog("Close", nil)
			d.SetContent(NewLabel("Save changes?"))
			save := d.AddButton("Save", DialogOK)
			d.AddButton("Discard", DialogNo)
			d.AddButton("Cancel", DialogCancel)
			native, win := testPopupWindow(d, WtForm, nil)
			win.enabled, win.inModal = true, true
			d.activateInitialFocus()
			if focusWidget != save {
				t.Fatal("no initial action focus")
			}
			for range tc.tabs {
				onKey(native, glfw.KeyTab, 0, glfw.Press, 0)
			}
			onKey(native, tc.key, 0, glfw.Repeat, 0)
			if d.closed {
				t.Fatal("held key submitted dialog")
			}
			onKey(native, tc.key, 0, glfw.Press, 0)
			if !d.closed || d.Result() != tc.want || win.inModal {
				t.Fatalf("closed=%v result=%v modal=%v", d.closed, d.Result(), win.inModal)
			}
		})
	}
}
