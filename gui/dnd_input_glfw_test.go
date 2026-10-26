//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
	"time"
)

func TestDragOwnsEscapeAndTextInput(t *testing.T) {
	isolatePopupState(t)
	root := NewForm()
	ed := NewCodeEditor()
	ed.SetParent(root)
	ed.SetText("unchanged")
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	ed.SetFocus()
	active, cancel := dndActive, dndCancelRequested
	defer func() { dndActive, dndCancelRequested = active, cancel }()
	dndActive, dndCancelRequested = true, false
	onKey(native, glfw.KeyEscape, 0, glfw.Press, 0)
	onKey(native, glfw.KeyEscape, 0, glfw.Release, 0)
	onChar(native, 'X')
	onKey(native, glfw.KeyBackspace, 0, glfw.Press, 0)
	if !dndCancelRequested || ed.Text() != "unchanged" {
		t.Fatal("drag lost cancellation or leaked keyboard to editor")
	}
	dndActive = false
	onChar(native, 'Y')
	if ed.Text() == "unchanged" {
		t.Fatal("input stayed disabled after drag")
	}
}

func TestDragRejectsReentrancyWithoutOverwritingPayload(t *testing.T) {
	active, data := dndActive, privateDndData
	defer func() { dndActive, privateDndData = active, data }()
	dndActive = true
	privateDndData = []interface{}{"outer"}
	if (&Window{}).DoDragDrop(nil, nil, DndCopy, "nested") != DndIgnore || privateDndData[0] != "outer" {
		t.Fatal("nested drag corrupted outer session")
	}
	if dragIdleWait < time.Second/120 || dragIdleWait > time.Second/25 {
		t.Fatal("drag idle wait outside responsive frame budget")
	}
}

func TestDragCompletionClearsCaptureAndRoutesNextButton(t *testing.T) {
	isolatePopupState(t)
	moving := mouseMoving
	defer func() { mouseMoving = moving }()
	root := NewForm()
	root.SetSize(300, 200)
	source := NewLabel("drag source")
	source.SetParent(root)
	source.SetBounds(0, 0, 100, 50)
	next := NewButton1("next", nil)
	next.SetParent(root)
	next.SetBounds(0, 100, 120, 40)
	calls := 0
	next.Action().BindFunc0(func() { calls++ })
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	for _, captured := range []bool{false, true} {
		captureStack = nil
		if captured {
			captureStack = []IWidget{source}
		}
		lastMouseWidget, mouseHoverWidget = source, source
		mouseMoving = true
		win.autoCaptured = captured
		win.toCapture = !captured
		forgetMouseAfterModalLoop(win)
		if curCapture() != nil || lastMouseWidget != nil || mouseHoverWidget != nil || mouseMoving || win.autoCaptured || win.toCapture {
			t.Fatal("drag retained stale input state")
		}
		win.pointer = pointerEventState{x: 20, y: 120, valid: true}
		onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
		onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
	}
	if calls != 2 {
		t.Fatal("next button was not invoked exactly once per click", calls)
	}
	forgetMouseAfterModalLoop(nil)
}
