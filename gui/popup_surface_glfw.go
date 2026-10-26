//go:build !windows

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

func popupFramebufferHint(wt WindowType) int {
	if wt == WtPopup {
		return glfw.True
	}
	return glfw.False
}

func (w *Window) hasAlphaSurface() bool { return w.transparentFramebuffer }
