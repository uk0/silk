package gui

import (
	"math"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/win32"
)

func TestWindowsMenuReopenLayoutsWithoutNativeResize(t *testing.T) {
	isolateWindowsPopupState(t)
	m := scrollTestMenu(t)
	// Native dimensions are integer pixels. A fractional size request may
	// produce no WM_SIZE at all. Use the zero-HWND fixture to model that path
	// without synthesizing a resize notification that the real OS omits.
	m.SetSize(math.Floor(m.SizeHints().Width-0.25), 240)
	m.OnKeyDown(KeyEnd, false)
	m.OnHide()
	attachWindowsLifecycleFixture(m, WtPopup, nil)
	m.fitPopupWorkArea(geom.Rect{Width: 1000, Height: 240})
	if m.items[0].Y() != m.ChildViewport().Y || m.scroll.offset != 0 {
		t.Fatal("reopened menu retained scrolled rows without WM_SIZE")
	}
	m.OnKeyDown(KeyEnd, false)
	assertMenuTargetVisible(t, m)
	m.OnKeyDown(KeyHome, false)
	assertMenuTargetVisible(t, m)
}

func TestWindowsKeyboardMenuIgnoresStationaryMouseMessages(t *testing.T) {
	isolateWindowsPopupState(t)
	oldMoving, oldTime := mouseMoving, lastMouseTime
	oldPointer := menuPointer
	t.Cleanup(func() { mouseMoving, lastMouseTime, menuPointer = oldMoving, oldTime, oldPointer })
	m := scrollTestMenu(t)
	win := attachWindowsLifecycleFixture(m, WtPopup, nil)
	captureStack = []IWidget{m}
	point := uintptr(20 | 60<<16)
	win.on_WM_MOUSEMOVE(win32.WM_MOUSEMOVE, 0, point)
	m.OnKeyDown(KeyEnd, false)
	if mouseMoving {
		t.Fatal("keyboard navigation retained a pending hover-stop event")
	}
	target := m.highlightedButton()
	// Creating/capturing a submenu can deliver WM_MOUSEMOVE without physical
	// movement. It must not replace the keyboard target or schedule hover-close.
	win.on_WM_MOUSEMOVE(win32.WM_MOUSEMOVE, 0, point)
	if mouseMoving || mouseHoverWidget != target {
		t.Fatal("stationary native event stole keyboard menu navigation")
	}
	win.on_WM_MOUSEMOVE(win32.WM_MOUSEMOVE, 0, point+1)
	if !mouseMoving || mouseHoverWidget == target {
		t.Fatal("actual pointer movement did not regain hover handling")
	}
}

func TestWindowsNativeWheelBubblesOverMenuButton(t *testing.T) {
	isolateWindowsPopupState(t)
	m := scrollTestMenu(t)
	win := &Window{widget: m}
	// Real WM_MOUSEWHEEL carries signed screen pixels in lParam and 120-unit
	// wheel deltas in wParam. No HWND is needed to exercise the handler route.
	x, y := m.MapToGlobal(20, 60)
	lparam := uintptr(uint32(uint16(int(x))) | uint32(uint16(int(y)))<<16)
	var delta int16 = -120
	wparam := uintptr(uint32(uint16(delta)) << 16)
	lastMouseWidget = m.items[0]
	win.on_WM_MOUSEWHEEL(win32.WM_MOUSEWHEEL, wparam, lparam)
	if m.scroll.offset <= 0 {
		t.Fatal("Win32 wheel was dropped by the menu button")
	}
}
