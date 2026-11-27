//go:build linux && cgo && !wayland

package gui

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
*/
import "C"

import (
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// prepareNativePopupOwner supplies X11's transient relationship before the
// window is mapped. This scopes WM stacking to the owner, not every app.
func prepareNativePopupOwner(popup, owner *glfw.Window) {
	if popup == nil || owner == nil || popup == owner {
		return
	}
	dpy := (*C.Display)(unsafe.Pointer(glfw.GetX11Display()))
	if dpy == nil {
		return
	}
	child, parent := C.Window(popup.GetX11Window()), C.Window(owner.GetX11Window())
	if child != 0 && parent != 0 && child != parent {
		C.XSetTransientForHint(dpy, child, parent)
	}
}

// GLFW maps a reused X11 window without raising it. In sessions without a
// window manager, it can stay covered by the owner after focus was returned.
// Also request stacking after each Show, without setting native input focus.
func bindNativePopupOwner(popup, owner *glfw.Window) bool {
	if popup == nil || owner == nil || popup == owner {
		return false
	}
	dpy := (*C.Display)(unsafe.Pointer(glfw.GetX11Display()))
	if dpy == nil {
		return false
	}
	child, parent := C.Window(popup.GetX11Window()), C.Window(owner.GetX11Window())
	if child == 0 || parent == 0 || child == parent {
		return false
	}
	C.XRaiseWindow(dpy, child)
	C.XFlush(dpy)
	return true
}

func unbindNativePopupOwner(popup *glfw.Window) {
	if popup == nil {
		return
	}
	dpy := (*C.Display)(unsafe.Pointer(glfw.GetX11Display()))
	if dpy == nil {
		return
	}
	if xid := C.Window(popup.GetX11Window()); xid != 0 {
		C.XDeleteProperty(dpy, xid, C.Atom(C.XA_WM_TRANSIENT_FOR))
	}
}

func nativeApplicationActive(keyWindowFocused bool) bool { return keyWindowFocused }
