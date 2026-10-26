//go:build !windows && (!darwin || !cgo)

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

func installNativeTabNavigation(window *glfw.Window) func() { return nil }
