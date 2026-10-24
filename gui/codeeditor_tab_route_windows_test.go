//go:build windows

package gui

import (
	"testing"

	"github.com/uk0/silk/win32"
)

func TestWindowsNativeTabReachesCodeEditor(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.filePath = "notes.txt"
	e.SetText("iferr\n")
	e.ScrollToLineCol(0, 5)
	h := e.Window().hWnd
	win32.SendMessage(h, win32.WM_KEYDOWN, win32.VK_TAB, 1)
	win32.SendMessage(h, win32.WM_CHAR, '\t', 1)
	if e.Text() != "iferr\t\n" || focusWidget != e {
		t.Fatalf("native Tab bypassed editor or inserted twice: %q focus=%T", e.Text(), focusWidget)
	}
	win32.SendMessage(h, win32.WM_KEYDOWN, win32.VK_TAB, 1<<30)
	win32.SendMessage(h, win32.WM_CHAR, '\t', 1)
	if e.Text() != "iferr\t\t\n" {
		t.Fatalf("native Tab repeat was not delivered once: %q", e.Text())
	}
}
