package gui

// Native popup creation/capture may synthesize a motion event while the pointer
// has not moved. Preserve keyboard menu selection until actual screen movement;
// otherwise the existing hover-stop timer closes the newly opened submenu.
// Shared by Win32/GLFW and used only on the UI thread; no extra timer is needed.
var menuPointer = struct {
	x, y            float64
	known, keyboard bool
}{}

func holdMenuPointerForKeyboard() {
	if _, popup := curCapture().(*Menu); !popup {
		return
	}
	menuPointer.keyboard = true
	mouseMoving = false // Cancel a hover-stop left pending before the key event.
}

func observeMenuPointerMotion(gx, gy float64) bool {
	stationary := menuPointer.known && menuPointer.x == gx && menuPointer.y == gy
	menuPointer.x, menuPointer.y, menuPointer.known = gx, gy, true
	_, popup := curCapture().(*Menu)
	if !stationary || !popup {
		menuPointer.keyboard = false
	}
	if menuPointer.keyboard {
		mouseMoving = false
		return true
	}
	return false
}
