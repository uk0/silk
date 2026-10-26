//go:build darwin && cgo

package gui

/*
#cgo LDFLAGS: -framework Cocoa
void *silkInstallTabNavigation(void *window);
void silkRemoveTabNavigation(void *token);
*/
import "C"

import "github.com/go-gl/glfw/v3.3/glfw"

// Cocoa consumes Control-Tab before GLFW's keyDown callback. Forward only
// that native chord into the same callback, retaining IME/shortcut ownership.
// Install/remove on the UI thread while the GLFW window is still valid.
func installNativeTabNavigation(window *glfw.Window) func() {
	if window == nil {
		return nil
	}
	token := C.silkInstallTabNavigation(window.GetCocoaWindow())
	if token == nil {
		return nil
	}
	return func() {
		if token != nil {
			C.silkRemoveTabNavigation(token)
			token = nil
		}
	}
}
