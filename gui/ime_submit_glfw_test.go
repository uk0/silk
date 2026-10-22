//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
)

// This verifies routing/order, not a real Cocoa IME candidate session.
func TestComposingEnterCannotSubmitInputBox(t *testing.T) {
	isolatePopupState(t)
	oldQuery, oldReady := glfwCompositionActive, glfwReady
	t.Cleanup(func() { glfwCompositionActive, glfwReady = oldQuery, oldReady })
	composing := true
	glfwReady = true
	glfwCompositionActive = func(*glfw.Window) bool { return composing }
	box := NewInputBox()
	box.SetSize(380, 170)
	box.SetMessage("old.go")
	native, win := testPopupWindow(box, WtForm, nil)
	win.enabled, win.inModal = true, true
	box.focusInput()
	onKey(native, glfw.KeyEnter, 0, glfw.Press, 0)
	if !win.inModal || win.modalRet != nil {
		t.Fatal("composition confirmation submitted dialog")
	}
	onKey(native, glfw.KeyEscape, 0, glfw.Press, 0)
	if !win.inModal {
		t.Fatal("composition cancellation closed dialog")
	}
	// Cocoa insertText is delivered after the key callback, before next Enter.
	box.edit.OnTextInput("新しい.go")
	composing = false
	onKey(native, glfw.KeyEnter, 0, glfw.Press, 0)
	if win.modalRet != true || box.edit.Text() != "新しい.go" {
		t.Fatalf("final submit lost committed text: %v %q", win.modalRet, box.edit.Text())
	}
}
