package gui

import "testing"

type wheelTestWidget struct {
	Widget
	events [][3]float64
}

func (w *wheelTestWidget) OnMouseWheel(x, y, z float64) {
	w.events = append(w.events, [3]float64{x, y, z})
}

func TestWheelBubblesFromChildAndIgnoresStaleHover(t *testing.T) {
	oldCapture, oldHover := captureStack, lastMouseWidget
	t.Cleanup(func() { captureStack, lastMouseWidget = oldCapture, oldHover })
	captureStack = nil
	root := new(wheelTestWidget)
	root.Init(root)
	root.SetBounds(120, 240, 400, 300)
	child := NewButton1("not a scroller", nil)
	child.SetParent(root)
	child.SetBounds(10, 20, 100, 30)
	stale := new(wheelTestWidget)
	stale.Init(stale)
	lastMouseWidget = stale
	gx, gy := child.MapToGlobal(5, 6)
	if !dispatchMouseWheel(root, gx, gy, -0.5) || len(root.events) != 1 || root.events[0] != [3]float64{15, 26, -0.5} || len(stale.events) != 0 {
		t.Fatal("wheel did not bubble in correct local coordinates", root.events)
	}
	captureStack = []IWidget{stale}
	if !dispatchMouseWheel(root, gx, gy, 1) || len(stale.events) != 1 || len(root.events) != 1 {
		t.Fatal("capture was ignored")
	}
	stale.Hide()
	if !dispatchMouseWheel(root, gx, gy, 1) || len(stale.events) != 1 || len(root.events) != 2 {
		t.Fatal("closed capture was not released to the live pointer target")
	}
}
