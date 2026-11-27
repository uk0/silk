package gui

import "github.com/uk0/silk/win32"

func pointerStillInTooltipWindow(anchor hoverToolTipAnchor) bool {
	if !anchor.win.mouseEntered {
		return false
	}
	x, y := MousePosition()
	if win32.WindowFromPoint(int32(x), int32(y)) != anchor.win.hWnd {
		return false
	}
	cx, cy := anchor.win.mapFromGlobal(x, y)
	return cx == anchor.x && cy == anchor.y
}

func mouseMiddleDown() bool { return IsKeyDown(win32.VK_MBUTTON) }
