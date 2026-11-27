//go:build linux && wayland

package gui

/*
#cgo pkg-config: wayland-client
#include "file_dialog_portal_wayland.h"
*/
import "C"

import (
	"time"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// UI-thread registry for the current GLFW display. A Wayland registry is a
// connection-lifetime resource, not created anew for every chooser.
var filePortalDisplay *C.struct_silk_foreign_registry

func linuxFilePortalParent(window *glfw.Window, alive func() bool) (string, func()) {
	if !alive() {
		return "", nil
	}
	display := (*C.struct_wl_display)(unsafe.Pointer(glfw.GetWaylandDisplay()))
	if display == nil {
		return "", nil
	}
	if filePortalDisplay == nil {
		filePortalDisplay = C.silk_foreign_discover(display)
		if filePortalDisplay == nil {
			return "", nil
		}
	}
	deadline := time.Now().Add(750 * time.Millisecond)
	for C.silk_foreign_state(filePortalDisplay) == 0 && alive() && time.Now().Before(deadline) {
		nativeDialogFrame()
	}
	if !alive() || C.silk_foreign_state(filePortalDisplay) != 1 || !time.Now().Before(deadline) {
		return "", nil
	}
	// Do not retain a wl_surface across the event pump above: processing
	// events may have detached the exact window that was opening the picker.
	surface := (*C.struct_wl_surface)(unsafe.Pointer(window.GetWaylandWindow()))
	if surface == nil {
		return "", nil
	}
	export := C.silk_foreign_export(filePortalDisplay, surface)
	if export == nil {
		return "", nil
	}
	release := func() { C.silk_foreign_revoke(export) }
	for alive() && time.Now().Before(deadline) {
		handle := C.silk_foreign_handle(export)
		if handle != nil {
			if name := C.GoString(handle); name != "" {
				return "wayland:" + name, release
			}
			break
		}
		nativeDialogFrame()
	}
	release()
	return "", nil
}

func closeLinuxFilePortalDisplay() {
	if filePortalDisplay != nil {
		C.silk_foreign_forget(filePortalDisplay)
		filePortalDisplay = nil
	}
}
