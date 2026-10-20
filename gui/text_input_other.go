//go:build !windows

package gui

// GLFW delivers committed Unicode scalar values rather than UTF-16 messages.
func textInputFocusChanged() {}
