package gui

import (
	"testing"

	"github.com/uk0/silk/win32"
)

func TestWinCaptureMutationMatchesOnlyExpectedOwnerAndTarget(t *testing.T) {
	for _, item := range []struct {
		name    string
		request winCaptureMutation
		from    win32.HWND
		to      win32.HWND
		want    bool
	}{
		{"inactive", winCaptureMutation{from: 11, to: 22}, 11, 22, false},
		{"transfer", winCaptureMutation{from: 11, to: 22, active: true}, 11, 22, true},
		{"same surface", winCaptureMutation{from: 11, to: 11, active: true}, 11, 11, true},
		{"release", winCaptureMutation{from: 11, active: true}, 11, 0, true},
		{"foreign owner", winCaptureMutation{from: 11, active: true}, 22, 0, false},
		{"unexpected target", winCaptureMutation{from: 11, to: 22, active: true}, 11, 33, false},
		{"no native owner", winCaptureMutation{active: true}, 0, 0, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			if item.request.matches(item.from, item.to) != item.want {
				t.Fatal("native ownership acknowledgement confused with external loss")
			}
		})
	}
}

func TestWinInternalCaptureAcknowledgementKeepsGestureButExternalLossCancels(t *testing.T) {
	isolateEnabledTest(t)
	old := expectedWinCaptureMutation
	t.Cleanup(func() { expectedWinCaptureMutation = old })
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	win.hWnd = 11 // logical surface only; no HWND is created by this regression
	button := NewButton1("action", nil)
	button.SetParent(root)
	button.pushed = true
	captureStack = []IWidget{button}
	lastMouseWidget = button
	expectedWinCaptureMutation = winCaptureMutation{from: win.hWnd, to: win.hWnd, active: true}
	win.nativeCaptureChanged(win.hWnd)
	if curCapture() != button || lastMouseWidget != button || !button.pushed {
		t.Fatal("internal same-HWND acknowledgement canceled the active gesture")
	}
	win.nativeCaptureChanged(22)
	if curCapture() != nil || lastMouseWidget != nil || button.pushed {
		t.Fatal("the internal transfer fence swallowed a genuine external loss")
	}
}
