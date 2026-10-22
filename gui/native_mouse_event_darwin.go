//go:build darwin && cgo

package gui

/*
#cgo LDFLAGS: -framework Cocoa
int silkMouseButtonEventPosition(void *window, int button, int action, double *x, double *y);
int silkHasMarkedText(void *window);
int silkInputIsMainThread(void);
*/
import "C"

import "github.com/go-gl/glfw/v3.3/glfw"

func nativeHasMarkedText(window *glfw.Window) bool {
	return window != nil && C.silkInputIsMainThread() != 0 && C.silkHasMarkedText(window.GetCocoaWindow()) != 0
}

func nativeMouseButtonEventPosition(window *glfw.Window, button glfw.MouseButton, action glfw.Action) (float64, float64, bool) {
	if window == nil {
		return 0, 0, false
	}
	var x, y C.double
	ok := C.silkMouseButtonEventPosition(window.GetCocoaWindow(), C.int(button), C.int(action), &x, &y)
	return float64(x), float64(y), ok != 0
}
