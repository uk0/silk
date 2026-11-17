package gui

import "testing"

func TestWindowPointerCancelRetiresUncapturedPress(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	button := NewButton1("action", nil)
	button.SetParent(root)
	lastMouseWidget = button
	win.autoCaptured, win.toCapture = true, true
	button.OnLeftDown(1, 1)
	if !button.pushed {
		t.Fatal("fixture has no active press")
	}
	win.cancelPointerGestures()
	if button.pushed || lastMouseWidget != nil || win.autoCaptured || win.toCapture {
		t.Fatal("native cancellation left an uncaptured press or old mouse routing alive")
	}
}

func TestWindowPointerCancelClearsEveryOwnedCaptureSlot(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	parent, child := NewButton1("parent", nil), NewButton1("child", nil)
	parent.SetParent(root)
	child.SetParent(parent)
	captureStack = []IWidget{parent, child}
	backing := captureStack[:cap(captureStack)]
	lastMouseWidget = child
	parent.pushed, child.pushed = true, true
	win.cancelPointerGestures()
	if curCapture() != nil || lastMouseWidget != nil || parent.pushed || child.pushed {
		t.Fatal("nested native cancellation restored an already-canceled ancestor")
	}
	for _, old := range backing {
		if old != nil {
			t.Fatal("canceled native ownership survives in a capture backing slot")
		}
	}
}

type legacyCaptureControl struct{ Widget }

func TestWindowPointerCancelRetiresLegacyCaptureWithoutCancelHook(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	control := new(legacyCaptureControl)
	control.Init(control)
	control.SetParent(root)
	captureStack = []IWidget{control}
	lastMouseWidget = control
	win.cancelPointerGestures()
	if curCapture() != nil || lastMouseWidget != nil {
		t.Fatal("a legacy SDK control retained native routing after capture loss")
	}
}

type reentrantPointerCancelControl struct {
	Widget
	window *Window
	calls  int
}

func (c *reentrantPointerCancelControl) OnPointerCancel() {
	c.calls++
	// Bound the negative replay instead of crashing the test process's stack.
	if c.calls < 3 {
		c.window.cancelPointerGestures()
	}
	c.PopCapture()
}

func TestWindowPointerCancelReentryRetiresOnce(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	control := new(reentrantPointerCancelControl)
	control.Init(control)
	control.window = win
	control.SetParent(root)
	captureStack = []IWidget{control}
	lastMouseWidget = control
	win.cancelPointerGestures()
	if control.calls != 1 || curCapture() != nil || lastMouseWidget != nil {
		t.Fatalf("cancellation reentered or retained routing: %d calls", control.calls)
	}
}
