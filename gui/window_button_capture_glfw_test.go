//go:build !windows

package gui

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"testing"
)

type capturedButtonProbe struct {
	Button
	downs, ups, clicks int
}

func newCapturedButtonProbe(parent IWidget, label string) *capturedButtonProbe {
	b := new(capturedButtonProbe)
	b.Init(b)
	b.SetText(label)
	b.SetParent(parent)
	b.Action().BindFunc0(func() { b.clicks++ })
	return b
}
func (b *capturedButtonProbe) OnLeftDown(x, y float64) { b.downs++; b.Button.OnLeftDown(x, y) }
func (b *capturedButtonProbe) OnLeftUp(x, y float64)   { b.ups++; b.Button.OnLeftUp(x, y) }

func capturedMenuFixture(t *testing.T) (*glfw.Window, *glfw.Window, *Menu, *capturedButtonProbe, *capturedButtonProbe, func(float64, float64)) {
	t.Helper()
	isolatePopupState(t)
	root := NewLabel("owner")
	root.SetSize(600, 400)
	owner, ow := testPopupWindow(root, WtForm, nil)
	ow.enabled = true
	menu := NewMenu(false)
	menu.SetVertical(true)
	menu.SetParent(root)
	menu.SetSize(200, 100)
	native, mw := testPopupWindow(menu, WtPopup, ow)
	mw.enabled = true
	a, b := newCapturedButtonProbe(menu, "first"), newCapturedButtonProbe(menu, "second")
	menu.items = []IWidget{a, b}
	menu.Layout()
	captureStack = []IWidget{menu}
	lastMouseWidget = menu
	mouseHoverWidget = nil
	focusWidget = menu
	old := mouseButtonEventPosition
	x, y := 0.0, 0.0
	mouseButtonEventPosition = func(*glfw.Window, glfw.MouseButton, glfw.Action) (float64, float64, bool) { return x, y, true }
	t.Cleanup(func() { mouseButtonEventPosition = old })
	return owner, native, menu, a, b, func(px, py float64) { x, y = px, py }
}

func TestCapturedMenuClickWithoutPointerMove(t *testing.T) {
	for _, cross := range []bool{false, true} {
		t.Run(map[bool]string{false: "popup-event", true: "owner-event"}[cross], func(t *testing.T) {
			owner, native, menu, a, _, position := capturedMenuFixture(t)
			if cross {
				native = owner
			}
			position(a.X()+5, a.Y()+5)
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
			if a.downs != 1 || a.ups != 1 || a.clicks != 1 || a.pushed || menu.IsVisible() {
				t.Fatalf("stationary menu click lost: down/up/click=%d/%d/%d pushed=%v visible=%v", a.downs, a.ups, a.clicks, a.pushed, menu.IsVisible())
			}
		})
	}
}

func TestCapturedMenuReleaseKeepsPressOwnerAndFreshHover(t *testing.T) {
	for _, cross := range []bool{false, true} {
		t.Run(map[bool]string{false: "popup-event", true: "owner-event"}[cross], func(t *testing.T) {
			owner, native, menu, a, b, position := capturedMenuFixture(t)
			if cross {
				native = owner
			}
			position(a.X()+5, a.Y()+5)
			mouseHoverWidget = a
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
			// A new native release coordinate must work without an intervening MOVE.
			position(b.X()+5, b.Y()+5)
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
			if a.downs != 1 || a.ups != 1 || a.clicks != 0 || a.pushed || b.downs != 0 || b.ups != 0 || b.clicks != 0 || !menu.IsVisible() {
				t.Fatalf("release retargeted/stuck: first=%+v second=%+v visible=%v", []int{a.downs, a.ups, a.clicks}, []int{b.downs, b.ups, b.clicks}, menu.IsVisible())
			}
			if curCapture() != menu {
				t.Fatal("release did not restore menu capture")
			}
		})
	}
}

func TestCapturedLeafDoesNotRetargetOtherWindowEvents(t *testing.T) {
	owner, native, menu, a, b, position := capturedMenuFixture(t)
	_ = native
	captureStack = []IWidget{menu, a}
	lastMouseWidget = a
	mouseHoverWidget = a
	position(b.X()+5, b.Y()+5)
	onMouseButton(owner, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(owner, glfw.MouseButtonLeft, glfw.Release, 0)
	if a.downs != 1 || a.ups != 1 || a.clicks != 0 || b.downs != 0 || b.ups != 0 || b.clicks != 0 {
		t.Fatalf("explicit capture retargeted: first=%v second=%v", []int{a.downs, a.ups, a.clicks}, []int{b.downs, b.ups, b.clicks})
	}
}

func TestCapturedMenuReleaseOutsideWithoutMove(t *testing.T) {
	for _, cross := range []bool{false, true} {
		t.Run(map[bool]string{false: "popup-event", true: "owner-event"}[cross], func(t *testing.T) {
			owner, native, menu, a, _, position := capturedMenuFixture(t)
			if cross {
				native = owner
			}
			position(a.X()+5, a.Y()+5)
			mouseHoverWidget = a
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
			position(menu.Width()+20, menu.Height()+20)
			onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
			if a.downs != 1 || a.ups != 1 || a.clicks != 0 || a.pushed || mouseHoverWidget != nil || curCapture() != menu {
				t.Fatal("outside release did not cancel only the pressed row")
			}
		})
	}
}

func TestCapturedDisabledWindowRejectsTranslatedButtons(t *testing.T) {
	owner, native, _, a, _, position := capturedMenuFixture(t)
	winMap[native].enabled = false
	position(a.X()+5, a.Y()+5)
	onMouseButton(owner, glfw.MouseButtonLeft, glfw.Press, 0)
	onMouseButton(owner, glfw.MouseButtonLeft, glfw.Release, 0)
	if a.downs != 0 || a.ups != 0 || a.clicks != 0 {
		t.Fatal("translated event bypassed disabled window")
	}
}

// Ordinary button gestures have identical semantics before/after the capture
// fix, making this a comparable event-dispatch microbenchmark (not frame time).
func BenchmarkNativeButtonDispatch(b *testing.B) {
	oldMap, oldCapture, oldFocus := winMap, captureStack, focusWidget
	oldHover, oldMouse, oldActive, oldPointer := mouseHoverWidget, lastMouseWidget, activeForm, pointerWindow
	oldEvent := mouseButtonEventPosition
	b.Cleanup(func() {
		winMap, captureStack, focusWidget = oldMap, oldCapture, oldFocus
		mouseHoverWidget, lastMouseWidget, activeForm, pointerWindow = oldHover, oldMouse, oldActive, oldPointer
		mouseButtonEventPosition = oldEvent
	})
	root := NewLabel("root")
	root.SetSize(400, 300)
	native, win := testPopupWindow(root, WtForm, nil)
	win.enabled = true
	winMap = map[*glfw.Window]*Window{native: win}
	captureStack = nil
	focusWidget = nil
	button := NewButton1("action", nil)
	button.SetParent(root)
	button.SetBounds(5, 5, 100, 30)
	mouseButtonEventPosition = func(*glfw.Window, glfw.MouseButton, glfw.Action) (float64, float64, bool) { return 10, 10, true }
	clicks := 0
	button.Action().BindFunc0(func() { clicks++ })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		onMouseButton(native, glfw.MouseButtonLeft, glfw.Press, 0)
		onMouseButton(native, glfw.MouseButtonLeft, glfw.Release, 0)
	}
	b.StopTimer()
	if clicks != b.N {
		b.Fatalf("lost gestures: %d/%d", clicks, b.N)
	}
}
