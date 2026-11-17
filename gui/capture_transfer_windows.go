package gui

import "github.com/uk0/silk/win32"

// SetCapture/ReleaseCapture can synchronously notify the old native surface,
// including a transfer between widgets sharing the same HWND. Such a message
// is an acknowledgement of our own mutation, not an external gesture cancel.
// Otherwise Button.OnPointerCancel can reenter PopCapture and leave a hidden
// popup holding native capture after the logical stack has already been cleared.
type winCaptureMutation struct {
	from, to win32.HWND
	active   bool
}

var expectedWinCaptureMutation winCaptureMutation

func (m winCaptureMutation) matches(from, to win32.HWND) bool {
	return m.active && from != 0 && m.from == from && m.to == to
}

func setWinCapture(target win32.HWND) {
	previous := expectedWinCaptureMutation
	expectedWinCaptureMutation = winCaptureMutation{from: win32.GetCapture(), to: target, active: true}
	defer func() { expectedWinCaptureMutation = previous }()
	win32.SetCapture(target)
}

func releaseWinCapture() {
	previous := expectedWinCaptureMutation
	expectedWinCaptureMutation = winCaptureMutation{from: win32.GetCapture(), active: true}
	defer func() { expectedWinCaptureMutation = previous }()
	win32.ReleaseCapture()
}

func (win *Window) nativeCaptureChanged(next win32.HWND) {
	if win != nil && !expectedWinCaptureMutation.matches(win.hWnd, next) {
		win.cancelPointerGestures()
	}
}
