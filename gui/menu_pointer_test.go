package gui

import "testing"

func TestKeyboardMenuPointerGuardRequiresActualMovement(t *testing.T) {
	oldPointer, oldMoving := menuPointer, mouseMoving
	t.Cleanup(func() { menuPointer, mouseMoving = oldPointer, oldMoving })
	m := scrollTestMenu(t)
	captureStack = []IWidget{m}
	menuPointer.keyboard = false
	observeMenuPointerMotion(-120, 80)
	mouseMoving = true
	m.OnKeyDown(KeyEnd, false)
	if mouseMoving || !observeMenuPointerMotion(-120, 80) {
		t.Fatal("stationary native notification retained a pending hover event")
	}
	if observeMenuPointerMotion(-119, 80) {
		t.Fatal("real pointer motion was suppressed")
	}
	mouseMoving = true
	if observeMenuPointerMotion(-119, 80) || !mouseMoving {
		t.Fatal("pointer mode lost its legitimate hover-stop after a duplicate event")
	}
	m.OnKeyDown(KeyHome, false)
	dispatchMouseWheel(m, -119, 80, -1)
	if menuPointer.keyboard {
		t.Fatal("wheel did not resume pointer interaction")
	}
	m.OnKeyDown(KeyEnd, false)
	captureStack = nil
	if observeMenuPointerMotion(-119, 80) || menuPointer.keyboard {
		t.Fatal("closed popup suppressed an unrelated window's mouse event")
	}
}
