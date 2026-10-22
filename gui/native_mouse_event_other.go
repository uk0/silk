//go:build !windows && (!darwin || !cgo)

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

func nativeHasMarkedText(window *glfw.Window) bool { return false }

func nativeMouseButtonEventPosition(window *glfw.Window, button glfw.MouseButton, action glfw.Action) (float64, float64, bool) {
	return 0, 0, false
}
