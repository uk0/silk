//go:build windows

package gui

import (
	"testing"

	"github.com/uk0/silk/win32"
)

func TestWindowsDialogActivationRoutesFocusedChoice(t *testing.T) {
	for _, tc := range []struct {
		name string
		tabs int
		key  int
		want DialogResult
	}{
		{"save", 0, KeyEnter, DialogOK},
		{"discard", 1, KeyEnter, DialogNo},
		{"cancel", 2, KeyEnter, DialogCancel},
		{"space", 1, KeySpace, DialogNo},
		{"escape", 1, KeyEsc, DialogCancel},
		{"wrap", 3, KeyEnter, DialogOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := nativeModalOwner(t)
			d := NewDialog("Dialog keyboard QA", owner)
			d.SetContent(NewLabel("Save changes?"))
			save := d.AddButton("Save", DialogOK)
			d.AddButton("Discard", DialogNo)
			d.AddButton("Cancel", DialogCancel)
			Post(func() {
				win := d.Window()
				defer func() {
					if !d.closed {
						win.EndModal(DialogCancel)
					}
				}()
				if focusWidget != save || owner.Window().IsEnabled() {
					t.Error("native modal activation did not isolate owner and focus default")
					return
				}
				for range tc.tabs {
					win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, win32.VK_TAB, 0)
				}
				win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, uintptr(tc.key), 1<<30)
				if d.closed {
					t.Error("held key submitted dialog")
					return
				}
				win32.SendMessage(win.hWnd, win32.WM_KEYDOWN, uintptr(tc.key), 0)
				if !d.closed || d.Result() != tc.want {
					t.Errorf("closed=%v result=%v want=%v", d.closed, d.Result(), tc.want)
				}
			})
			if got := d.ShowModal(); got != tc.want || d.LastError() != nil {
				t.Fatalf("modal result=%v error=%v", got, d.LastError())
			}
			if focusWidget != owner || !owner.Window().IsEnabled() {
				t.Fatal("native dialog did not return keyboard input to owner")
			}
		})
	}
}
