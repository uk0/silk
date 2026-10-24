//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestWindowTabReachesCodeEditor(t *testing.T) {
	isolatePopupState(t)
	root := NewForm()
	e := NewCodeEditor()
	e.SetParent(root)
	e.filePath = "notes.txt"
	e.SetText("iferr\n")
	e.SetSize(800, 500)
	e.ScrollToLineCol(0, 5)
	next := NewButton()
	next.SetParent(root)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	e.SetFocus()
	onKey(native, glfw.KeyTab, 0, glfw.Press, 0)
	if focusWidget != e || e.Text() != "iferr\t\n" {
		t.Fatalf("window routed Tab away: focus=%T text=%q", focusWidget, e.Text())
	}
	onKey(native, glfw.KeyTab, 0, glfw.Repeat, 0)
	if focusWidget != e || e.Text() != "iferr\t\t\n" {
		t.Fatalf("Tab repeat not delivered once: %q", e.Text())
	}
	// Modified Tab provides an escape to normal host focus traversal.
	onKey(native, glfw.KeyTab, 0, glfw.Press, glfw.ModControl)
	if focusWidget != next || e.Text() != "iferr\t\t\n" {
		t.Fatal("Ctrl+Tab did not preserve host focus navigation")
	}
}
