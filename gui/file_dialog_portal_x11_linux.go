//go:build linux && !wayland

package gui

import (
	"fmt"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func linuxFilePortalParent(window *glfw.Window, _ func() bool) (string, func()) {
	xid := window.GetX11Window()
	if xid == 0 {
		return "", nil
	}
	return fmt.Sprintf("x11:%x", uint64(xid)), nil
}

func closeLinuxFilePortalDisplay() {}
