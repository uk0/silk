//go:build darwin && cgo

package gui

/*
#cgo LDFLAGS: -framework Cocoa
int silkShowCharacterPalette(void *window);
int silkInputIsMainThread(void);
*/
import "C"

const nativeCharacterPaletteAvailable = true

func nativeShowCharacterPalette(win *Window) bool {
	if !glfwReady || win == nil || win.glfwWin == nil || C.silkInputIsMainThread() == 0 {
		return false
	}
	return C.silkShowCharacterPalette(win.glfwWin.GetCocoaWindow()) != 0
}
