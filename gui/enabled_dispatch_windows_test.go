package gui

import (
	"testing"

	"github.com/uk0/silk/win32"
)

func TestEnabledWindowsMouseDispatchGatesEveryButton(t *testing.T) {
	for _, kind := range []string{"left", "right", "middle"} {
		t.Run(kind, func(t *testing.T) {
			isolateWindowsPopupState(t)
			root := NewVBox()
			root.SetSize(300, 200)
			probe := new(enabledLegacyProbe)
			probe.Init(probe)
			probe.SetParent(root)
			probe.SetBounds(10, 10, 100, 50)
			win := attachWindowsLifecycleFixture(root, WtForm, nil)
			pos := uintptr(30 | 25<<16)
			down, up := win.on_WM_LBUTTONDOWN, win.on_WM_LBUTTONUP
			dm, um := uint32(win32.WM_LBUTTONDOWN), uint32(win32.WM_LBUTTONUP)
			if kind == "right" {
				down, up = win.on_WM_RBUTTONDOWN, win.on_WM_RBUTTONUP
				dm, um = win32.WM_RBUTTONDOWN, win32.WM_RBUTTONUP
			} else if kind == "middle" {
				down, up = win.on_WM_MBUTTONDOWN, win.on_WM_MBUTTONUP
				dm, um = win32.WM_MBUTTONDOWN, win32.WM_MBUTTONUP
			}
			root.SetEnabled(false)
			down(dm, 0, pos)
			up(um, 0, pos)
			if probe.downs != 0 || probe.ups != 0 || win.toCapture {
				t.Fatal("disabled descendant accepted a Win32 mouse event or armed capture")
			}
			root.SetEnabled(true)
			down(dm, 0, pos)
			root.SetEnabled(false)
			up(um, 0, pos)
			if probe.downs != 1 || probe.ups != 0 {
				t.Fatal("disabling between press and release delivered a stale release")
			}
		})
	}
}
