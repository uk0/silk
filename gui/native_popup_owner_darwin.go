//go:build darwin && cgo

package gui

/*
#cgo LDFLAGS: -framework Cocoa
int silkBindPopupOwner(void *popup, void *owner);
void silkUnbindPopupOwner(void *popup);
int silkApplicationActive(void);
*/
import "C"

import "github.com/go-gl/glfw/v3.3/glfw"

// bindNativePopupOwner attaches the real NSWindow, not just the Go popup
// bookkeeping or an OpenGL share context. Call on the UI thread next to Show,
// after final geometry has been applied: addChildWindow is an ordering action.
// Cocoa's orderOut removes the parent relationship, so every re-show must bind
// again. Both GLFW windows must still be valid; nil is harmless.
func bindNativePopupOwner(popup, owner *glfw.Window) bool {
	if popup == nil || owner == nil || popup == owner {
		return false
	}
	return C.silkBindPopupOwner(popup.GetCocoaWindow(), owner.GetCocoaWindow()) != 0
}

// unbindNativePopupOwner must run before the corresponding GLFW Destroy,
// never after its queued native destruction has already released the handle.
func unbindNativePopupOwner(popup *glfw.Window) {
	if popup != nil {
		C.silkUnbindPopupOwner(popup.GetCocoaWindow())
	}
}

// AppKit can have no key window between closing a popup/dialog and choosing
// another Form, while this application remains active. Do not confuse that
// short transition with the user switching to a different application.
func nativeApplicationActive(keyWindowFocused bool) bool {
	active := int(C.silkApplicationActive())
	if active < 0 {
		return keyWindowFocused // Off-thread/unknown: retain conservative fallback.
	}
	return active != 0
}

func prepareNativePopupOwner(popup, owner *glfw.Window) {}
