//go:build windows

package gui

import "testing"

// TestForgetMouseAfterModalLoopClearsStaleTracking pins the reset that has to
// follow OLE's modal drag loop. Without it the Win32 backend keeps pointing at
// the widget the drag started from — on_WM_LBUTTONDOWN re-resolves
// lastMouseWidget only when it is nil — so the first press after dropping a
// widget on the canvas was delivered to the palette instead of to the toolbar
// button under the cursor, and the Run button appeared dead.
func TestForgetMouseAfterModalLoopClearsStaleTracking(t *testing.T) {
	prevLast, prevHover, prevMoving := lastMouseWidget, mouseHoverWidget, mouseMoving
	t.Cleanup(func() {
		lastMouseWidget, mouseHoverWidget, mouseMoving = prevLast, prevHover, prevMoving
	})

	stale := NewLabel("drag source")
	prevStack := captureStack
	t.Cleanup(func() { captureStack = prevStack })
	captureStack = []IWidget{stale}
	lastMouseWidget = stale
	mouseHoverWidget = stale
	mouseMoving = true
	win := &Window{autoCaptured: true, toCapture: true}

	forgetMouseAfterModalLoop(win)

	if len(captureStack) != 0 {
		t.Error("the capture stack survived the drag; curCapture() stays non-nil, on_WM_MOUSEMOVE takes its redirect branch and lastMouseWidget is never re-resolved again")
	}
	if lastMouseWidget != nil {
		t.Error("lastMouseWidget still names the drag source; the next press goes there instead of to what was clicked")
	}
	if mouseHoverWidget != nil {
		t.Error("mouseHoverWidget survived the drag; hover and cursor stay on the source")
	}
	if mouseMoving {
		t.Error("mouseMoving is still set; the idle mouse timer acts on a move that ended inside the modal loop")
	}
	if win.autoCaptured || win.toCapture {
		t.Error("capture bookkeeping still says a button is down; OLE swallowed the button-up that would have cleared it")
	}
}

// TestForgetMouseAfterModalLoopToleratesNoWindow keeps the reset usable from a
// drag that started without a resolved window rather than panicking inside a
// path that runs right after a modal loop, where a panic is hardest to see.
func TestForgetMouseAfterModalLoopToleratesNoWindow(t *testing.T) {
	prevLast, prevHover, prevMoving := lastMouseWidget, mouseHoverWidget, mouseMoving
	t.Cleanup(func() {
		lastMouseWidget, mouseHoverWidget, mouseMoving = prevLast, prevHover, prevMoving
	})

	lastMouseWidget = NewLabel("x")
	forgetMouseAfterModalLoop(nil)
	if lastMouseWidget != nil {
		t.Error("the nil-window path skipped the reset entirely")
	}
}
