//go:build !windows

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

func pointerStillInTooltipWindow(anchor hoverToolTipAnchor) bool {
	win := anchor.win
	host := win
	if win.clientPopupHost != nil {
		host = win.clientPopupHost
	}
	if pointerWindow != host || !host.pointer.valid {
		return false
	}
	x, y := host.pointer.x, host.pointer.y
	if win.clientPopupHost != nil {
		if x-win.clientPopupX != anchor.x || y-win.clientPopupY != anchor.y {
			return false
		}
	} else if x != anchor.x || y != anchor.y {
		return false
	}
	popup := host.clientPopupAt(x, y)
	return (host == win && popup == nil) || popup == win
}

func mouseMiddleDown() bool {
	for gw := range winMap {
		if gw.GetMouseButton(glfw.MouseButtonMiddle) == glfw.Press {
			return true
		}
	}
	return false
}
